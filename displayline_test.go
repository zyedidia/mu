package main

import (
	"testing"
)

// newWrapState creates a KeyState whose buffer is shown in a softwrapped
// view of the given width (no gutter, no line numbers).
func newWrapState(text string, width int) (*KeyState, *View) {
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
	v.Resize(width, 10)
	ks.activeView = func() *View { return v }
	return ks, v
}

// --- Geometry ---

func TestDisplayGeometry(t *testing.T) {
	_, v := newWrapState("abcdefghij\nxy\n\nabcde\n", 5)

	if r := v.displayRows(0); r != 2 {
		t.Fatalf("displayRows(0) = %d, want 2 (10 chars at width 5)", r)
	}
	if r := v.displayRows(1); r != 1 {
		t.Fatalf("displayRows(1) = %d, want 1", r)
	}
	if r := v.displayRows(2); r != 1 {
		t.Fatalf("displayRows(2) = %d, want 1 (empty line)", r)
	}
	// A line exactly filling its row does not get a phantom extra row.
	if r := v.displayRows(3); r != 1 {
		t.Fatalf("displayRows(3) = %d, want 1 (exact width)", r)
	}

	if row, col := v.displayLoc(7); row != 1 || col != 2 {
		t.Fatalf("displayLoc(7) = (%d,%d), want (1,2)", row, col)
	}
	if row, col := v.displayLoc(3); row != 0 || col != 3 {
		t.Fatalf("displayLoc(3) = (%d,%d), want (0,3)", row, col)
	}

	if p := v.displayPos(0, 1, 2); p != 7 {
		t.Fatalf("displayPos(0,1,2) = %d, want 7", p)
	}
	// wantX past the row end clamps to the row's last position.
	if p := v.displayPos(0, 0, 99); p != 4 {
		t.Fatalf("displayPos(0,0,99) = %d, want 4 (last cell of row 0)", p)
	}
}

// --- Motions ---

func TestDisplayMotionDownUp(t *testing.T) {
	ks, _ := newWrapState("abcdefghij\nqrstuvwxyz\n", 5)

	feedDisplay(ks, "ll") // col 2
	feedDisplay(ks, "gj")
	if cursorPos(ks) != 7 {
		t.Fatalf("gj within line: pos=%d, want 7", cursorPos(ks))
	}
	feedDisplay(ks, "gj")
	if cursorPos(ks) != 13 {
		t.Fatalf("gj across lines: pos=%d, want 13", cursorPos(ks))
	}
	feedDisplay(ks, "gj")
	if cursorPos(ks) != 18 {
		t.Fatalf("gj to last row: pos=%d, want 18", cursorPos(ks))
	}
	// The trailing newline ends the last line rather than starting one
	// after it, so there is nowhere further down to go.
	feedDisplay(ks, "gj")
	if cursorPos(ks) != 18 {
		t.Fatalf("gj at last row: pos=%d, want 18", cursorPos(ks))
	}

	feedDisplay(ks, "gk", "gk", "gk")
	if cursorPos(ks) != 2 {
		t.Fatalf("gk back to start: pos=%d, want 2", cursorPos(ks))
	}
	feedDisplay(ks, "gk")
	if cursorPos(ks) != 2 {
		t.Fatalf("gk at first row: pos=%d, want 2", cursorPos(ks))
	}
}

func TestDisplayMotionSticky(t *testing.T) {
	// Crossing a short display row keeps the desired column.
	ks, _ := newWrapState("abcdefghij\nx\nabcdefghij\n", 5)

	feedDisplay(ks, "lll") // col 3
	feedDisplay(ks, "gj")
	if cursorPos(ks) != 8 {
		t.Fatalf("gj: pos=%d, want 8", cursorPos(ks))
	}
	feedDisplay(ks, "gj") // short line: clamps to "x"
	if cursorPos(ks) != 11 {
		t.Fatalf("gj onto short line: pos=%d, want 11", cursorPos(ks))
	}
	feedDisplay(ks, "gj") // column restored
	if cursorPos(ks) != 16 {
		t.Fatalf("gj past short line: pos=%d, want 16 (column restored)", cursorPos(ks))
	}
	feedDisplay(ks, "gk", "gk", "gk")
	if cursorPos(ks) != 3 {
		t.Fatalf("gk chain back: pos=%d, want 3", cursorPos(ks))
	}
}

func TestDisplayMotionCount(t *testing.T) {
	ks, _ := newWrapState("abcdefghij\nqrstuvwxyz\n", 5)

	feedDisplay(ks, "3gj")
	if cursorPos(ks) != 16 {
		t.Fatalf("3gj: pos=%d, want 16", cursorPos(ks))
	}
	feedDisplay(ks, "2gk")
	if cursorPos(ks) != 5 {
		t.Fatalf("2gk: pos=%d, want 5", cursorPos(ks))
	}
}

func TestDisplayMotionNoWrapFallback(t *testing.T) {
	ks, v := newWrapState("abcdefghij\nqrstuvwxyz\n", 5)
	v.SoftWrap = false

	feedDisplay(ks, "ll", "gj")
	if cursorPos(ks) != 13 {
		t.Fatalf("gj without softwrap: pos=%d, want 13 (buffer line down)", cursorPos(ks))
	}
	feedDisplay(ks, "gk")
	if cursorPos(ks) != 2 {
		t.Fatalf("gk without softwrap: pos=%d, want 2", cursorPos(ks))
	}
}

