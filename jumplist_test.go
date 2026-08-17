package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jumpEditor builds an isolated editor whose buffer has n numbered lines.
func jumpEditor(t *testing.T, n int) *Editor {
	t.Helper()
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	b := ed.ks.Buf()
	b.text.Insert(0, []byte(sb.String()))
	*b.Cursor() = b.Cursor().MoveTo(0)
	return ed
}

func curLine(ed *Editor) int {
	b := ed.ks.Buf()
	line, _ := b.LineColAt(b.Cursor().Pos)
	return line
}

func TestJumpListBackForward(t *testing.T) {
	ed := jumpEditor(t, 30)
	b := ed.ks.Buf()
	*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(10, 0))

	feedDisplay(ed.ks, "G")
	if curLine(ed) != 30 {
		t.Fatalf("G: line %d, want 30", curLine(ed))
	}
	feedSpecial(ed.ks, "<C-o>")
	if curLine(ed) != 10 {
		t.Fatalf("<C-o>: line %d, want 10", curLine(ed))
	}
	feedSpecial(ed.ks, KeyTab)
	if curLine(ed) != 30 {
		t.Fatalf("<C-i>: line %d, want 30", curLine(ed))
	}
}

func TestJumpListChain(t *testing.T) {
	ed := jumpEditor(t, 30)

	ed.RunCommand("15") // goto: jump from line 0
	if curLine(ed) != 14 {
		t.Fatalf(":15: line %d, want 14", curLine(ed))
	}
	feedDisplay(ed.ks, "G") // jump from line 14
	feedDisplay(ed.ks, "gg")
	if curLine(ed) != 0 {
		t.Fatalf("gg: line %d, want 0", curLine(ed))
	}

	// Walk back through the chain: 30 (before gg), 14 (before G), 0.
	feedSpecial(ed.ks, "<C-o>")
	if curLine(ed) != 30 {
		t.Fatalf("first <C-o>: line %d, want 30", curLine(ed))
	}
	feedSpecial(ed.ks, "<C-o>")
	if curLine(ed) != 14 {
		t.Fatalf("second <C-o>: line %d, want 14", curLine(ed))
	}
	// A count walks several entries at once.
	feedDisplay(ed.ks, "2")
	feedSpecial(ed.ks, KeyTab)
	if curLine(ed) != 0 {
		t.Fatalf("2<C-i>: line %d, want 0", curLine(ed))
	}
}

func TestJumpListDedupeSameLine(t *testing.T) {
	ed := jumpEditor(t, 30)

	feedDisplay(ed.ks, "G", "gg", "G", "gg")
	// Jumps from line 30 and line 0 repeated: one entry each.
	if n := len(ed.jumps.entries); n != 2 {
		t.Fatalf("entries = %d, want 2 (deduped)", n)
	}
}

func TestJumpListSearch(t *testing.T) {
	ed := jumpEditor(t, 30)
	b := ed.ks.Buf()
	b.text.Insert(b.Len(), []byte("needle\n"))

	edKeys(ed, "/needle", "<CR>")
	if curLine(ed) != 30 {
		t.Fatalf("search: line %d, want 30", curLine(ed))
	}
	feedSpecial(ed.ks, "<C-o>")
	if curLine(ed) != 0 {
		t.Fatalf("<C-o> after search: line %d, want 0", curLine(ed))
	}
}

func TestJumpListSearchNext(t *testing.T) {
	ed := jumpEditor(t, 5)
	b := ed.ks.Buf()
	b.text.Insert(b.Len(), []byte("x\nfiller\nx\n"))

	edKeys(ed, "/x", "<CR>") // line 5
	feedKeys(ed.ks, "n")     // line 7
	if curLine(ed) != 7 {
		t.Fatalf("n: line %d, want 7", curLine(ed))
	}
	feedSpecial(ed.ks, "<C-o>")
	if curLine(ed) != 5 {
		t.Fatalf("<C-o> after n: line %d, want 5", curLine(ed))
	}
}

func TestJumpListMark(t *testing.T) {
	ed := jumpEditor(t, 30)

	feedKeys(ed.ks, "ma")
	feedDisplay(ed.ks, "G")
	feedKeys(ed.ks, "'a") // mark jump back to line 0
	if curLine(ed) != 0 {
		t.Fatalf("'a: line %d, want 0", curLine(ed))
	}
	feedSpecial(ed.ks, "<C-o>")
	if curLine(ed) != 30 {
		t.Fatalf("<C-o> after mark jump: line %d, want 30", curLine(ed))
	}
}

func TestJumpListCrossBuffer(t *testing.T) {
	ed := jumpEditor(t, 10)
	first := ed.ks.Buf()
	path := filepath.Join(t.TempDir(), "other.txt")
	os.WriteFile(path, []byte("other\n"), 0644)

	*first.Cursor() = first.Cursor().MoveTo(first.OffsetAt(5, 0))
	ed.RunCommand("e " + path)
	if ed.ks.Buf() == first {
		t.Fatal("should be showing the other buffer")
	}
	feedSpecial(ed.ks, "<C-o>")
	if ed.ks.Buf() != first {
		t.Fatal("<C-o> should switch back to the first buffer")
	}
	if curLine(ed) != 5 {
		t.Fatalf("<C-o>: line %d, want 5", curLine(ed))
	}
	feedSpecial(ed.ks, KeyTab)
	if ed.ks.Buf() == first {
		t.Fatal("<C-i> should return to the other buffer")
	}
}

func TestJumpListPrunesDeletedBuffers(t *testing.T) {
	ed := jumpEditor(t, 10)
	first := ed.ks.Buf()
	path := filepath.Join(t.TempDir(), "other.txt")
	os.WriteFile(path, []byte("other\n"), 0644)

	*first.Cursor() = first.Cursor().MoveTo(first.OffsetAt(5, 0))
	ed.RunCommand("e " + path)
	other := ed.ks.Buf()
	feedDisplay(ed.ks, "G") // jump within other: entry referencing other
	ed.RunCommand("b #")    // back to first buffer
	ed.RunCommand("bd other.txt")
	if ed.bufferListed(other) {
		t.Fatal("other buffer should be deleted")
	}
	for _, p := range ed.jumps.entries {
		if p.buf == other {
			t.Fatal("jump list still references the deleted buffer")
		}
	}
}

func TestJumpListClampsAfterEdits(t *testing.T) {
	ed := jumpEditor(t, 30)
	b := ed.ks.Buf()
	*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(20, 0))

	feedDisplay(ed.ks, "gg")
	// Shrink the buffer below the recorded line.
	b.Remove(b.OffsetAt(10, 0), b.Len())
	feedSpecial(ed.ks, "<C-o>")
	if l := curLine(ed); l > b.NumLines() {
		t.Fatalf("<C-o> beyond EOF: line %d of %d", l, b.NumLines())
	}
}

func TestJumpsCommand(t *testing.T) {
	ed := jumpEditor(t, 30)

	ed.RunCommand("jumps")
	if ed.infobar.message != "no jumps" {
		t.Fatalf("empty jumps: %q", ed.infobar.message)
	}
	feedDisplay(ed.ks, "G")
	ed.RunCommand("jumps")
	if !strings.Contains(ed.infobar.message, "1:0") {
		t.Fatalf("jumps output: %q", ed.infobar.message)
	}
}
