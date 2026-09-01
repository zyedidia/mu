package main

import (
	"testing"
)

// feedDisplay feeds keys through the state machine, recalculating Vx after
// each key the way the editor's Display loop does, so that j/k preserve the
// cursor column in tests.
func feedDisplay(ks *KeyState, keys ...string) {
	for _, k := range keys {
		for _, ch := range splitKeys(k) {
			ks.HandleKey(ch)
			if !ks.vertical && len(ks.keys) == 0 {
				b := ks.Buf()
				for i := 0; i < b.NumCursors(); i++ {
					b.cursors[i].Vx = b.VisualCol(b.cursors[i].Pos)
				}
				ks.displayVx = false
			}
			ks.vertical = false
		}
	}
}

// splitKeys splits a string into key events; a string starting with '<' is
// treated as a single special key (e.g. "<C-v>").
func splitKeys(s string) []string {
	if len(s) > 1 && s[0] == '<' {
		return []string{s}
	}
	keys := make([]string, 0, len(s))
	for _, ch := range s {
		keys = append(keys, string(ch))
	}
	return keys
}

func TestBlockEnterExit(t *testing.T) {
	ks := newVimState("hello\nworld\n")

	feedDisplay(ks, "<C-v>")
	if ks.ModeID() != ModeVisualBlock {
		t.Fatalf("mode = %v, want ModeVisualBlock", ks.ModeID())
	}
	if ks.Mode().Name != "V-BLOCK" {
		t.Fatalf("mode name = %q, want V-BLOCK", ks.Mode().Name)
	}
	c := ks.Buf().Cursor()
	if !c.HasSel || !c.BlockSel {
		t.Fatalf("expected block selection, got HasSel=%v BlockSel=%v", c.HasSel, c.BlockSel)
	}

	feedSpecial(ks, KeyEscape)
	if ks.ModeID() != ModeNormal {
		t.Fatalf("mode after escape = %v, want normal", ks.ModeID())
	}
	if ks.Buf().Cursor().HasSel {
		t.Fatal("selection should be cleared after escape")
	}

	// <C-v> toggles off too.
	feedDisplay(ks, "<C-v>", "<C-v>")
	if ks.ModeID() != ModeNormal {
		t.Fatalf("mode after C-v C-v = %v, want normal", ks.ModeID())
	}
}

func TestBlockDelete(t *testing.T) {
	ks := newVimState("abcd\nefgh\nijkl\n")

	feedDisplay(ks, "l", "<C-v>", "jjl", "d")
	if bufText(ks) != "ad\neh\nil\n" {
		t.Fatalf("block delete: got %q", bufText(ks))
	}
	if cursorPos(ks) != 1 {
		t.Fatalf("cursor after block delete: got %d, want 1", cursorPos(ks))
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be in normal mode after d")
	}

	reg := ks.regs.Get(RegDefault)
	if !reg.Block || string(reg.Content) != "bc\nfg\njk" || reg.BlockWidth != 2 {
		t.Fatalf("register: block=%v content=%q width=%d", reg.Block, reg.Content, reg.BlockWidth)
	}
}

func TestBlockDeleteReverse(t *testing.T) {
	ks := newVimState("abcd\nefgh\nijkl\n")

	// Anchor at bottom-right, extend to top-left.
	feedDisplay(ks, "jjll", "<C-v>", "kkhh", "d")
	if bufText(ks) != "d\nh\nl\n" {
		t.Fatalf("reverse block delete: got %q", bufText(ks))
	}
}

func TestBlockDeleteX(t *testing.T) {
	ks := newVimState("abc\ndef\n")

	feedDisplay(ks, "<C-v>", "j", "x")
	if bufText(ks) != "bc\nef\n" {
		t.Fatalf("block x: got %q", bufText(ks))
	}
}

func TestBlockDeleteShortLine(t *testing.T) {
	// The middle line does not reach the block's columns: it is untouched.
	ks := newVimState("abcdef\nab\nabcdef\n")

	feedDisplay(ks, "lll", "<C-v>", "jjl", "d")
	if bufText(ks) != "abcf\nab\nabcf\n" {
		t.Fatalf("short line block delete: got %q", bufText(ks))
	}
	reg := ks.regs.Get(RegDefault)
	if string(reg.Content) != "de\n\nde" {
		t.Fatalf("register content: %q", reg.Content)
	}
}

