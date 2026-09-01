package main

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// Tests in this file were written to capture bugs found during an audit of the
// vim layer. Each test documents the expected vim behavior.

// --- Motion bugs ---

// The built-in bindings follow vim: 0 moves to column 0 and ^ to the first
// non-blank. (The default init.tcl swaps them via mappings; that behavior is
// tested at the editor level in keymap_test.go.)
func TestVimZeroCaretDefaults(t *testing.T) {
	ks := newVimState("  hello\n")

	feedKeys(ks, "$")
	feedKeys(ks, "^")
	if cursorPos(ks) != 2 {
		t.Fatalf("^: pos=%d, want 2 (first non-blank)", cursorPos(ks))
	}
	feedKeys(ks, "$")
	feedKeys(ks, "0")
	if cursorPos(ks) != 0 {
		t.Fatalf("0: pos=%d, want 0 (column 0)", cursorPos(ks))
	}
	// A count containing 0 is unaffected by the binding: 10l is still a
	// 10-column move, not a move plus beginning-of-line.
	feedKeys(ks, "10l")
	if cursorPos(ks) != 6 {
		t.Fatalf("10l: pos=%d, want 6 (clamped to last char)", cursorPos(ks))
	}
}

// % is an inclusive motion: d% deletes the matching bracket too.
func TestVimDeletePercentInclusive(t *testing.T) {
	ks := newVimState("(foo)x\n")

	feedKeys(ks, "d%")
	if bufText(ks) != "x\n" {
		t.Fatalf("d%%: got %q, want %q", bufText(ks), "x\n")
	}

	// Backward: from the closing bracket.
	ks = newVimState("(foo)x\n")
	feedKeys(ks, "f)")
	feedKeys(ks, "d%")
	if bufText(ks) != "x\n" {
		t.Fatalf("d%% backward: got %q, want %q", bufText(ks), "x\n")
	}
}

// dj/dk/dG/dgg are linewise motions: they delete whole lines.
func TestVimDeleteLinewiseMotions(t *testing.T) {
	ks := newVimState("aa\nbb\ncc\n")
	feedKeys(ks, "dj")
	if bufText(ks) != "cc\n" {
		t.Fatalf("dj: got %q, want %q", bufText(ks), "cc\n")
	}

	ks = newVimState("aa\nbb\ncc\n")
	feedKeys(ks, "j")
	feedKeys(ks, "dk")
	if bufText(ks) != "cc\n" {
		t.Fatalf("dk: got %q, want %q", bufText(ks), "cc\n")
	}

	ks = newVimState("aa\nbb\ncc\n")
	feedKeys(ks, "j")
	feedKeys(ks, "dG")
	if bufText(ks) != "aa\n" {
		t.Fatalf("dG: got %q, want %q", bufText(ks), "aa\n")
	}

	ks = newVimState("aa\nbb\ncc\n")
	feedKeys(ks, "j")
	feedKeys(ks, "dgg")
	if bufText(ks) != "cc\n" {
		t.Fatalf("dgg: got %q, want %q", bufText(ks), "cc\n")
	}
}

// --- Count handling ---

// Counts before an operator and before its motion multiply: 2d3w = 6 words.
func TestVimCountMultiplication(t *testing.T) {
	ks := newVimState("w1 w2 w3 w4 w5 w6 w7 w8\n")

	feedKeys(ks, "2d3w")
	if bufText(ks) != "w7 w8\n" {
		t.Fatalf("2d3w: got %q, want %q", bufText(ks), "w7 w8\n")
	}
}

// d2d behaves like 2dd.
func TestVimCountAfterOperatorLinewise(t *testing.T) {
	ks := newVimState("aa\nbb\ncc\n")

	feedKeys(ks, "d2d")
	if bufText(ks) != "cc\n" {
		t.Fatalf("d2d: got %q, want %q", bufText(ks), "cc\n")
	}
}

// --- Dot repeat ---

