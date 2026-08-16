package main

import (
	"strings"
	"testing"
)

// newScrollState creates a KeyState with a softwrapped view of the given
// geometry (no gutter or line numbers, explicit scroll margin).
func newScrollState(text string, width, height, margin int) (*KeyState, *View) {
	b := NewEmptyBuffer()
	if len(text) > 0 {
		b.text.Insert(0, []byte(text))
	}
	*b.Cursor() = b.Cursor().MoveTo(0)
	ks := NewKeyState(b, NewRegisterSet())
	SetupBindings(ks)
	v := NewView(b, 4)
	v.LineNums = false
	v.GutterWidth = 0
	v.SoftWrap = true
	v.ScrollMargin = margin
	v.Resize(width, height)
	ks.activeView = func() *View { return v }
	return ks, v
}

// cursorScreenRow relocates and renders the view, returning the screen row
// the primary cursor was drawn on (-1 if it never appeared).
func cursorScreenRow(v *View) int {
	v.Relocate()
	row := -1
	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {},
		func(x, y int, main bool) {
			if main {
				row = y
			}
		}, DefaultTheme)
	return row
}

// fourWrapped is four 10-char lines: at width 5 each occupies 2 visual rows.
const fourWrapped = "aaaaaaaaaa\nbbbbbbbbbb\ncccccccccc\ndddddddddd\n"

func TestRelocateWrappedDown(t *testing.T) {
	ks, v := newScrollState(fourWrapped, 5, 4, 0)
	b := ks.Buf()

	// Cursor on line 2 (absolute visual row 4, beyond the 4-row window).
	*b.Cursor() = b.Cursor().MoveTo(22)
	if row := cursorScreenRow(v); row != 3 {
		t.Fatalf("cursor screen row = %d, want 3 (bottom)", row)
	}
	if v.topline != 0 || v.topcol != 5 {
		t.Fatalf("top = (%d,%d), want (0,5) (second row of line 0)", v.topline, v.topcol)
	}

	// Back to the start: viewport returns to the very top.
	*b.Cursor() = b.Cursor().MoveTo(0)
	if row := cursorScreenRow(v); row != 0 {
		t.Fatalf("cursor screen row = %d, want 0", row)
	}
	if v.topline != 0 || v.topcol != 0 {
		t.Fatalf("top = (%d,%d), want (0,0)", v.topline, v.topcol)
	}
}

func TestRelocateTallLine(t *testing.T) {
	// A single line taller than the window: the viewport enters it
	// gradually via topcol.
	ks, v := newScrollState(strings.Repeat("x", 60)+"\n", 5, 4, 0)
	b := ks.Buf()

	*b.Cursor() = b.Cursor().MoveTo(30) // visual row 6 of 12
	if row := cursorScreenRow(v); row != 3 {
		t.Fatalf("cursor screen row = %d, want 3", row)
	}
	if v.topline != 0 || v.topcol != 15 {
		t.Fatalf("top = (%d,%d), want (0,15) (row 3 of the line)", v.topline, v.topcol)
	}

	*b.Cursor() = b.Cursor().MoveTo(59) // last row
	if row := cursorScreenRow(v); row != 3 {
		t.Fatalf("cursor screen row = %d, want 3", row)
	}
	if v.topcol != 40 {
		t.Fatalf("topcol = %d, want 40", v.topcol)
	}

	// Scrolling back up within the same line.
	*b.Cursor() = b.Cursor().MoveTo(0)
	if row := cursorScreenRow(v); row != 0 {
		t.Fatalf("cursor screen row = %d, want 0", row)
	}
	if v.topcol != 0 {
		t.Fatalf("topcol = %d, want 0", v.topcol)
	}
}

func TestRelocateEOFBottomAligned(t *testing.T) {
	ks, v := newScrollState("aaaaaaaaaa\nbbbbbbbbbb\ncccccccccc\n", 5, 4, 0)
	b := ks.Buf()

	feedDisplay(ks, "G")
	if row := cursorScreenRow(v); row != 3 {
		t.Fatalf("cursor screen row after G = %d, want 3", row)
	}
	// maxTop: the buffer's last visual row sits on the bottom screen row
	// (rows 3-6 of 7: second row of line 1 through the trailing line).
	if v.topline != 1 || v.topcol != 5 {
		t.Fatalf("top = (%d,%d), want (1,5)", v.topline, v.topcol)
	}
	_ = b
}

