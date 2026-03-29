package main

import (
	"testing"
)

func TestViewRelocateDown(t *testing.T) {
	b := NewEmptyBuffer()
	// Create a 20-line file.
	for i := 0; i < 20; i++ {
		b.Insert(b.Len(), []byte("line\n"))
	}
	v := NewView(b, 4)
	v.Resize(80, 10)
	v.ScrollMargin = 2

	// Move cursor to line 15.
	pos := b.OffsetAt(15, 0)
	*b.Cursor() = b.Cursor().MoveTo(pos)
	v.Relocate()

	// topline should have scrolled so cursor is within view.
	if v.topline+v.height-v.ScrollMargin <= 15 {
		t.Fatalf("topline too low: %d (cursor at line 15, height 10)", v.topline)
	}
	if v.topline > 15 {
		t.Fatalf("topline too high: %d", v.topline)
	}
}

func TestViewRelocateUp(t *testing.T) {
	b := NewEmptyBuffer()
	for i := 0; i < 20; i++ {
		b.Insert(b.Len(), []byte("line\n"))
	}
	v := NewView(b, 4)
	v.Resize(80, 10)
	v.ScrollMargin = 2
	v.topline = 10

	// Move cursor to line 3.
	pos := b.OffsetAt(3, 0)
	*b.Cursor() = b.Cursor().MoveTo(pos)
	v.Relocate()

	if v.topline > 3-v.ScrollMargin {
		t.Fatalf("topline should scroll up: got %d", v.topline)
	}
}

func TestViewDisplay(t *testing.T) {
	b := NewEmptyBuffer()
	b.Insert(0, []byte("hello\nworld\n"))

	v := NewView(b, 4)
	v.Resize(40, 10)
	v.LineNums = true
	v.GutterWidth = 0

	// Move cursor to position 0.
	*b.Cursor() = b.Cursor().MoveTo(0)

	var drawn int
	var cursorX, cursorY int
	cursorFound := false

	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		drawn++
	}, func(x, y int, main bool) {
		if main {
			cursorX, cursorY = x, y
			cursorFound = true
		}
	}, DefaultTheme)

	if !cursorFound {
		t.Fatal("cursor not found in display")
	}
	if cursorY != 0 {
		t.Fatalf("cursor y: got %d, want 0", cursorY)
	}
	// Cursor should be offset by line number gutter.
	if cursorX < 1 {
		t.Fatalf("cursor x: got %d, should be offset by gutter", cursorX)
	}
	if drawn == 0 {
		t.Fatal("nothing drawn")
	}
}

func TestViewHorizontalScroll(t *testing.T) {
	b := NewEmptyBuffer()
	// Long line.
	b.Insert(0, []byte("abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ\n"))

	v := NewView(b, 4)
	v.Resize(20, 5)
	v.LineNums = false
	v.GutterWidth = 0
	v.HScrollMargin = 3

	// Move cursor far right.
	*b.Cursor() = b.Cursor().MoveTo(50)
	v.Relocate()

	if v.stcol == 0 {
		t.Fatal("stcol should have scrolled right")
	}
}

func TestViewDiagnosticGutter(t *testing.T) {
	b := NewEmptyBuffer()
	b.Insert(0, []byte("line1\nline2\nline3\n"))
	b.AddDiagnostic(1, 0, "test error", DiagError)

	v := NewView(b, 4)
	v.Resize(40, 5)
	v.GutterWidth = 1

	gutterDrawn := false
	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		if x == 0 && y == 1 && mainc == '>' {
			gutterDrawn = true
		}
	}, func(x, y int, main bool) {}, DefaultTheme)

	if !gutterDrawn {
		t.Fatal("diagnostic gutter marker not drawn")
	}
}

func TestVisualizerTab(t *testing.T) {
	vis := Visualizer{TabSize: 4, CharMap: make(map[rune]string)}

	sz := vis.Size('\t', 0, 1)
	if sz != 4 {
		t.Fatalf("tab at col 0: got %d, want 4", sz)
	}
	sz = vis.Size('\t', 1, 1)
	if sz != 3 {
		t.Fatalf("tab at col 1: got %d, want 3", sz)
	}
	sz = vis.Size('\t', 4, 1)
	if sz != 4 {
		t.Fatalf("tab at col 4: got %d, want 4", sz)
	}
}
