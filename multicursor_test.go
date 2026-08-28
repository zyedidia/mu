package main

import (
	"testing"
)

func TestMCStartSelectsWord(t *testing.T) {
	ks := newVimState("foo bar foo\n")

	feedSpecial(ks, "<C-n>")
	if ks.ModeID() != ModeVisual {
		t.Fatalf("mode = %v, want visual", ks.ModeID())
	}
	c := ks.Buf().Cursor()
	if !c.HasSel || c.Sel != [2]int{0, 3} {
		t.Fatalf("selection = %v, want [0,3]", c.Sel)
	}
	if ks.mcPattern != `\bfoo\b` {
		t.Fatalf("pattern = %q, want %q", ks.mcPattern, `\bfoo\b`)
	}
}

func TestMCSpawnNextAndStop(t *testing.T) {
	ks := newVimState("foo bar foo\n")

	feedSpecial(ks, "<C-n>", "<C-n>")
	b := ks.Buf()
	if b.NumCursors() != 2 {
		t.Fatalf("cursors = %d, want 2", b.NumCursors())
	}
	if b.Cursor().Sel != [2]int{8, 11} {
		t.Fatalf("second selection = %v, want [8,11]", b.Cursor().Sel)
	}
	// All occurrences claimed: another <C-n> adds nothing.
	feedSpecial(ks, "<C-n>")
	if b.NumCursors() != 2 {
		t.Fatalf("cursors after full cycle = %d, want 2", b.NumCursors())
	}
}

func TestMCWordBoundary(t *testing.T) {
	// The word pattern is boundary-anchored: "in" does not match inside
	// "int" or "print".
	ks := newVimState("int in print\n")

	feedKeys(ks, "w") // onto "in"
	feedSpecial(ks, "<C-n>")
	if ks.Buf().Cursor().Sel != [2]int{4, 6} {
		t.Fatalf("selection = %v, want [4,6]", ks.Buf().Cursor().Sel)
	}
	feedSpecial(ks, "<C-n>")
	if ks.Buf().NumCursors() != 1 {
		t.Fatalf("cursors = %d, want 1 (no other whole-word match)", ks.Buf().NumCursors())
	}
}

func TestMCSkip(t *testing.T) {
	ks := newVimState("aa aa aa\n")

	feedSpecial(ks, "<C-n>", "<C-x>")
	b := ks.Buf()
	if b.NumCursors() != 1 {
		t.Fatalf("cursors after skip = %d, want 1", b.NumCursors())
	}
	if b.Cursor().Sel != [2]int{3, 5} {
		t.Fatalf("selection after skip = %v, want [3,5]", b.Cursor().Sel)
	}
	feedSpecial(ks, "<C-n>")
	if b.NumCursors() != 2 {
		t.Fatalf("cursors = %d, want 2", b.NumCursors())
	}
	feedKeys(ks, "d")
	if bufText(ks) != "aa  \n" {
		t.Fatalf("delete after skip: got %q", bufText(ks))
	}
}

func TestMCPop(t *testing.T) {
	ks := newVimState("x x x\n")

	feedSpecial(ks, "<C-n>", "<C-n>", "<C-n>")
	b := ks.Buf()
	if b.NumCursors() != 3 {
		t.Fatalf("cursors = %d, want 3", b.NumCursors())
	}
	feedSpecial(ks, "<C-p>")
	if b.NumCursors() != 2 {
		t.Fatalf("cursors after <C-p> = %d, want 2", b.NumCursors())
	}
	if b.Cursor().Sel != [2]int{2, 3} {
		t.Fatalf("active selection = %v, want [2,3]", b.Cursor().Sel)
	}
}

func TestMCChangeAll(t *testing.T) {
	// The flagship flow: select occurrences, change them all at once.
	ks := newVimState("foo bar foo\n")

	feedSpecial(ks, "<C-n>", "<C-n>")
	feedKeys(ks, "c")
	if ks.ModeID() != ModeInsert {
		t.Fatal("c should enter insert mode")
	}
	feedKeys(ks, "new")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "new bar new\n" {
		t.Fatalf("multi-cursor change: got %q", bufText(ks))
	}
	// The cursors survive the edit for further work...
	if ks.Buf().NumCursors() != 2 {
		t.Fatalf("cursors after change = %d, want 2", ks.Buf().NumCursors())
	}
	// ...and Escape ends the session.
	feedSpecial(ks, KeyEscape)
	if ks.Buf().NumCursors() != 1 {
		t.Fatalf("cursors after escape = %d, want 1", ks.Buf().NumCursors())
	}
}

func TestMCDeleteAll(t *testing.T) {
	ks := newVimState("foo bar foo baz foo\n")

	feedSpecial(ks, "<C-n>", "<C-n>", "<C-n>")
	feedKeys(ks, "x")
	if bufText(ks) != " bar  baz \n" {
		t.Fatalf("multi-cursor x: got %q", bufText(ks))
	}
}