// A `.` repeat must not leak into the recording of the next action.
// Sequence: x (delete 1), 3. (delete 3), x (delete 1), . (repeat x → delete 1).
func TestVimDotRepeatRecordingNotCorrupted(t *testing.T) {
	ks := newVimState("abcdefghij\n")

	feedKeys(ks, "x")  // "bcdefghij"
	feedKeys(ks, "3.") // "efghij"
	if bufText(ks) != "efghij\n" {
		t.Fatalf("x3.: got %q, want %q", bufText(ks), "efghij\n")
	}
	feedKeys(ks, "x") // "fghij"
	if bufText(ks) != "fghij\n" {
		t.Fatalf("x3.x: got %q, want %q", bufText(ks), "fghij\n")
	}
	feedKeys(ks, ".") // repeats the single x → "ghij"
	if bufText(ks) != "ghij\n" {
		t.Fatalf("x3.x.: got %q, want %q", bufText(ks), "ghij\n")
	}
}

// --- Text objects ---

// iw on a symbol selects just the symbol run, not the surrounding words.
func TestVimInnerWordOnSymbol(t *testing.T) {
	ks := newVimState("a+b\n")

	feedKeys(ks, "l") // on '+'
	feedKeys(ks, "diw")
	if bufText(ks) != "ab\n" {
		t.Fatalf("diw on symbol: got %q, want %q", bufText(ks), "ab\n")
	}
}

// iw on whitespace selects the whitespace run.
func TestVimInnerWordOnWhitespace(t *testing.T) {
	ks := newVimState("a  b\n")

	feedKeys(ks, "l") // on first space
	feedKeys(ks, "diw")
	if bufText(ks) != "ab\n" {
		t.Fatalf("diw on whitespace: got %q, want %q", bufText(ks), "ab\n")
	}
}

// di" works with the cursor on the opening quote.
func TestVimInnerQuoteOnOpeningQuote(t *testing.T) {
	ks := newVimState("say \"hi\" ok\n")

	feedKeys(ks, "f\"") // on opening quote (pos 4)
	feedKeys(ks, "di\"")
	if bufText(ks) != "say \"\" ok\n" {
		t.Fatalf("di\" on opening quote: got %q, want %q", bufText(ks), "say \"\" ok\n")
	}
}

// Quotes pair up from the start of the line: with the cursor on the third
// quote of `"a" "b"`, di" must operate on the second pair.
func TestVimInnerQuotePairing(t *testing.T) {
	ks := newVimState("\"a\" \"b\"\n")

	feedKeys(ks, "4l") // on quote at index 4 (opener of second pair)
	feedKeys(ks, "di\"")
	if bufText(ks) != "\"a\" \"\"\n" {
		t.Fatalf("di\" pairing: got %q, want %q", bufText(ks), "\"a\" \"\"\n")
	}
}

// --- Visual mode ---

// v$ selects to the last character of the line, not the newline:
// v$d must not join lines.
func TestVimVisualDollarExcludesNewline(t *testing.T) {
	ks := newVimState("hello\nworld\n")

	feedKeys(ks, "v$d")
	if bufText(ks) != "\nworld\n" {
		t.Fatalf("v$d: got %q, want %q", bufText(ks), "\nworld\n")
	}
}

// u/U case operators work in visual-line mode too.
func TestVimVisualLineUppercase(t *testing.T) {
	ks := newVimState("abc\ndef\n")

	feedKeys(ks, "VU")
	if bufText(ks) != "ABC\ndef\n" {
		t.Fatalf("VU: got %q, want %q", bufText(ks), "ABC\ndef\n")
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be back in normal mode")
	}
}

// A register prefix works in visual mode: viw"ay yanks into register a.
func TestVimRegisterInVisualMode(t *testing.T) {
	ks := newVimState("hello world\n")

	feedKeys(ks, "vll\"ay")
	reg := ks.regs.Get('a')
	if string(reg.Content) != "hel" {
		t.Fatalf("visual \"ay: register a=%q, want %q", reg.Content, "hel")
	}
}

// --- Linewise register semantics at EOF ---

// dd on the last line of a file without a trailing newline removes the
// preceding newline and yields a linewise register, so p pastes it back
// as a full line.
func TestVimDeleteLastLineNoTrailingNewline(t *testing.T) {
	ks := newVimState("aa\nbb")

	feedKeys(ks, "j")
	feedKeys(ks, "dd")
	if bufText(ks) != "aa" {
		t.Fatalf("dd last line: got %q, want %q", bufText(ks), "aa")
	}
	reg := ks.regs.Get(RegDefault)
	if !reg.Linewise {
		t.Fatalf("dd last line: register not linewise (content %q)", reg.Content)
	}
	feedKeys(ks, "p")
	if bufText(ks) != "aa\nbb" {
		t.Fatalf("dd last line + p: got %q, want %q", bufText(ks), "aa\nbb")
	}
}

