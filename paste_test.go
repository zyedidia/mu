package main

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// Pasted text is inserted verbatim: autoclose does not double brackets or
// quotes and autoindent does not re-indent, even in insert mode.
func TestPasteInsertVerbatim(t *testing.T) {
	ed := newTestEditor()
	ed.dispatchKey("i")

	text := "if (x) {\n\t\"s\"\n}\n"
	ed.pasteText(text)
	if got := bufText(ed.ks); got != text {
		t.Fatalf("paste: got %q, want %q", got, text)
	}
	if ed.ks.ModeID() != ModeInsert {
		t.Fatalf("mode = %v, want insert", ed.ks.ModeID())
	}
	if ed.ks.Buf().Cursor().Pos != len(text) {
		t.Fatalf("cursor = %d, want %d (after pasted text)", ed.ks.Buf().Cursor().Pos, len(text))
	}
}

// The whole paste is one undo unit.
func TestPasteSingleUndo(t *testing.T) {
	ed := newTestEditor()
	b := ed.ks.Buf()
	b.text.Insert(0, []byte("base\n"))

	ed.dispatchKey("i")
	ed.pasteText("one\ntwo\nthree\n")
	ed.dispatchKey("<Esc>")
	if got := bufText(ed.ks); got != "one\ntwo\nthree\nbase\n" {
		t.Fatalf("after paste: %q", got)
	}
	ed.dispatchKey("u")
	if got := bufText(ed.ks); got != "base\n" {
		t.Fatalf("one undo should remove the whole paste: %q", got)
	}
}

// Normal-mode paste inserts before the cursor and lands on the last pasted
// character, like i<text><Esc>.
func TestPasteNormalMode(t *testing.T) {
	ed := newTestEditor()
	b := ed.ks.Buf()
	b.text.Insert(0, []byte("XY\n"))

	ed.pasteText("ab")
	if got := bufText(ed.ks); got != "abXY\n" {
		t.Fatalf("normal paste: %q", got)
	}
	if b.Cursor().Pos != 1 {
		t.Fatalf("cursor = %d, want 1 (on 'b')", b.Cursor().Pos)
	}
}

// A charwise visual selection is replaced by the paste.
func TestPasteVisualReplaces(t *testing.T) {
	ed := newTestEditor()
	b := ed.ks.Buf()
	b.text.Insert(0, []byte("hello world\n"))

	for _, k := range []string{"v", "l", "l", "l", "l"} {
		ed.dispatchKey(k)
	}
	ed.pasteText("hi")
	if got := bufText(ed.ks); got != "hi world\n" {
		t.Fatalf("visual paste: %q", got)
	}
	if ed.ks.ModeID() != ModeNormal {
		t.Fatalf("mode = %v, want normal", ed.ks.ModeID())
	}
	if b.Cursor().Pos != 1 {
		t.Fatalf("cursor = %d, want 1 (on 'i')", b.Cursor().Pos)
	}
}

// CR and CRLF line endings normalize to LF.
func TestPasteCRLFNormalized(t *testing.T) {
	ed := newTestEditor()
	ed.dispatchKey("i")
	ed.pasteText("a\r\nb\rc")
	if got := bufText(ed.ks); got != "a\nb\nc" {
		t.Fatalf("normalized paste: %q", got)
	}
}

// Insert-mode paste lands at every cursor.
func TestPasteMultiCursor(t *testing.T) {
	ed := newTestEditor()
	b := ed.ks.Buf()
	b.text.Insert(0, []byte("a b\n"))
	b.SpawnCursor(2)

	ed.dispatchKey("i")
	ed.pasteText("X")
	if got := bufText(ed.ks); got != "Xa Xb\n" {
		t.Fatalf("multi-cursor paste: %q", got)
	}
}

// An active command-line prompt receives the first pasted line.
func TestPastePrompt(t *testing.T) {
	ed := newTestEditor()
	ed.infobar.StartPrompt(":", func(string) {})
	ed.pasteText("write\nextra junk")
	if got := string(ed.infobar.input); got != "write" {
		t.Fatalf("prompt input = %q, want %q", got, "write")
	}
}

// The run-loop collector translates paste key events back into text.
func TestPasteCollector(t *testing.T) {
	ed := newTestEditor()
	for _, ev := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyRune, 'a', 0),
		tcell.NewEventKey(tcell.KeyEnter, 0, 0),
		tcell.NewEventKey(tcell.KeyTab, 0, 0),
		tcell.NewEventKey(tcell.KeyRune, 'é', 0),
		tcell.NewEventKey(tcell.KeyUp, 0, 0), // stray sequence: dropped
	} {
		ed.collectPasteKey(ev)
	}
	if got := ed.pasteBuf.String(); got != "a\r\té" {
		t.Fatalf("collected = %q, want %q", got, "a\r\té")
	}
	ed.dispatchKey("i")
	ed.pasteText(ed.pasteBuf.String())
	if got := bufText(ed.ks); got != "a\n\té" {
		t.Fatalf("collected paste: %q", got)
	}
}
