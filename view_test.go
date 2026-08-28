package main

import (
	"strings"
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

// An inlay hint draws an 'i' gutter marker, but a diagnostic on the same
// line takes precedence over it.
func TestViewInlayHintGutter(t *testing.T) {
	b := NewEmptyBuffer()
	b.Insert(0, []byte("line1\nline2\nline3\n"))
	b.SetInlayHints([]InlayHintMark{{Line: 0, Text: ": int"}, {Line: 1, Text: ": string"}})
	b.AddDiagnostic(1, 0, "test error", DiagError)

	v := NewView(b, 4)
	v.Resize(40, 5)
	v.GutterWidth = 1

	var line0, line1 rune
	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		if x != 0 {
			return
		}
		switch y {
		case 0:
			line0 = mainc
		case 1:
			line1 = mainc
		}
	}, func(x, y int, main bool) {}, DefaultTheme)

	if line0 != 'i' {
		t.Fatalf("line 0 (inlay hint only): got %q, want 'i'", line0)
	}
	if line1 != '>' {
		t.Fatalf("line 1 (diagnostic + inlay hint): got %q, want '>' (diagnostic takes precedence)", line1)
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

// cursorScreenPos renders v and returns where the main cursor was placed.
func cursorScreenPos(v *View) (x, y int, found bool) {
	x, y = -1, -1
	v.Display(func(int, int, rune, []rune, Style) {}, func(cx, cy int, main bool) {
		if main {
			x, y, found = cx, cy, true
		}
	}, DefaultTheme, true)
	return x, y, found
}

// A cursor one past the last cell of an exactly-full softwrapped row (insert
// mode appending at the end of such a line) belongs at the start of the row
// below — where the next character typed will land — not on top of the
// line's last character.
func TestCursorEndOfFullRowWrapsToNextRow(t *testing.T) {
	full := strings.Repeat("a", 10)
	tests := []struct {
		name         string
		text         string
		pos          int
		wantX, wantY int
	}{
		{"line exactly fills the row", full + "\nnext\n", 10, 0, 1},
		{"line fills two rows", strings.Repeat("a", 20) + "\nnext\n", 20, 0, 2},
		{"last line of the buffer", full + "\n", 10, 0, 1},
		{"no trailing newline", full, 10, 0, 1},
		{"one column short", strings.Repeat("a", 9) + "\nnext\n", 9, 9, 0},
		{"mid-row position", strings.Repeat("a", 15) + "\nnext\n", 15, 5, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewEmptyBuffer()
			b.text.Insert(0, []byte(tt.text))
			v := NewView(b, 4)
			v.Resize(10, 5)
			v.LineNums = false
			v.GutterWidth = 0
			v.SoftWrap = true
			*b.Cursor() = b.Cursor().MoveTo(tt.pos)
			v.Relocate()

			x, y, found := cursorScreenPos(v)
			if !found {
				t.Fatal("cursor not placed")
			}
			if x != tt.wantX || y != tt.wantY {
				t.Fatalf("cursor at (%d,%d), want (%d,%d)", x, y, tt.wantX, tt.wantY)
			}
		})
	}
}

// The wrapped position starts after the gutter, not at screen column 0.
func TestCursorEndOfFullRowWrapsPastGutter(t *testing.T) {
	b := NewEmptyBuffer()
	v := NewView(b, 4)
	v.Resize(14, 5)
	v.LineNums = true
	v.GutterWidth = 1
	v.SoftWrap = true
	gutter := v.gutterTotalWidth()
	// A line that exactly fills the text area, which the gutter narrows.
	b.text.Insert(0, []byte(strings.Repeat("a", v.bufferWidth())+"\nnext\n"))
	*b.Cursor() = b.Cursor().MoveTo(v.bufferWidth())
	v.Relocate()

	x, y, found := cursorScreenPos(v)
	if !found {
		t.Fatal("cursor not placed")
	}
	if x != gutter || y != 1 {
		t.Fatalf("cursor at (%d,%d), want (%d,1)", x, y, gutter)
	}
}

// Without softwrap there is no row below to wrap onto, and on the last row
// there is none on screen: the cursor stays on its own row.
func TestCursorEndOfFullRowStaysWhenNoRowBelow(t *testing.T) {
	full := strings.Repeat("a", 10)

	t.Run("no softwrap", func(t *testing.T) {
		b := NewEmptyBuffer()
		b.text.Insert(0, []byte(full+"\nnext\n"))
		v := NewView(b, 4)
		v.Resize(10, 5)
		v.LineNums = false
		v.GutterWidth = 0
		v.SoftWrap = false
		*b.Cursor() = b.Cursor().MoveTo(10)
		v.Relocate()

		x, y, found := cursorScreenPos(v)
		if !found {
			t.Fatal("cursor not placed")
		}
		if y != 0 || x >= v.width {
			t.Fatalf("cursor at (%d,%d), want it on row 0 inside the view", x, y)
		}
	})

	t.Run("last row on screen", func(t *testing.T) {
		b := NewEmptyBuffer()
		b.text.Insert(0, []byte("x\nx\nx\nx\n"+full+"\n"))
		v := NewView(b, 4)
		v.Resize(10, 5)
		v.LineNums = false
		v.GutterWidth = 0
		v.SoftWrap = true
		v.ScrollMargin = 0
		*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(4, 10))
		v.topline = 0 // the full line is the bottom row

		x, y, found := cursorScreenPos(v)
		if !found {
			t.Fatal("cursor not placed")
		}
		if x != v.width-1 || y != v.height-1 {
			t.Fatalf("cursor at (%d,%d), want it pinned to (%d,%d)", x, y, v.width-1, v.height-1)
		}
	})
}