func TestMCVisualAdopt(t *testing.T) {
	// A manual visual selection becomes the (literal) pattern.
	ks := newVimState("ab cd ab\n")

	feedKeys(ks, "vl") // select "ab"
	feedSpecial(ks, "<C-n>")
	b := ks.Buf()
	if b.NumCursors() != 2 {
		t.Fatalf("cursors = %d, want 2", b.NumCursors())
	}
	if b.Cursor().Sel != [2]int{6, 8} {
		t.Fatalf("spawned selection = %v, want [6,8]", b.Cursor().Sel)
	}
}

func TestMCMergeOnMotion(t *testing.T) {
	// Cursors that converge onto the same position merge into one.
	ks := newVimState("x. x.\n")

	feedSpecial(ks, "<C-n>", "<C-n>") // the two "x" words
	feedKeys(ks, "d")                 // normal mode, 2 cursors at distinct positions
	if bufText(ks) != ". .\n" {
		t.Fatalf("multi-cursor d: got %q", bufText(ks))
	}
	if ks.Buf().NumCursors() != 2 {
		t.Fatalf("cursors = %d, want 2", ks.Buf().NumCursors())
	}
	feedKeys(ks, "^") // both move to column 0
	if ks.Buf().NumCursors() != 1 {
		t.Fatalf("cursors after convergence = %d, want 1", ks.Buf().NumCursors())
	}
}

func TestMCRestartFresh(t *testing.T) {
	// <C-n> in normal mode with cursors left over starts a fresh flow.
	ks := newVimState("aa bb aa\n")

	feedSpecial(ks, "<C-n>", "<C-n>")
	feedKeys(ks, "d") // 2 cursors in normal mode
	feedKeys(ks, "gg")
	feedSpecial(ks, "<C-n>")
	b := ks.Buf()
	if b.NumCursors() != 1 {
		t.Fatalf("cursors after restart = %d, want 1", b.NumCursors())
	}
	if ks.ModeID() != ModeVisual {
		t.Fatalf("mode = %v, want visual", ks.ModeID())
	}
}

func TestMCInsertAtCursors(t *testing.T) {
	// A one-char manual selection adopted as the pattern, changed at both
	// cursors.
	ks := newVimState("a1 a2\n")

	feedKeys(ks, "v") // select "a" only
	feedSpecial(ks, "<C-n>")
	if ks.Buf().NumCursors() != 2 {
		t.Fatalf("cursors = %d, want 2", ks.Buf().NumCursors())
	}
	feedKeys(ks, "c")
	feedKeys(ks, "b")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "b1 b2\n" {
		t.Fatalf("adopted single-char change: got %q", bufText(ks))
	}
}

func TestMCAppendAll(t *testing.T) {
	// a moves every cursor right, not just the primary.
	ks := newVimState("foo bar foo\n")
	ks.Buf().SpawnCursor(8) // second cursor on the second "foo"
	feedKeys(ks, "aX")
	feedSpecial(ks, KeyEscape)
	if got := bufText(ks); got != "fXoo bar fXoo\n" {
		t.Fatalf("aX at all cursors: got %q", got)
	}
}

func TestMCLineInsertAll(t *testing.T) {
	// A appends at every cursor's line end; I inserts at every cursor's
	// first non-blank.
	ks := newVimState("  foo x\n  foo y\n")
	feedKeys(ks, "w") // onto the first "foo"
	feedSpecial(ks, "<C-n>", "<C-n>")
	feedKeys(ks, "y")
	feedKeys(ks, "A!")
	feedSpecial(ks, KeyEscape)
	if got := bufText(ks); got != "  foo x!\n  foo y!\n" {
		t.Fatalf("A at all cursors: got %q", got)
	}
	feedKeys(ks, "I-")
	feedSpecial(ks, KeyEscape)
	if got := bufText(ks); got != "  -foo x!\n  -foo y!\n" {
		t.Fatalf("I at all cursors: got %q", got)
	}
}

func TestMCOpenBelowAll(t *testing.T) {
	ks := newVimState("foo a\nfoo b\n")
	feedSpecial(ks, "<C-n>", "<C-n>")
	feedKeys(ks, "y")
	feedKeys(ks, "oz")
	feedSpecial(ks, KeyEscape)
	if got := bufText(ks); got != "foo a\nz\nfoo b\nz\n" {
		t.Fatalf("o at all cursors: got %q", got)
	}
}

func TestMCOpenAboveAll(t *testing.T) {
	ks := newVimState("foo a\nfoo b\n")
	feedSpecial(ks, "<C-n>", "<C-n>")
	feedKeys(ks, "y")
	feedKeys(ks, "Oz")
	feedSpecial(ks, KeyEscape)
	if got := bufText(ks); got != "z\nfoo a\nz\nfoo b\n" {
		t.Fatalf("O at all cursors: got %q", got)
	}
}

func TestMCJoinAll(t *testing.T) {
	ks := newVimState("foo\nb\nfoo\nd\n")
	feedSpecial(ks, "<C-n>", "<C-n>")
	feedKeys(ks, "y")
	feedKeys(ks, "J")
	if got := bufText(ks); got != "foo b\nfoo d\n" {
		t.Fatalf("J at all cursors: got %q", got)
	}
}

