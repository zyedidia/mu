package main

import (
	"testing"
)

// --- v_x / v_s ---

func TestVisualX(t *testing.T) {
	ks := newVimState("hello world\n")
	feedKeys(ks, "vllllx")
	if bufText(ks) != " world\n" {
		t.Fatalf("v x: got %q", bufText(ks))
	}
	// Like d, x fills the register.
	if got := string(ks.regs.Get(RegDefault).Content); got != "hello" {
		t.Fatalf("register after v x: %q", got)
	}

	ks2 := newVimState("aa\nbb\ncc\n")
	feedDisplay(ks2, "V", "j", "x")
	if bufText(ks2) != "cc\n" {
		t.Fatalf("V x: got %q", bufText(ks2))
	}
}

func TestVisualS(t *testing.T) {
	ks := newVimState("hello world\n")
	feedKeys(ks, "vlllls")
	if ks.ModeID() != ModeInsert {
		t.Fatal("v s should enter insert mode")
	}
	feedKeys(ks, "bye")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "bye world\n" {
		t.Fatalf("v s: got %q", bufText(ks))
	}

	// Linewise: the line's contents are replaced on a fresh line.
	ks2 := newVimState("aa\nbb\n")
	feedDisplay(ks2, "V", "s")
	feedKeys(ks2, "new")
	feedSpecial(ks2, KeyEscape)
	if bufText(ks2) != "new\nbb\n" {
		t.Fatalf("V s: got %q", bufText(ks2))
	}
}

// --- v_~ ---

func TestVisualTilde(t *testing.T) {
	ks := newVimState("aBc def\n")
	feedKeys(ks, "vll~")
	if bufText(ks) != "AbC def\n" {
		t.Fatalf("v ~: got %q", bufText(ks))
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be back in normal mode")
	}

	ks2 := newVimState("aBc\ndEf\n")
	feedDisplay(ks2, "V", "j", "~")
	if bufText(ks2) != "AbC\nDeF\n" {
		t.Fatalf("V ~: got %q", bufText(ks2))
	}
}

// --- v_J ---

func TestVisualJoin(t *testing.T) {
	ks := newVimState("aa\nbb\ncc\ndd\n")
	feedDisplay(ks, "V", "jj", "J")
	if bufText(ks) != "aa bb cc\ndd\n" {
		t.Fatalf("Vjj J: got %q", bufText(ks))
	}
	// Cursor on the last inserted space.
	if cursorPos(ks) != 5 {
		t.Fatalf("cursor after join: got %d, want 5", cursorPos(ks))
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be back in normal mode")
	}
}

func TestVisualJoinSingleLine(t *testing.T) {
	// A selection within one line joins it with the next, like normal J.
	ks := newVimState("aa\nbb\n")
	feedKeys(ks, "vlJ")
	if bufText(ks) != "aa bb\n" {
		t.Fatalf("v J single line: got %q", bufText(ks))
	}
}

func TestVisualJoinLeadingWhitespace(t *testing.T) {
	ks := newVimState("aa\n   bb\n")
	feedDisplay(ks, "V", "j", "J")
	if bufText(ks) != "aa bb\n" {
		t.Fatalf("join with indented line: got %q", bufText(ks))
	}
}

func TestVisualJoinCharwise(t *testing.T) {
	// A charwise selection spanning lines joins those lines.
	ks := newVimState("aa\nbb\ncc\n")
	feedDisplay(ks, "v", "j", "J")
	if bufText(ks) != "aa bb\ncc\n" {
		t.Fatalf("vj J: got %q", bufText(ks))
	}
}

func TestVisualBlockJoin(t *testing.T) {
	ks := newVimState("aa\nbb\ncc\n")
	feedDisplay(ks, "<C-v>", "j", "J")
	if bufText(ks) != "aa bb\ncc\n" {
		t.Fatalf("block J: got %q", bufText(ks))
	}
}

func TestVisualJoinUndoOneStep(t *testing.T) {
	ks := newVimState("aa\nbb\ncc\n")
	feedDisplay(ks, "V", "jj", "J")
	feedKeys(ks, "u")
	if bufText(ks) != "aa\nbb\ncc\n" {
		t.Fatalf("undo after visual join: got %q", bufText(ks))
	}
}

// --- v_r ---

func TestVisualReplace(t *testing.T) {
	ks := newVimState("hello world\n")
	feedKeys(ks, "vllllrx")
	if bufText(ks) != "xxxxx world\n" {
		t.Fatalf("v r: got %q", bufText(ks))
	}
	if cursorPos(ks) != 0 {
		t.Fatalf("cursor after v r: got %d, want 0 (selection start)", cursorPos(ks))
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be back in normal mode")
	}
}

func TestVisualReplaceMultiline(t *testing.T) {
	// Newlines inside the selection are preserved.
	ks := newVimState("ab\ncd\n")
	feedDisplay(ks, "v", "jl", "rz")
	if bufText(ks) != "zz\nzz\n" {
		t.Fatalf("multiline v r: got %q", bufText(ks))
	}

	ks2 := newVimState("ab\ncd\nef\n")
	feedDisplay(ks2, "V", "j", "r-")
	if bufText(ks2) != "--\n--\nef\n" {
		t.Fatalf("V r: got %q", bufText(ks2))
	}
}

func TestVisualReplaceEscapeCancels(t *testing.T) {
	ks := newVimState("abc\n")
	feedKeys(ks, "vlr")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "abc\n" {
		t.Fatalf("v r <Esc>: got %q", bufText(ks))
	}
}

func TestVisualReplaceUndoOneStep(t *testing.T) {
	ks := newVimState("ab\ncd\n")
	feedDisplay(ks, "V", "j", "rz")
	feedKeys(ks, "u")
	if bufText(ks) != "ab\ncd\n" {
		t.Fatalf("undo after v r: got %q", bufText(ks))
	}
}