func TestBlockYankPaste(t *testing.T) {
	ks := newVimState("ab\ncd\n")

	feedDisplay(ks, "<C-v>", "j", "y")
	reg := ks.regs.Get(RegDefault)
	if !reg.Block || string(reg.Content) != "a\nc" {
		t.Fatalf("yank register: block=%v content=%q", reg.Block, reg.Content)
	}
	if cursorPos(ks) != 0 {
		t.Fatalf("cursor after yank: got %d, want 0 (top-left)", cursorPos(ks))
	}

	// p pastes the block after the cursor column.
	feedDisplay(ks, "l", "p")
	if bufText(ks) != "aba\ncdc\n" {
		t.Fatalf("block paste: got %q", bufText(ks))
	}
}

func TestBlockPasteBefore(t *testing.T) {
	ks := newVimState("ab\ncd\n")

	feedDisplay(ks, "<C-v>", "j", "y", "l", "P")
	if bufText(ks) != "aab\nccd\n" {
		t.Fatalf("block P: got %q", bufText(ks))
	}
}

func TestBlockPastePadsShortLines(t *testing.T) {
	ks := newVimState("abcd\nab\n")

	// Yank a 2x2 block, then paste it after the end of a short line.
	feedDisplay(ks, "<C-v>", "jl", "y", "lll", "p")
	if bufText(ks) != "abcdab\nab  ab\n" {
		t.Fatalf("block paste pad: got %q", bufText(ks))
	}
}

func TestBlockPasteCreatesLines(t *testing.T) {
	ks := newVimState("xy\n")

	ks.regs.SetDefaultBlock([]byte("a\nb\nc"), 1, false)
	feedDisplay(ks, "p")
	// The lines the paste adds are real lines, so the file keeps the
	// final newline it had.
	if bufText(ks) != "xay\n b\n c\n" {
		t.Fatalf("block paste past EOF: got %q", bufText(ks))
	}
}

func TestBlockPasteCount(t *testing.T) {
	ks := newVimState("ab\ncd\n")

	ks.regs.SetDefaultBlock([]byte("x\ny"), 1, false)
	feedDisplay(ks, "3p")
	if bufText(ks) != "axxxb\ncyyyd\n" {
		t.Fatalf("block paste with count: got %q", bufText(ks))
	}
}

func TestBlockChange(t *testing.T) {
	ks := newVimState("abc\nabc\n")

	feedDisplay(ks, "<C-v>", "j", "c")
	if ks.ModeID() != ModeInsert {
		t.Fatal("c should enter insert mode")
	}
	if ks.Buf().NumCursors() != 2 {
		t.Fatalf("expected 2 cursors, got %d", ks.Buf().NumCursors())
	}
	feedKeys(ks, "XY")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "XYbc\nXYbc\n" {
		t.Fatalf("block change: got %q", bufText(ks))
	}
	if ks.Buf().NumCursors() != 1 {
		t.Fatalf("cursors should collapse after escape, got %d", ks.Buf().NumCursors())
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be back in normal mode")
	}
}

func TestBlockChangeMiddle(t *testing.T) {
	ks := newVimState("a12d\na34d\n")

	feedDisplay(ks, "l", "<C-v>", "jl", "c")
	feedKeys(ks, "X")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "aXd\naXd\n" {
		t.Fatalf("block change middle: got %q", bufText(ks))
	}
}

func TestBlockInsertI(t *testing.T) {
	ks := newVimState("abc\nabc\n")

	feedDisplay(ks, "l", "<C-v>", "j", "I")
	feedKeys(ks, "-")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "a-bc\na-bc\n" {
		t.Fatalf("block I: got %q", bufText(ks))
	}
}

func TestBlockInsertSkipsShortLines(t *testing.T) {
	ks := newVimState("abcd\nx\nabcd\n")

	feedDisplay(ks, "ll", "<C-v>", "jj", "I")
	feedKeys(ks, "Z")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "abZcd\nx\nabZcd\n" {
		t.Fatalf("block I short lines: got %q", bufText(ks))
	}
}

func TestBlockAppend(t *testing.T) {
	// The short middle line is padded with spaces up to the append column.
	ks := newVimState("abcd\na\nabcd\n")

	feedDisplay(ks, "<C-v>", "ljj", "A")
	feedKeys(ks, "X")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "abXcd\na X\nabXcd\n" {
		t.Fatalf("block A: got %q", bufText(ks))
	}
}

func TestBlockAppendEOL(t *testing.T) {
	ks := newVimState("ab\nabcd\n")

	// $ makes A append at each line's end without padding.
	feedDisplay(ks, "<C-v>", "j$", "A")
	feedKeys(ks, "!")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "ab!\nabcd!\n" {
		t.Fatalf("block $ A: got %q", bufText(ks))
	}
}

