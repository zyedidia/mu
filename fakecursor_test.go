package main

import (
	"testing"
)

// renderCells displays the view with the given theme and returns the style
// and rune drawn at each screen cell.
func renderCells(v *View, th *Theme, active bool) (map[[2]int]Style, map[[2]int]rune) {
	cells := make(map[[2]int]Style)
	runes := make(map[[2]int]rune)
	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		cells[[2]int{x, y}] = style
		runes[[2]int{x, y}] = mainc
	}, func(x, y int, main bool) {}, th, active)
	return cells, runes
}

func newFakeCursorView(text string) (*Buffer, *View, *KeyState) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte(text))
	v := NewView(b, 4)
	v.Resize(20, 5)
	v.LineNums = false
	v.GutterWidth = 0
	ks := NewKeyState(b, NewRegisterSet())
	SetupBindings(ks)
	return b, v, ks
}

func TestFakeCursorsVisual(t *testing.T) {
	b, v, ks := newFakeCursorView("abc abc\n")

	feedDisplay(ks, "<C-n>", "<C-n>") // select both "abc" occurrences
	if b.NumCursors() != 2 {
		t.Fatalf("cursors = %d, want 2", b.NumCursors())
	}

	def := DefaultTheme.Default()
	rev := def.Add(AttrReverse)
	cells, runes := renderCells(v, DefaultTheme, true)

	// Selection cells are reverse video.
	for _, x := range []int{0, 1, 4, 5} {
		if cells[[2]int{x, 0}] != rev {
			t.Fatalf("cell %d = %v, want selection style", x, cells[[2]int{x, 0}])
		}
	}
	// The cursor cell inside each selection is inverted back to the
	// default style, so it stands out against the selection.
	for _, x := range []int{2, 6} {
		if cells[[2]int{x, 0}] != def {
			t.Fatalf("cursor cell %d = %v, want inverted selection", x, cells[[2]int{x, 0}])
		}
		if runes[[2]int{x, 0}] != 'c' {
			t.Fatalf("cursor cell %d rune = %q, want 'c'", x, runes[[2]int{x, 0}])
		}
	}
}

func TestFakeCursorsNoSelection(t *testing.T) {
	b, v, _ := newFakeCursorView("abc abc\n")
	b.SpawnCursor(4)

	def := DefaultTheme.Default()
	rev := def.Add(AttrReverse)
	cells, runes := renderCells(v, DefaultTheme, true)

	for _, x := range []int{0, 4} {
		if cells[[2]int{x, 0}] != rev {
			t.Fatalf("cursor cell %d = %v, want reverse", x, cells[[2]int{x, 0}])
		}
		if runes[[2]int{x, 0}] != 'a' {
			t.Fatalf("cursor cell %d rune = %q, want 'a'", x, runes[[2]int{x, 0}])
		}
	}
	for _, x := range []int{1, 2, 3, 5, 6} {
		if cells[[2]int{x, 0}] != def {
			t.Fatalf("cell %d = %v, want default", x, cells[[2]int{x, 0}])
		}
	}
}

func TestFakeCursorsSingleCursorOff(t *testing.T) {
	// With a single cursor the hardware cursor is used: no cell is
	// restyled, and the cursor position is still reported as main.
	_, v, _ := newFakeCursorView("abc\n")

	def := DefaultTheme.Default()
	var mains [][2]int
	cells := make(map[[2]int]Style)
	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		cells[[2]int{x, y}] = style
	}, func(x, y int, main bool) {
		if main {
			mains = append(mains, [2]int{x, y})
		}
	}, DefaultTheme)

	for cell, s := range cells {
		if s != def {
			t.Fatalf("cell %v = %v, want default (no fake cursor)", cell, s)
		}
	}
	if len(mains) != 1 || mains[0] != [2]int{0, 0} {
		t.Fatalf("main cursor reported at %v, want [(0,0)]", mains)
	}
}