// yy on the last line (no trailing newline) is still a linewise yank.
func TestVimYankLastLineNoTrailingNewline(t *testing.T) {
	ks := newVimState("aa\nbb")

	feedKeys(ks, "j")
	feedKeys(ks, "yy")
	reg := ks.regs.Get(RegDefault)
	if !reg.Linewise {
		t.Fatalf("yy last line: register not linewise (content %q)", reg.Content)
	}
	feedKeys(ks, "p")
	if bufText(ks) != "aa\nbb\nbb" {
		t.Fatalf("yy last line + p: got %q, want %q", bufText(ks), "aa\nbb\nbb")
	}
}

// Y is a linewise yank as well.
func TestVimYLinewise(t *testing.T) {
	ks := newVimState("aa\nbb")

	feedKeys(ks, "j")
	feedKeys(ks, "Y")
	reg := ks.regs.Get(RegDefault)
	if !reg.Linewise {
		t.Fatalf("Y last line: register not linewise (content %q)", reg.Content)
	}
}

// Vd on the last line of a file without a trailing newline removes the whole
// line including its preceding newline.
func TestVimVisualLineDeleteLastLine(t *testing.T) {
	ks := newVimState("aa\nbb")

	feedKeys(ks, "j")
	feedKeys(ks, "Vd")
	if bufText(ks) != "aa" {
		t.Fatalf("Vd last line: got %q, want %q", bufText(ks), "aa")
	}
	reg := ks.regs.Get(RegDefault)
	if !reg.Linewise {
		t.Fatalf("Vd last line: register not linewise (content %q)", reg.Content)
	}
}

// --- J join count ---

// 3J joins 3 lines into one (2 joins), not 3 joins.
func TestVimJoinCount(t *testing.T) {
	ks := newVimState("a\nb\nc\nd\n")

	feedKeys(ks, "3J")
	if bufText(ks) != "a b c\nd\n" {
		t.Fatalf("3J: got %q, want %q", bufText(ks), "a b c\nd\n")
	}
}

// --- Undo granularity ---

// Text typed in an insert session forms its own undo event: after
// `x` then `iabc<Esc>`, one undo removes only the inserted text.
func TestVimUndoSeparatesInsertFromPrecedingEdit(t *testing.T) {
	ks := newVimState("hello\n")

	feedKeys(ks, "x") // "ello"
	feedKeys(ks, "iabc")
	feedSpecial(ks, KeyEscape) // "abcello"
	if bufText(ks) != "abcello\n" {
		t.Fatalf("x iabc: got %q, want %q", bufText(ks), "abcello\n")
	}
	feedKeys(ks, "u")
	if bufText(ks) != "ello\n" {
		t.Fatalf("first undo: got %q, want %q", bufText(ks), "ello\n")
	}
	feedKeys(ks, "u")
	if bufText(ks) != "hello\n" {
		t.Fatalf("second undo: got %q, want %q", bufText(ks), "hello\n")
	}
}

// The whitespace cleanup performed when leaving insert mode coalesces with
// the insert event: a single undo reverts the whole o+typing session.
func TestVimUndoOEscapeSingleEvent(t *testing.T) {
	ks := newVimState("ab\n")

	feedKeys(ks, "o ") // open line below, type a space
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "ab\n\n" {
		t.Fatalf("o<space><Esc>: got %q, want %q", bufText(ks), "ab\n\n")
	}
	feedKeys(ks, "u")
	if bufText(ks) != "ab\n" {
		t.Fatalf("undo after o<space><Esc>: got %q, want %q", bufText(ks), "ab\n")
	}
}

// --- Special keys as character arguments ---

// r<Esc> cancels instead of inserting the literal string "<Esc>".
func TestVimReplaceCharEscapeCancels(t *testing.T) {
	ks := newVimState("hello\n")

	feedKeys(ks, "r")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "hello\n" {
		t.Fatalf("r<Esc>: got %q, want %q", bufText(ks), "hello\n")
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be in normal mode")
	}
}