func TestBlockDollarDelete(t *testing.T) {
	ks := newVimState("abcd\nab\nabcdef\n")

	feedDisplay(ks, "l", "<C-v>", "jj$", "d")
	if bufText(ks) != "a\na\na\n" {
		t.Fatalf("block $ delete: got %q", bufText(ks))
	}
	reg := ks.regs.Get(RegDefault)
	if string(reg.Content) != "bcd\nb\nbcdef" || reg.BlockWidth != 5 {
		t.Fatalf("register: content=%q width=%d", reg.Content, reg.BlockWidth)
	}
}

func TestBlockDollarStaysAtEOL(t *testing.T) {
	// After $, moving vertically keeps the right edge pinned to line ends.
	ks := newVimState("ab\nabcdef\n")

	feedDisplay(ks, "<C-v>", "$j", "d")
	if bufText(ks) != "\n\n" {
		t.Fatalf("block $ then j delete: got %q", bufText(ks))
	}
}

func TestBlockD(t *testing.T) {
	ks := newVimState("abcd\nabcd\n")

	// D deletes to end of line on every block line.
	feedDisplay(ks, "l", "<C-v>", "j", "D")
	if bufText(ks) != "a\na\n" {
		t.Fatalf("block D: got %q", bufText(ks))
	}
}

func TestBlockReplace(t *testing.T) {
	ks := newVimState("abc\nabc\n")

	feedDisplay(ks, "<C-v>", "jl", "rz")
	if bufText(ks) != "zzc\nzzc\n" {
		t.Fatalf("block r: got %q", bufText(ks))
	}
	if cursorPos(ks) != 0 {
		t.Fatalf("cursor after block r: got %d, want 0", cursorPos(ks))
	}
}

func TestBlockCase(t *testing.T) {
	ks := newVimState("abc\nabc\n")

	feedDisplay(ks, "<C-v>", "jl", "U")
	if bufText(ks) != "ABc\nABc\n" {
		t.Fatalf("block U: got %q", bufText(ks))
	}

	ks2 := newVimState("aBc\n")
	feedDisplay(ks2, "<C-v>", "l", "~")
	if bufText(ks2) != "Abc\n" {
		t.Fatalf("block ~: got %q", bufText(ks2))
	}
}

func TestBlockCornerSwaps(t *testing.T) {
	// o: diagonally opposite corner; the rectangle is unchanged.
	ks := newVimState("abcd\nefgh\n")
	feedDisplay(ks, "l", "<C-v>", "jl", "o")
	if cursorPos(ks) != 1 {
		t.Fatalf("o: cursor at %d, want 1 (top-left)", cursorPos(ks))
	}
	feedDisplay(ks, "d")
	if bufText(ks) != "ad\neh\n" {
		t.Fatalf("delete after o: got %q", bufText(ks))
	}

	// O: other horizontal corner on the same line.
	ks = newVimState("abcd\nefgh\n")
	feedDisplay(ks, "l", "<C-v>", "jl", "O")
	if cursorPos(ks) != 6 {
		t.Fatalf("O: cursor at %d, want 6 (bottom-left)", cursorPos(ks))
	}
	feedDisplay(ks, "d")
	if bufText(ks) != "ad\neh\n" {
		t.Fatalf("delete after O: got %q", bufText(ks))
	}
}

func TestBlockModeSwitching(t *testing.T) {
	ks := newVimState("abc\nabc\n")

	feedDisplay(ks, "v", "<C-v>")
	if ks.ModeID() != ModeVisualBlock || !ks.Buf().Cursor().BlockSel {
		t.Fatalf("v then C-v: mode=%v block=%v", ks.ModeID(), ks.Buf().Cursor().BlockSel)
	}
	feedDisplay(ks, "v")
	if ks.ModeID() != ModeVisual || ks.Buf().Cursor().BlockSel {
		t.Fatalf("C-v then v: mode=%v block=%v", ks.ModeID(), ks.Buf().Cursor().BlockSel)
	}
	feedDisplay(ks, "<C-v>", "V")
	if ks.ModeID() != ModeVisualLine || ks.Buf().Cursor().BlockSel {
		t.Fatalf("C-v then V: mode=%v block=%v", ks.ModeID(), ks.Buf().Cursor().BlockSel)
	}
	feedDisplay(ks, "<C-v>")
	if ks.ModeID() != ModeVisualBlock {
		t.Fatalf("V then C-v: mode=%v", ks.ModeID())
	}
}