func TestMCInsertEscapeMovesAllLeft(t *testing.T) {
	// Leaving insert mode steps every cursor left, so each ends on the
	// last inserted character.
	ks := newVimState("foo bar foo\n")
	feedSpecial(ks, "<C-n>", "<C-n>")
	feedKeys(ks, "cxyz")
	feedSpecial(ks, KeyEscape)
	b := ks.Buf()
	if got := bufText(ks); got != "xyz bar xyz\n" {
		t.Fatalf("change: got %q", got)
	}
	if b.NumCursors() != 2 {
		t.Fatalf("cursors = %d, want 2", b.NumCursors())
	}
	if b.cursors[0].Pos != 2 || b.cursors[1].Pos != 10 {
		t.Fatalf("cursor positions = %d,%d, want 2,10 (on the z's)",
			b.cursors[0].Pos, b.cursors[1].Pos)
	}
}

func TestReplaceModeCtrlC(t *testing.T) {
	// <C-c> leaves replace mode like Escape.
	ks := newVimState("abc\n")
	feedKeys(ks, "R")
	if ks.ModeID() != ModeReplace {
		t.Fatalf("mode = %v, want replace", ks.ModeID())
	}
	feedSpecial(ks, "<C-c>")
	if ks.ModeID() != ModeNormal {
		t.Fatalf("mode after C-c = %v, want normal", ks.ModeID())
	}
}

// Leaving the multi-cursor visual flow is a two-stage exit, for Escape and
// <C-c> alike: the first press leaves visual mode with the cursors intact,
// so they can be typed at in normal mode, and only a second press collapses
// them to the primary cursor.
func TestMCExitKeysTwoStage(t *testing.T) {
	for _, key := range []string{KeyEscape, "<C-c>"} {
		t.Run(key, func(t *testing.T) {
			ks := newVimState("foo bar foo baz foo\n")
			feedSpecial(ks, "<C-n>", "<C-n>", "<C-n>")
			b := ks.Buf()
			if b.NumCursors() != 3 || ks.ModeID() != ModeVisual {
				t.Fatalf("setup: %d cursors, mode %v", b.NumCursors(), ks.ModeID())
			}

			feedSpecial(ks, key)
			if ks.ModeID() != ModeNormal {
				t.Fatalf("first press: mode %v, want normal", ks.ModeID())
			}
			if b.NumCursors() != 3 {
				t.Fatalf("first press: %d cursors, want the 3 to survive", b.NumCursors())
			}
			for i := 0; i < b.NumCursors(); i++ {
				if b.cursors[i].HasSel {
					t.Errorf("cursor %d still has a selection", i)
				}
			}
			// The pattern is dropped, so a later <C-n> adopts the new
			// selection rather than continuing this one.
			if ks.mcPattern != "" {
				t.Errorf("pattern = %q, want it cleared", ks.mcPattern)
			}

			// The surviving cursors are usable: an edit hits all three.
			feedKeys(ks, "x")
			if got := bufText(ks); got != "fo bar fo baz fo\n" {
				t.Fatalf("x after exit = %q, want %q", got, "fo bar fo baz fo\n")
			}

			feedSpecial(ks, key)
			if b.NumCursors() != 1 || ks.ModeID() != ModeNormal {
				t.Fatalf("second press: %d cursors, mode %v", b.NumCursors(), ks.ModeID())
			}
		})
	}
}

// Cursors left over from an operator that ended in normal mode collapse on
// a single press, since there is no visual mode to leave first.
func TestMCExitKeysCollapseLeftoverCursors(t *testing.T) {
	for _, key := range []string{KeyEscape, "<C-c>"} {
		t.Run(key, func(t *testing.T) {
			ks := newVimState("foo bar foo\n")
			feedSpecial(ks, "<C-n>", "<C-n>")
			feedKeys(ks, "y") // yank returns to normal with 2 cursors
			b := ks.Buf()
			if b.NumCursors() != 2 {
				t.Fatalf("cursors after y = %d, want 2", b.NumCursors())
			}
			feedSpecial(ks, key)
			if b.NumCursors() != 1 {
				t.Fatalf("cursors after exit = %d, want 1", b.NumCursors())
			}
			if ks.mcPattern != "" {
				t.Errorf("pattern = %q, want it cleared", ks.mcPattern)
			}
		})
	}
}

// An ordinary single-cursor visual selection is unaffected: one press is
// still the whole exit, since there are no extra cursors to keep.
func TestExitKeysFromPlainVisual(t *testing.T) {
	for _, key := range []string{KeyEscape, "<C-c>"} {
		for _, enter := range []string{"v", "V", "<C-v>"} {
			ks := newVimState("hello world\n")
			if enter == "<C-v>" {
				feedSpecial(ks, "<C-v>")
			} else {
				feedKeys(ks, enter)
			}
			feedKeys(ks, "ll")
			feedSpecial(ks, key)
			b := ks.Buf()
			if ks.ModeID() != ModeNormal || b.NumCursors() != 1 || b.Cursor().HasSel {
				t.Errorf("%s then %s: mode %v, %d cursors, hassel %v",
					enter, key, ks.ModeID(), b.NumCursors(), b.Cursor().HasSel)
			}
		}
	}
}