// f<Esc> cancels instead of searching for '<'.
func TestVimFindCharEscapeCancels(t *testing.T) {
	ks := newVimState("a<b\n")

	feedKeys(ks, "f")
	feedSpecial(ks, KeyEscape)
	if cursorPos(ks) != 0 {
		t.Fatalf("f<Esc>: pos=%d, want 0", cursorPos(ks))
	}
}

// r<CR> replaces the character with a newline.
func TestVimReplaceCharWithEnter(t *testing.T) {
	ks := newVimState("ab\n")

	feedKeys(ks, "l")
	feedKeys(ks, "r")
	feedSpecial(ks, KeyEnter)
	if bufText(ks) != "a\n\n" {
		t.Fatalf("r<CR>: got %q, want %q", bufText(ks), "a\n\n")
	}
}

// Leaving replace mode puts the cursor on the last character replaced, not
// past it, exactly as leaving insert mode does (vim).
func TestReplaceModeExitStepsLeft(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		start   int
		typed   string
		wantBuf string
		wantPos int
	}{
		{"after replacing", "abcdef\n", 0, "xyz", "xyzdef\n", 2},
		{"one character", "abcdef\n", 2, "X", "abXdef\n", 2},
		{"nothing typed", "abcdef\n", 3, "", "abcdef\n", 2},
		{"replacing to the end", "abc\n", 0, "XYZ", "XYZ\n", 2},
		{"past the end of the line", "abc\n", 0, "XYZW", "XYZW\n", 3},
		// No character of its own line to step back onto.
		{"at the start of a line", "abc\n", 0, "", "abc\n", 0},
		{"on an empty line", "\nabc\n", 0, "", "\nabc\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ks := newVimState(tt.text)
			b := ks.Buf()
			*b.Cursor() = b.Cursor().MoveTo(tt.start)

			feedKeys(ks, "R")
			if ks.ModeID() != ModeReplace {
				t.Fatalf("R did not enter replace mode: %v", ks.ModeID())
			}
			if tt.typed != "" {
				feedKeys(ks, tt.typed)
			}
			feedSpecial(ks, KeyEscape)

			if ks.ModeID() != ModeNormal {
				t.Fatalf("mode after escape = %v, want normal", ks.ModeID())
			}
			if got := string(b.text.Slice(0, b.Len())); got != tt.wantBuf {
				t.Fatalf("text = %q, want %q", got, tt.wantBuf)
			}
			if got := b.Cursor().Pos; got != tt.wantPos {
				t.Fatalf("cursor at %d, want %d", got, tt.wantPos)
			}
		})
	}
}

// <C-c> leaves replace mode the same way Escape does, cursor included.
func TestReplaceModeCtrlCStepsLeft(t *testing.T) {
	ks := newVimState("abcdef\n")
	feedKeys(ks, "R")
	feedKeys(ks, "xy")
	feedSpecial(ks, "<C-c>")

	b := ks.Buf()
	if got := string(b.text.Slice(0, b.Len())); got != "xycdef\n" {
		t.Fatalf("text = %q, want %q", got, "xycdef\n")
	}
	if got := b.Cursor().Pos; got != 1 {
		t.Fatalf("cursor at %d, want 1", got)
	}
}

// Replace mode is shown with an underline, under the character it is about
// to overwrite — not insert mode's between-characters bar.
func TestCursorStyleForMode(t *testing.T) {
	tests := []struct {
		mode ModeID
		want tcell.CursorStyle
	}{
		{ModeReplace, tcell.CursorStyleSteadyUnderline},
		{ModeInsert, tcell.CursorStyleSteadyBar},
		{ModeNormal, tcell.CursorStyleSteadyBlock},
		{ModeOperatorPending, tcell.CursorStyleSteadyUnderline},
		{ModeVisual, tcell.CursorStyleSteadyUnderline},
		{ModeVisualLine, tcell.CursorStyleSteadyUnderline},
		{ModeVisualBlock, tcell.CursorStyleSteadyUnderline},
	}
	for _, tt := range tests {
		if got := cursorStyleForMode(tt.mode); got != tt.want {
			t.Errorf("cursorStyleForMode(%v) = %v, want %v", tt.mode, got, tt.want)
		}
	}
}