func TestBlockSwitchToCharwiseDelete(t *testing.T) {
	// Switching from block to charwise keeps the corners as a byte range.
	ks := newVimState("abcd\nefgh\n")

	feedDisplay(ks, "l", "<C-v>", "jl", "v", "d")
	// Charwise selection from byte 1 through byte 7 inclusive.
	if bufText(ks) != "ah\n" {
		t.Fatalf("block->charwise delete: got %q", bufText(ks))
	}
}

func TestBlockIndent(t *testing.T) {
	ks := newVimState("ab\ncd\n")

	feedDisplay(ks, "<C-v>", "j", ">")
	if bufText(ks) != "\tab\n\tcd\n" {
		t.Fatalf("block >: got %q", bufText(ks))
	}
}

func TestBlockUndo(t *testing.T) {
	ks := newVimState("abc\ndef\n")

	feedDisplay(ks, "<C-v>", "jl", "d")
	if bufText(ks) != "c\nf\n" {
		t.Fatalf("block delete: got %q", bufText(ks))
	}
	feedDisplay(ks, "u")
	if bufText(ks) != "abc\ndef\n" {
		t.Fatalf("undo after block delete: got %q", bufText(ks))
	}
}

func TestBlockDotRepeat(t *testing.T) {
	ks := newVimState("xab\nxab\n")

	feedDisplay(ks, "l", "<C-v>", "j", "d")
	if bufText(ks) != "xb\nxb\n" {
		t.Fatalf("block delete: got %q", bufText(ks))
	}
	feedDisplay(ks, ".")
	if bufText(ks) != "x\nx\n" {
		t.Fatalf("dot repeat of block delete: got %q", bufText(ks))
	}
}

func TestBlockNamedRegister(t *testing.T) {
	ks := newVimState("ab\ncd\n")

	feedDisplay(ks, "\"a", "<C-v>", "j", "y")
	reg := ks.regs.Get(RegisterID('a'))
	if !reg.Block || string(reg.Content) != "a\nc" {
		t.Fatalf("named block register: block=%v content=%q", reg.Block, reg.Content)
	}
	feedDisplay(ks, "l", "\"a", "p")
	if bufText(ks) != "aba\ncdc\n" {
		t.Fatalf("named block paste: got %q", bufText(ks))
	}
}

func TestBlockVisualPaste(t *testing.T) {
	ks := newVimState("ab\ncd\n")

	// Yank column 0, then paste it over a block selection of column 1.
	feedDisplay(ks, "<C-v>", "j", "y", "l", "<C-v>", "j", "p")
	if bufText(ks) != "aa\ncc\n" {
		t.Fatalf("visual block paste: got %q", bufText(ks))
	}
}

func TestBlockTabPartialCoverage(t *testing.T) {
	// A tab straddling the block's edge is replaced by spaces for its
	// uncovered cells. Requires a Visualizer for tab-aware columns.
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte("\tabc\nvwxyz\n"))
	NewView(b, 4) // attaches a Visualizer with tabsize 4
	ks := NewKeyState(b, NewRegisterSet())
	SetupBindings(ks)

	// Cursor to (1,2) 'x', block up to (0, col 4) 'a': columns 2-4.
	feedDisplay(ks, "jll", "<C-v>", "k", "d")
	if bufText(ks) != "  bc\nvw\n" {
		t.Fatalf("partial tab block delete: got %q", bufText(ks))
	}
	reg := ks.regs.Get(RegDefault)
	if string(reg.Content) != "  a\nxyz" {
		t.Fatalf("partial tab register: %q", reg.Content)
	}
}

func TestBlockRenderHighlight(t *testing.T) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte("abcd\nefgh\nijkl\n"))
	v := NewView(b, 4)
	v.Resize(20, 5)
	v.LineNums = false
	v.GutterWidth = 0
	ks := NewKeyState(b, NewRegisterSet())
	SetupBindings(ks)

	// Block over rows 0-1, columns 1-2.
	feedDisplay(ks, "l", "<C-v>", "jl")

	selStyle := DefaultTheme.Default().Add(AttrReverse)
	selected := make(map[[2]int]bool)
	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		if style == selStyle {
			selected[[2]int{x, y}] = true
		}
	}, func(x, y int, main bool) {}, DefaultTheme)

	want := map[[2]int]bool{
		{1, 0}: true, {2, 0}: true,
		{1, 1}: true, {2, 1}: true,
	}
	if len(selected) != len(want) {
		t.Fatalf("selected cells = %v, want %v", selected, want)
	}
	for cell := range want {
		if !selected[cell] {
			t.Fatalf("cell %v not highlighted; got %v", cell, selected)
		}
	}
}