func TestRelocateWrappedMargin(t *testing.T) {
	ks, v := newScrollState(fourWrapped, 5, 6, 1)
	b := ks.Buf()

	// Line 2 row 1 is absolute row 5 = height-margin: scrolls so the
	// cursor sits margin rows above the bottom.
	*b.Cursor() = b.Cursor().MoveTo(27)
	if row := cursorScreenRow(v); row != 4 {
		t.Fatalf("cursor screen row = %d, want 4 (height-1-margin)", row)
	}
	if v.topline != 0 || v.topcol != 5 {
		t.Fatalf("top = (%d,%d), want (0,5)", v.topline, v.topcol)
	}

	// Moving back above the top margin scrolls up, leaving margin rows
	// above the cursor.
	*b.Cursor() = b.Cursor().MoveTo(11) // line 1 row 0
	if row := cursorScreenRow(v); row != 1 {
		t.Fatalf("cursor screen row = %d, want 1 (margin)", row)
	}
	if v.topline != 0 || v.topcol != 5 {
		t.Fatalf("top = (%d,%d), want (0,5)", v.topline, v.topcol)
	}
}

func TestScrollCtrlE(t *testing.T) {
	ks, v := newScrollState(fourWrapped, 5, 4, 0)

	feedSpecial(ks, "<C-e>")
	if v.topline != 0 || v.topcol != 5 {
		t.Fatalf("top after C-e = (%d,%d), want (0,5)", v.topline, v.topcol)
	}
	// The cursor was on the row scrolled off: pushed to the new top row.
	if cursorPos(ks) != 5 {
		t.Fatalf("cursor after C-e = %d, want 5", cursorPos(ks))
	}
	// Relocate must not undo the scroll.
	v.Relocate()
	if v.topline != 0 || v.topcol != 5 {
		t.Fatalf("top after relocate = (%d,%d), want (0,5)", v.topline, v.topcol)
	}

	feedSpecial(ks, "<C-y>")
	if v.topline != 0 || v.topcol != 0 {
		t.Fatalf("top after C-y = (%d,%d), want (0,0)", v.topline, v.topcol)
	}
}

func TestScrollCtrlYPushesCursor(t *testing.T) {
	ks, v := newScrollState(fourWrapped, 5, 4, 0)
	b := ks.Buf()

	// Start scrolled down with the cursor on the last visible row.
	v.setTopRow(1, 0)
	*b.Cursor() = b.Cursor().MoveTo(27) // line 2 row 1 (bottom row)
	feedSpecial(ks, "<C-y>")
	if v.topline != 0 || v.topcol != 5 {
		t.Fatalf("top after C-y = (%d,%d), want (0,5)", v.topline, v.topcol)
	}
	// Cursor pushed up to the new bottom row (line 2 row 0).
	if line, row := v.displayRowOf(cursorPos(ks)); line != 2 || row != 0 {
		t.Fatalf("cursor at (%d,%d), want (2,0)", line, row)
	}
}

func TestScrollHalfPage(t *testing.T) {
	ks, v := newScrollState(fourWrapped+"eeeeeeeeee\n", 5, 4, 0)

	// Ctrl-D scrolls view and cursor by 2 visual rows: same screen row.
	before := cursorScreenRow(v)
	feedSpecial(ks, "<C-d>")
	if v.topline != 1 || v.topcol != 0 {
		t.Fatalf("top after C-d = (%d,%d), want (1,0)", v.topline, v.topcol)
	}
	if cursorPos(ks) != 11 {
		t.Fatalf("cursor after C-d = %d, want 11", cursorPos(ks))
	}
	if after := cursorScreenRow(v); after != before {
		t.Fatalf("cursor screen row changed: %d -> %d", before, after)
	}

	feedSpecial(ks, "<C-u>")
	if v.topline != 0 || v.topcol != 0 {
		t.Fatalf("top after C-u = (%d,%d), want (0,0)", v.topline, v.topcol)
	}
	if cursorPos(ks) != 0 {
		t.Fatalf("cursor after C-u = %d, want 0", cursorPos(ks))
	}
}

func TestPageCtrlF(t *testing.T) {
	ks, _ := newScrollState(fourWrapped+"eeeeeeeeee\n", 5, 4, 0)

	// Ctrl-F moves the cursor a full window of visual rows (4 rows = 2
	// wrapped lines).
	feedSpecial(ks, "<C-f>")
	if cursorPos(ks) != 22 {
		t.Fatalf("cursor after C-f = %d, want 22", cursorPos(ks))
	}
	feedSpecial(ks, "<C-b>")
	if cursorPos(ks) != 0 {
		t.Fatalf("cursor after C-b = %d, want 0", cursorPos(ks))
	}
}

