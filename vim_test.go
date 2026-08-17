package main

import (
	"testing"
)

// newVimState creates a KeyState with all bindings and a buffer containing text.
func newVimState(text string) *KeyState {
	b := NewEmptyBuffer()
	if len(text) > 0 {
		b.text.Insert(0, []byte(text))
	}
	*b.Cursor() = b.Cursor().MoveTo(0)
	ks := NewKeyState(b, NewRegisterSet())
	SetupBindings(ks)
	return ks
}

func feedKeys(ks *KeyState, keys string) {
	for _, ch := range keys {
		ks.HandleKey(string(ch))
	}
}

func feedSpecial(ks *KeyState, keys ...string) {
	for _, k := range keys {
		ks.HandleKey(k)
	}
}

func bufText(ks *KeyState) string {
	b := ks.Buf()
	return string(b.Slice(0, b.Len()))
}

func cursorPos(ks *KeyState) int {
	return ks.Buf().Cursor().Pos
}

// --- Motion tests ---

func TestVimMotionHJKL(t *testing.T) {
	ks := newVimState("hello\nworld\n")

	feedKeys(ks, "l")
	if cursorPos(ks) != 1 {
		t.Fatalf("l: pos=%d, want 1", cursorPos(ks))
	}
	feedKeys(ks, "ll")
	if cursorPos(ks) != 3 {
		t.Fatalf("lll: pos=%d, want 3", cursorPos(ks))
	}
	feedKeys(ks, "h")
	if cursorPos(ks) != 2 {
		t.Fatalf("h: pos=%d, want 2", cursorPos(ks))
	}
	// Go back to start.
	feedKeys(ks, "0")
	feedKeys(ks, "j")
	line, _ := ks.Buf().LineColAt(cursorPos(ks))
	if line != 1 {
		t.Fatalf("j: line=%d, want 1", line)
	}
	feedKeys(ks, "k")
	line, _ = ks.Buf().LineColAt(cursorPos(ks))
	if line != 0 {
		t.Fatalf("k: line=%d, want 0", line)
	}
}

func TestVimMotionWordAndLineEnds(t *testing.T) {
	ks := newVimState("hello world foo\n")

	feedKeys(ks, "w")
	if cursorPos(ks) != 6 {
		t.Fatalf("w: pos=%d, want 6", cursorPos(ks))
	}
	feedKeys(ks, "b")
	if cursorPos(ks) != 0 {
		t.Fatalf("b: pos=%d, want 0", cursorPos(ks))
	}
	feedKeys(ks, "$")
	if cursorPos(ks) != 14 { // on the last char 'o' (vim clamps off the '\n')
		t.Fatalf("$: pos=%d, want 14", cursorPos(ks))
	}
	feedKeys(ks, "0")
	if cursorPos(ks) != 0 {
		t.Fatalf("0: pos=%d, want 0", cursorPos(ks))
	}
}

func TestVimMotionGG(t *testing.T) {
	ks := newVimState("line1\nline2\nline3\n")

	feedKeys(ks, "G")
	if cursorPos(ks) != ks.Buf().Len() {
		t.Fatalf("G: pos=%d, want %d", cursorPos(ks), ks.Buf().Len())
	}
	feedKeys(ks, "g")
	feedKeys(ks, "g")
	if cursorPos(ks) != 0 {
		t.Fatalf("gg: pos=%d, want 0", cursorPos(ks))
	}
}

func TestVimMotionCount(t *testing.T) {
	ks := newVimState("abcdefghij\n")

	feedKeys(ks, "3l")
	if cursorPos(ks) != 3 {
		t.Fatalf("3l: pos=%d, want 3", cursorPos(ks))
	}
}

func TestVimMotionFindChar(t *testing.T) {
	ks := newVimState("hello world\n")

	feedKeys(ks, "fo")
	// should find first 'o' at position 4
	if cursorPos(ks) != 4 {
		t.Fatalf("fo: pos=%d, want 4", cursorPos(ks))
	}
}

// --- Operator tests ---

func TestVimDeleteWord(t *testing.T) {
	ks := newVimState("hello world\n")

	feedKeys(ks, "dw")
	if bufText(ks) != "world\n" {
		t.Fatalf("dw: got %q", bufText(ks))
	}
}

func TestVimDeleteLine(t *testing.T) {
	ks := newVimState("line1\nline2\nline3\n")

	feedKeys(ks, "dd")
	if bufText(ks) != "line2\nline3\n" {
		t.Fatalf("dd: got %q", bufText(ks))
	}
}