func TestDisplayMotionVisual(t *testing.T) {
	ks, _ := newWrapState("abcdefghij\nqrstuvwxyz\n", 5)

	feedDisplay(ks, "v", "gj")
	c := ks.Buf().Cursor()
	if !c.HasSel || c.Sel[0] != 0 || c.Sel[1] != 6 {
		t.Fatalf("v gj: sel=%v, want [0,6]", c.Sel)
	}
}

func TestDisplayMotionOperator(t *testing.T) {
	// dgj deletes charwise from the cursor to the display-line target.
	ks, _ := newWrapState("abcdefghij\nqrstuvwxyz\n", 5)

	feedDisplay(ks, "dgj")
	if bufText(ks) != "fghij\nqrstuvwxyz\n" {
		t.Fatalf("dgj: got %q", bufText(ks))
	}
}

func TestDisplayMotionMixWithLineMotion(t *testing.T) {
	// After a gj chain, a buffer-line j converts the sticky display column
	// back to a line-wide column.
	ks, _ := newWrapState("abcdefghij\nqrstuvwxyz\n", 5)

	feedDisplay(ks, "ll", "gj") // pos 7 (line 0, line-wide col 7)
	if cursorPos(ks) != 7 {
		t.Fatalf("gj: pos=%d, want 7", cursorPos(ks))
	}
	feedDisplay(ks, "j") // buffer line down, keeping line-wide col 7
	if cursorPos(ks) != 18 {
		t.Fatalf("j after gj: pos=%d, want 18", cursorPos(ks))
	}
}

func TestDisplayMotionExactWidthLine(t *testing.T) {
	ks, _ := newWrapState("abcde\nxy\n", 5)

	feedDisplay(ks, "ll", "gj")
	if cursorPos(ks) != 7 {
		t.Fatalf("gj from exact-width line: pos=%d, want 7 (clamped to 'y')", cursorPos(ks))
	}
}

func TestDisplayMotionEmptyLines(t *testing.T) {
	ks, _ := newWrapState("abcdefghij\n\nxy\n", 5)

	feedDisplay(ks, "ll", "gj", "gj")
	if cursorPos(ks) != 11 {
		t.Fatalf("gj onto empty line: pos=%d, want 11", cursorPos(ks))
	}
	// Sticky column 2 clamps to the last character of "xy".
	feedDisplay(ks, "gj")
	if cursorPos(ks) != 13 {
		t.Fatalf("gj past empty line: pos=%d, want 13", cursorPos(ks))
	}
}

func TestDisplayMotionTabs(t *testing.T) {
	// Tab-aware row geometry: at width 8 with tabsize 4, "\tabcdefgh" wraps
	// after "abcd" (tab occupies cells 0-3).
	ks, v := newWrapState("\tabcdefgh\nqrstuvwxyz\n", 8)

	if r := v.displayRows(0); r != 2 {
		t.Fatalf("displayRows = %d, want 2", r)
	}
	if row, col := v.displayLoc(5); row != 1 || col != 0 {
		t.Fatalf("displayLoc('e') = (%d,%d), want (1,0)", row, col)
	}

	// From 'b' (screen col 5 on row 0), gj seeks col 5 on row 1: 'f'+... row
	// 1 is "efgh" (cols 0-3), so it clamps to 'h' at offset 8.
	feedDisplay(ks, "ll") // on 'b': screen col 5
	feedDisplay(ks, "gj")
	if cursorPos(ks) != 8 {
		t.Fatalf("gj with tab row: pos=%d, want 8", cursorPos(ks))
	}
}

// --- init.tcl integration ---

func TestInitScriptRemapsJK(t *testing.T) {
	ed := newMapTestEditor(t, "abcdefghij\nqrstuvwxyz\n")
	// Run the script before configuring the view: a `set` in init.tcl
	// refreshes view options from config, which would overwrite manual
	// view settings made earlier.
	ed.RunInitScript()
	v := ed.ActiveView()
	v.LineNums = false
	v.GutterWidth = 0
	v.SoftWrap = true
	v.Resize(5, 10)

	// The default init.tcl maps j/k to gj/gk: j moves one display row.
	feedDisplay(ed.ks, "j")
	if cursorPos(ed.ks) != 5 {
		t.Fatalf("remapped j: pos=%d, want 5 (next display row)", cursorPos(ed.ks))
	}
	feedDisplay(ed.ks, "j")
	if cursorPos(ed.ks) != 11 {
		t.Fatalf("remapped j: pos=%d, want 11", cursorPos(ed.ks))
	}
	feedDisplay(ed.ks, "k", "k")
	if cursorPos(ed.ks) != 0 {
		t.Fatalf("remapped k: pos=%d, want 0", cursorPos(ed.ks))
	}

	// dj is not remapped (operator-pending keeps buffer-line semantics):
	// it deletes two whole lines.
	feedDisplay(ed.ks, "dj")
	if bufText(ed.ks) != "" {
		t.Fatalf("dj with j remapped: got %q, want whole-buffer delete", bufText(ed.ks))
	}
}