func TestFakeCursorsEndOfBuffer(t *testing.T) {
	// A cursor on the phantom line past the final newline has no glyph;
	// it gets a blank overlay cell.
	b, v, _ := newFakeCursorView("ab\n")
	b.SpawnCursor(b.Len())

	def := DefaultTheme.Default()
	rev := def.Add(AttrReverse)
	cells, runes := renderCells(v, DefaultTheme, true)

	if cells[[2]int{0, 0}] != rev || runes[[2]int{0, 0}] != 'a' {
		t.Fatalf("primary cell = %v %q, want reverse 'a'", cells[[2]int{0, 0}], runes[[2]int{0, 0}])
	}
	if cells[[2]int{0, 1}] != rev || runes[[2]int{0, 1}] != ' ' {
		t.Fatalf("EOF cell = %v %q, want reverse blank", cells[[2]int{0, 1}], runes[[2]int{0, 1}])
	}
}
func TestFakeCursorsWrappedEdge(t *testing.T) {
	// A cursor one past an exactly-full softwrapped row (insert mode at
	// the end of such a line) is drawn at the start of the row below,
	// keeping the glyph already there under it, and leaves the full row's
	// last column alone.
	b, v, _ := newFakeCursorView("abcde\nxy\n")
	v.SoftWrap = true
	v.Resize(5, 5)
	b.cursors[0].Pos = 5 // on the newline of the exactly-full line
	b.SpawnCursor(7)     // on 'y'

	def := DefaultTheme.Default()
	rev := def.Add(AttrReverse)
	cells, runes := renderCells(v, DefaultTheme, true)

	if cells[[2]int{0, 1}] != rev || runes[[2]int{0, 1}] != 'x' {
		t.Fatalf("wrapped cell = %v %q, want reverse 'x'", cells[[2]int{0, 1}], runes[[2]int{0, 1}])
	}
	if cells[[2]int{4, 0}] != def || runes[[2]int{4, 0}] != 'e' {
		t.Fatalf("full row's last column = %v %q, want a plain 'e'", cells[[2]int{4, 0}], runes[[2]int{4, 0}])
	}
	if cells[[2]int{1, 1}] != rev || runes[[2]int{1, 1}] != 'y' {
		t.Fatalf("second cursor cell = %v %q, want reverse 'y'", cells[[2]int{1, 1}], runes[[2]int{1, 1}])
	}
}

func TestFakeCursorsPinnedEdgeNoRowBelow(t *testing.T) {
	// With no row below on screen there is nowhere to wrap onto, so the
	// cursor is pinned to the last column, over the glyph there.
	b, v, _ := newFakeCursorView("abcde\nx\n")
	v.SoftWrap = true
	v.Resize(5, 1)
	b.cursors[0].Pos = 5
	b.SpawnCursor(0)

	def := DefaultTheme.Default()
	rev := def.Add(AttrReverse)
	cells, runes := renderCells(v, DefaultTheme, true)

	if cells[[2]int{4, 0}] != rev || runes[[2]int{4, 0}] != 'e' {
		t.Fatalf("pinned cell = %v %q, want reverse 'e'", cells[[2]int{4, 0}], runes[[2]int{4, 0}])
	}
}

func TestFakeCursorsThemeStyle(t *testing.T) {
	// A theme "cursor" style overrides the inverted-cell default.
	th, err := LoadThemeYAML([]byte(`
default:
  fg: white
  bg: black
cursor:
  fg: black
  bg: yellow
`))
	if err != nil {
		t.Fatal(err)
	}

	b, v, _ := newFakeCursorView("abc abc\n")
	b.SpawnCursor(4)

	cells, _ := renderCells(v, th, true)
	want := th.Style("cursor")
	for _, x := range []int{0, 4} {
		if cells[[2]int{x, 0}] != want {
			t.Fatalf("cursor cell %d = %v, want theme cursor style %v", x, cells[[2]int{x, 0}], want)
		}
	}
	if cells[[2]int{1, 0}] != th.Default() {
		t.Fatalf("cell 1 = %v, want default", cells[[2]int{1, 0}])
	}
}

func TestFakeCursorsInactivePane(t *testing.T) {
	// An inactive pane draws no fake cursors, matching the hardware
	// cursor being shown only in the active pane.
	b, v, _ := newFakeCursorView("abc abc\n")
	b.SpawnCursor(4)

	def := DefaultTheme.Default()
	cells, _ := renderCells(v, DefaultTheme, false)
	for cell, s := range cells {
		if s != def {
			t.Fatalf("inactive pane cell %v = %v, want default", cell, s)
		}
	}
}