func TestVimDeleteToEnd(t *testing.T) {
	ks := newVimState("hello world\n")

	feedKeys(ks, "wD")
	if bufText(ks) != "hello \n" {
		t.Fatalf("wD: got %q", bufText(ks))
	}
}

func TestVimChangeWord(t *testing.T) {
	ks := newVimState("hello world\n")

	// cw acts like ce (vim quirk): changes "hello" not "hello ".
	feedKeys(ks, "cwbye")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "bye world\n" {
		t.Fatalf("cw: got %q", bufText(ks))
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be back in normal mode")
	}
}

func TestVimYankPaste(t *testing.T) {
	ks := newVimState("hello\n")

	feedKeys(ks, "yy")
	feedKeys(ks, "p")
	if bufText(ks) != "hello\nhello\n" {
		t.Fatalf("yyp: got %q", bufText(ks))
	}
}

func TestVimX(t *testing.T) {
	ks := newVimState("hello\n")

	feedKeys(ks, "x")
	if bufText(ks) != "ello\n" {
		t.Fatalf("x: got %q", bufText(ks))
	}
}

func TestVimUndoRedo(t *testing.T) {
	ks := newVimState("hello\n")

	feedKeys(ks, "dd")
	if bufText(ks) != "" {
		t.Fatalf("dd: got %q", bufText(ks))
	}
	feedKeys(ks, "u")
	if bufText(ks) != "hello\n" {
		t.Fatalf("u: got %q", bufText(ks))
	}
	feedSpecial(ks, "<C-r>")
	if bufText(ks) != "" {
		t.Fatalf("C-r: got %q", bufText(ks))
	}
}

// --- Text object tests ---

func TestVimDeleteInnerWord(t *testing.T) {
	ks := newVimState("hello world\n")

	feedKeys(ks, "wdiw")
	if bufText(ks) != "hello \n" {
		t.Fatalf("diw: got %q", bufText(ks))
	}
}

func TestVimDeleteInnerParen(t *testing.T) {
	ks := newVimState("foo(bar)\n")

	feedKeys(ks, "lllldi(")
	if bufText(ks) != "foo()\n" {
		t.Fatalf("di(: got %q", bufText(ks))
	}
}

func TestVimChangeInnerQuote(t *testing.T) {
	ks := newVimState("say \"hello\" ok\n")

	// Position cursor on 'h' inside the quotes (pos 5).
	feedKeys(ks, "fh")
	feedKeys(ks, "ci\"")
	feedKeys(ks, "bye")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "say \"bye\" ok\n" {
		t.Fatalf("ci\": got %q", bufText(ks))
	}
}

// --- Insert mode ---

func TestVimInsertAndEscape(t *testing.T) {
	ks := newVimState("")

	feedKeys(ks, "i")
	if ks.ModeID() != ModeInsert {
		t.Fatal("should be in insert mode")
	}
	feedKeys(ks, "hello")
	feedSpecial(ks, KeyEscape)
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be in normal mode")
	}
	if bufText(ks) != "hello" {
		t.Fatalf("insert: got %q", bufText(ks))
	}
}

func TestVimOpenLineBelow(t *testing.T) {
	ks := newVimState("line1\nline2\n")

	feedKeys(ks, "o")
	feedKeys(ks, "new")
	feedSpecial(ks, KeyEscape)
	if bufText(ks) != "line1\nnew\nline2\n" {
		t.Fatalf("o: got %q", bufText(ks))
	}
}

// --- Visual mode ---

func TestVimVisualDelete(t *testing.T) {
	ks := newVimState("hello world\n")

	feedKeys(ks, "vllld")
	// "v" selects first char, "lll" extends, "d" deletes
	if bufText(ks) != "o world\n" {
		t.Fatalf("vlld: got %q", bufText(ks))
	}
}

func TestVimIndent(t *testing.T) {
	ks := newVimState("line1\nline2\n")

	feedKeys(ks, ">>")
	if bufText(ks) != "\tline1\nline2\n" {
		t.Fatalf(">>: got %q", bufText(ks))
	}
}

func TestVimJoinLines(t *testing.T) {
	ks := newVimState("hello\nworld\n")

	feedKeys(ks, "J")
	if bufText(ks) != "hello world\n" {
		t.Fatalf("J: got %q", bufText(ks))
	}
}

// --- Paste cursor placement (vim semantics) ---