// selectedCells renders the view and returns the cells drawn as selected.
func selectedCells(v *View) map[[2]int]bool {
	selStyle := DefaultTheme.Default().Add(AttrReverse)
	selected := make(map[[2]int]bool)
	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		if style == selStyle {
			selected[[2]int{x, y}] = true
		}
	}, func(x, y int, main bool) {}, DefaultTheme)
	return selected
}

func blockRenderState(t *testing.T, text string) (*Buffer, *View, *KeyState) {
	t.Helper()
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte(text))
	v := NewView(b, 4)
	v.Resize(20, 5)
	v.LineNums = false
	v.GutterWidth = 0
	v.CursorLine = false
	ks := NewKeyState(b, NewRegisterSet())
	SetupBindings(ks)
	return b, v, ks
}

// The block-rectangle flags describe a visual-block selection and must not
// outlive it: left set after block mode, they made the next charwise or
// linewise selection render as a rectangle — a whole-line selection showed
// up as a single character.
func TestSelectionAfterBlockModeRendersWhole(t *testing.T) {
	exits := []struct {
		name string
		keys []string
	}{
		{"escape", []string{KeyEscape}},
		{"ctrl-c", []string{"<C-c>"}},
		{"ctrl-v toggle", []string{"<C-v>"}},
		{"operator", []string{"y"}},
	}
	for _, exit := range exits {
		t.Run(exit.name, func(t *testing.T) {
			b, v, ks := blockRenderState(t, "abcd\nefgh\nijkl\n")

			// Use block mode, then leave it by this route.
			feedDisplay(ks, "l", "<C-v>", "jl")
			feedDisplay(ks, exit.keys...)
			if c := b.Cursor(); c.BlockSel || c.BlockEOL {
				t.Fatalf("block flags outlived the selection: BlockSel=%v BlockEOL=%v", c.BlockSel, c.BlockEOL)
			}

			// A whole line, selected linewise, is highlighted whole.
			*b.Cursor() = b.Cursor().MoveTo(0)
			feedDisplay(ks, "V")
			selected := selectedCells(v)
			for x := 0; x < 4; x++ {
				if !selected[[2]int{x, 0}] {
					t.Fatalf("linewise selection is missing cell %d; got %v", x, selected)
				}
			}
		})
	}
}

// The same leak made a charwise selection spanning two lines render as the
// rectangle between its corners instead of the run of text between them.
func TestCharwiseSelectionAfterBlockMode(t *testing.T) {
	b, v, ks := blockRenderState(t, "abcd\nefgh\nijkl\n")

	// The shape a fresh charwise selection renders with.
	feedDisplay(ks, "v", "jl")
	want := selectedCells(v)
	feedDisplay(ks, KeyEscape)
	if len(want) == 0 {
		t.Fatal("nothing selected")
	}

	// The same selection, made after a block-mode session.
	*b.Cursor() = b.Cursor().MoveTo(0)
	feedDisplay(ks, "<C-v>", "jl", KeyEscape)
	*b.Cursor() = b.Cursor().MoveTo(0)
	feedDisplay(ks, "v", "jl")
	got := selectedCells(v)

	if len(got) != len(want) {
		t.Fatalf("charwise selection after block mode covers %d cells, want %d (%v vs %v)",
			len(got), len(want), got, want)
	}
	for cell := range want {
		if !got[cell] {
			t.Fatalf("cell %v not highlighted after block mode; got %v", cell, got)
		}
	}
}

// ClearSelection is what enforces the invariant, wherever a selection ends.
func TestClearSelectionDropsBlockFlags(t *testing.T) {
	c := Cursor{HasSel: true, BlockSel: true, BlockEOL: true, Sel: [2]int{0, 4}}
	c.ClearSelection()
	if c.HasSel || c.BlockSel || c.BlockEOL {
		t.Fatalf("ClearSelection left %+v", c)
	}

	moved := Cursor{Pos: 0, HasSel: true, BlockSel: true, BlockEOL: true}.MoveTo(3)
	if moved.HasSel || moved.BlockSel || moved.BlockEOL {
		t.Fatalf("MoveTo left a block selection: %+v", moved)
	}
	if moved.Pos != 3 {
		t.Fatalf("MoveTo went to %d, want 3", moved.Pos)
	}
}