func TestZCommandsWrapped(t *testing.T) {
	ks, v := newScrollState(strings.Repeat("x", 60)+"\n", 5, 4, 0)
	b := ks.Buf()

	*b.Cursor() = b.Cursor().MoveTo(30) // row 6

	feedKeys(ks, "zz")
	if row := cursorScreenRow(v); row != 2 {
		t.Fatalf("zz: cursor screen row = %d, want 2 (center)", row)
	}
	if v.topcol != 20 {
		t.Fatalf("zz: topcol = %d, want 20", v.topcol)
	}

	feedKeys(ks, "zt")
	if row := cursorScreenRow(v); row != 0 {
		t.Fatalf("zt: cursor screen row = %d, want 0", row)
	}
	if v.topcol != 30 {
		t.Fatalf("zt: topcol = %d, want 30", v.topcol)
	}

	feedKeys(ks, "zb")
	if row := cursorScreenRow(v); row != 3 {
		t.Fatalf("zb: cursor screen row = %d, want 3", row)
	}
	if v.topcol != 15 {
		t.Fatalf("zb: topcol = %d, want 15", v.topcol)
	}
}

func TestHMLWrapped(t *testing.T) {
	ks, v := newScrollState(fourWrapped, 5, 4, 0)

	// Scroll mid-line so screen rows and buffer lines disagree.
	v.setTopRow(0, 1)

	feedKeys(ks, "H")
	if cursorPos(ks) != 5 {
		t.Fatalf("H: pos=%d, want 5 (start of top row)", cursorPos(ks))
	}
	feedKeys(ks, "M")
	if cursorPos(ks) != 16 {
		t.Fatalf("M: pos=%d, want 16 (row height/2)", cursorPos(ks))
	}
	feedKeys(ks, "L")
	if cursorPos(ks) != 22 {
		t.Fatalf("L: pos=%d, want 22 (bottom row)", cursorPos(ks))
	}
}

func TestRelocateStaleTopcol(t *testing.T) {
	ks, v := newScrollState("abc\ndef\n", 5, 4, 0)

	// Simulate an edit that left topcol pointing past the line's end.
	v.topline = 0
	v.topcol = 8
	if row := cursorScreenRow(v); row != 0 {
		t.Fatalf("cursor screen row = %d, want 0", row)
	}
	if v.topcol != 0 {
		t.Fatalf("stale topcol not repaired: %d", v.topcol)
	}
	_ = ks
}

func TestRelocateTopcolAfterResize(t *testing.T) {
	ks, v := newScrollState(fourWrapped, 5, 4, 0)

	v.setTopRow(0, 1)
	if v.topcol != 5 {
		t.Fatalf("topcol = %d, want 5", v.topcol)
	}
	// Wider view: the line no longer wraps, so the mid-line top snaps back
	// to a valid row boundary (the line start).
	v.Resize(20, 4)
	if row := cursorScreenRow(v); row != 0 {
		t.Fatalf("cursor screen row = %d, want 0", row)
	}
	if v.topcol != 0 {
		t.Fatalf("topcol after resize = %d, want 0", v.topcol)
	}
	_ = ks
}

func TestExactWidthLineNoBlankRow(t *testing.T) {
	// A line whose length is an exact multiple of the width must not be
	// followed by a blank continuation row: "bb" renders on screen row 2.
	_, v := newScrollState("aaaaaaaaaa\nbb\n", 5, 6, 0)

	rowText := make(map[int][]rune)
	v.Relocate()
	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		if mainc != ' ' {
			rowText[y] = append(rowText[y], mainc)
		}
	}, func(x, y int, main bool) {}, DefaultTheme)

	if string(rowText[2]) != "bb" {
		t.Fatalf("row 2 = %q, want %q (rows: %v)", string(rowText[2]), "bb", rowText)
	}
	if len(rowText[3]) != 0 {
		t.Fatalf("row 3 should be empty, got %q", string(rowText[3]))
	}
}

func TestScrollNoSoftwrapUnchanged(t *testing.T) {
	// With softwrap off the row machinery degrades to buffer lines.
	ks, v := newScrollState("a\nb\nc\nd\ne\nf\ng\nh\n", 5, 4, 0)
	v.SoftWrap = false

	feedSpecial(ks, "<C-e>")
	if v.topline != 1 || v.topcol != 0 {
		t.Fatalf("top after C-e = (%d,%d), want (1,0)", v.topline, v.topcol)
	}
	feedSpecial(ks, "<C-d>")
	if v.topline != 3 {
		t.Fatalf("top after C-d = %d, want 3", v.topline)
	}
	line, _ := ks.Buf().LineColAt(cursorPos(ks))
	if line != 3 {
		t.Fatalf("cursor line after C-d = %d, want 3", line)
	}
}