func TestPasteCharwiseCursor(t *testing.T) {
	ks := newVimState("abc\n")
	ks.regs.Set(RegDefault, []byte("XY"), false)

	// p: paste after the cursor char, cursor on the last pasted char.
	feedKeys(ks, "p")
	if bufText(ks) != "aXYbc\n" {
		t.Fatalf("p: got %q", bufText(ks))
	}
	if cursorPos(ks) != 2 {
		t.Fatalf("p cursor: got %d, want 2 (on 'Y')", cursorPos(ks))
	}

	// P: paste before the cursor char, cursor on the last pasted char.
	ks2 := newVimState("abc\n")
	ks2.regs.Set(RegDefault, []byte("XY"), false)
	feedKeys(ks2, "P")
	if bufText(ks2) != "XYabc\n" {
		t.Fatalf("P: got %q", bufText(ks2))
	}
	if cursorPos(ks2) != 1 {
		t.Fatalf("P cursor: got %d, want 1 (on 'Y')", cursorPos(ks2))
	}
}

func TestPasteCharwiseMultilineCursor(t *testing.T) {
	ks := newVimState("abc\n")
	ks.regs.Set(RegDefault, []byte("XY\nZ"), false)

	feedKeys(ks, "p")
	if bufText(ks) != "aXY\nZbc\n" {
		t.Fatalf("p: got %q", bufText(ks))
	}
	if cursorPos(ks) != 4 {
		t.Fatalf("p cursor: got %d, want 4 (on 'Z')", cursorPos(ks))
	}
}

func TestPasteCharwiseCountCursor(t *testing.T) {
	ks := newVimState("xy\n")
	ks.regs.Set(RegDefault, []byte("ab"), false)

	feedKeys(ks, "3p")
	if bufText(ks) != "xabababy\n" {
		t.Fatalf("3p: got %q", bufText(ks))
	}
	if cursorPos(ks) != 6 {
		t.Fatalf("3p cursor: got %d, want 6 (last pasted 'b')", cursorPos(ks))
	}
}

func TestPasteOnEmptyLine(t *testing.T) {
	// p on an empty line pastes into that line, not the next one.
	ks := newVimState("a\n\nb\n")
	ks.regs.Set(RegDefault, []byte("X"), false)

	feedKeys(ks, "j") // to the empty line
	feedKeys(ks, "p")
	if bufText(ks) != "a\nX\nb\n" {
		t.Fatalf("p on empty line: got %q", bufText(ks))
	}
	if cursorPos(ks) != 2 {
		t.Fatalf("cursor: got %d, want 2 (on 'X')", cursorPos(ks))
	}
}

func TestPasteLinewiseCursor(t *testing.T) {
	// Linewise paste puts the cursor on the first non-blank of the first
	// pasted line.
	ks := newVimState("one\ntwo\n")
	ks.regs.Set(RegDefault, []byte("  new\n"), true)

	feedKeys(ks, "p")
	if bufText(ks) != "one\n  new\ntwo\n" {
		t.Fatalf("p: got %q", bufText(ks))
	}
	if cursorPos(ks) != 6 {
		t.Fatalf("p cursor: got %d, want 6 (first non-blank of pasted line)", cursorPos(ks))
	}

	ks2 := newVimState("one\ntwo\n")
	ks2.regs.Set(RegDefault, []byte("  new\n"), true)
	feedKeys(ks2, "j") // paste above line 1
	feedKeys(ks2, "P")
	if bufText(ks2) != "one\n  new\ntwo\n" {
		t.Fatalf("P: got %q", bufText(ks2))
	}
	if cursorPos(ks2) != 6 {
		t.Fatalf("P cursor: got %d, want 6", cursorPos(ks2))
	}
}

func TestPasteLinewiseEOFCursor(t *testing.T) {
	// Pasting below the last line of a file without a trailing newline.
	ks := newVimState("one")
	ks.regs.Set(RegDefault, []byte("new\n"), true)

	feedKeys(ks, "p")
	if bufText(ks) != "one\nnew" {
		t.Fatalf("p: got %q", bufText(ks))
	}
	if cursorPos(ks) != 4 {
		t.Fatalf("p cursor: got %d, want 4 (start of pasted line)", cursorPos(ks))
	}
}

func TestVisualPasteCursor(t *testing.T) {
	ks := newVimState("hello world\n")
	ks.regs.Set(RegDefault, []byte("world"), false)

	feedKeys(ks, "vllll") // select "hello"
	feedKeys(ks, "p")
	if bufText(ks) != "world world\n" {
		t.Fatalf("visual p: got %q", bufText(ks))
	}
	if cursorPos(ks) != 4 {
		t.Fatalf("visual p cursor: got %d, want 4 (last pasted char)", cursorPos(ks))
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be back in normal mode")
	}
}
