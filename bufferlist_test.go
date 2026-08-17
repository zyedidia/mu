package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupBufEditor creates an isolated editor plus two files on disk, both
// opened (b.txt last, so it is in the active pane and a.txt is hidden).
func setupBufEditor(t *testing.T) (*Editor, string, string) {
	t.Helper()
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	os.WriteFile(a, []byte("aaa\n"), 0644)
	os.WriteFile(b, []byte("bbb\n"), 0644)
	if err := ed.OpenFile(a); err != nil {
		t.Fatal(err)
	}
	if err := ed.OpenFile(b); err != nil {
		t.Fatal(err)
	}
	return ed, a, b
}

func TestBufferListLs(t *testing.T) {
	ed, a, b := setupBufEditor(t)

	ed.RunCommand("ls")
	msg := ed.infobar.message
	if !strings.Contains(msg, a) || !strings.Contains(msg, b) {
		t.Fatalf("ls output missing buffers: %q", msg)
	}
	if !strings.Contains(msg, "% "+b) {
		t.Fatalf("ls should mark the current buffer: %q", msg)
	}
	if !strings.Contains(msg, "# "+a) {
		t.Fatalf("ls should mark the alternate buffer: %q", msg)
	}

	// A hidden modified buffer shows +.
	ed.RunCommand("b " + a)
	ed.ks.Buf().Insert(0, []byte("x"))
	ed.RunCommand("b " + b)
	ed.RunCommand("ls")
	if !strings.Contains(ed.infobar.message, "#+ ") {
		t.Fatalf("ls should mark hidden modified buffer: %q", ed.infobar.message)
	}
}

func TestBufferSwitch(t *testing.T) {
	ed, a, b := setupBufEditor(t)

	// By name substring.
	ed.RunCommand("b a.txt")
	if ed.ActiveView().buf.Path != a {
		t.Fatalf("b by name: showing %q, want %q", ed.ActiveView().buf.Path, a)
	}
	// By number (b.txt was the third buffer: initial, a, b).
	num := 0
	for _, ob := range ed.buffers {
		if ob.Path == b {
			num = ob.bufnum
		}
	}
	ed.RunCommand("buffer " + string(rune('0'+num)))
	if ed.ActiveView().buf.Path != b {
		t.Fatalf("b by number: showing %q, want %q", ed.ActiveView().buf.Path, b)
	}
}

func TestBufferSwitchAmbiguous(t *testing.T) {
	ed, _, _ := setupBufEditor(t)

	ed.RunCommand("b .txt") // matches several buffers
	if !ed.infobar.msgErr {
		t.Fatal("ambiguous buffer name should error")
	}
}

func TestBufferAlternate(t *testing.T) {
	ed, a, b := setupBufEditor(t)

	ed.RunCommand("b #")
	if ed.ActiveView().buf.Path != a {
		t.Fatalf("b #: showing %q, want %q", ed.ActiveView().buf.Path, a)
	}
	ed.RunCommand("b #")
	if ed.ActiveView().buf.Path != b {
		t.Fatalf("b # again: showing %q, want %q", ed.ActiveView().buf.Path, b)
	}
}

func TestBufferNextPrev(t *testing.T) {
	ed, a, b := setupBufEditor(t)

	// Buffers: [initial, a, b]; active is b (last).
	ed.RunCommand("bn") // wraps to the initial buffer
	first := ed.ActiveView().buf
	if first.Path == a || first.Path == b {
		t.Fatalf("bn from last should wrap to the first buffer, got %q", first.Path)
	}
	ed.RunCommand("bn")
	if ed.ActiveView().buf.Path != a {
		t.Fatalf("bn: showing %q, want %q", ed.ActiveView().buf.Path, a)
	}
	ed.RunCommand("bp")
	if ed.ActiveView().buf != first {
		t.Fatalf("bp: showing %q, want the first buffer", ed.ActiveView().buf.Path)
	}
}

func TestHiddenBufferKeepsChanges(t *testing.T) {
	ed, a, _ := setupBufEditor(t)

	ed.RunCommand("b a.txt")
	ed.ks.Buf().Insert(0, []byte("edit "))
	// Switching away from the modified buffer is allowed; it hides.
	ed.RunCommand("b b.txt")
	if ed.infobar.msgErr {
		t.Fatalf("switching away from modified buffer: %s", ed.infobar.message)
	}
	ed.RunCommand("b a.txt")
	if got := bufText(ed.ks); got != "edit aaa\n" {
		t.Fatalf("hidden buffer content: got %q", got)
	}
	if !ed.ks.Buf().Modified() {
		t.Fatal("hidden buffer should still be modified")
	}
	_ = a
}

func TestQuitRefusesHiddenModified(t *testing.T) {
	ed, _, _ := setupBufEditor(t)

	ed.RunCommand("b a.txt")
	ed.ks.Buf().Insert(0, []byte("edit "))
	ed.RunCommand("b b.txt")

	ed.RunCommand("q") // last pane: would exit
	if !ed.running {
		t.Fatal("q must refuse while a hidden buffer is modified")
	}
	if !ed.infobar.msgErr {
		t.Fatal("expected an error message")
	}
	ed.RunCommand("qa")
	if !ed.running {
		t.Fatal("qa must refuse while a hidden buffer is modified")
	}
	ed.RunCommand("qa!")
	if ed.running {
		t.Fatal("qa! should force quit")
	}
}

func TestPaneCloseWithModifiedBufferAllowed(t *testing.T) {
	// Mid-session :q hides a modified buffer instead of refusing.
	ed, a, _ := setupBufEditor(t)

	ed.VSplit(nil)
	ed.RunCommand("b a.txt")
	ed.ks.Buf().Insert(0, []byte("edit "))
	ed.RunCommand("q")
	if ed.infobar.msgErr {
		t.Fatalf("mid-session q on modified buffer: %s", ed.infobar.message)
	}
	if !ed.running {
		t.Fatal("editor should still be running")
	}
	// The buffer is hidden with its changes.
	ed.RunCommand("b a.txt")
	if got := bufText(ed.ks); got != "edit aaa\n" {
		t.Fatalf("hidden buffer content: got %q", got)
	}
	_ = a
}

func TestWriteAllIncludesHidden(t *testing.T) {
	ed, a, _ := setupBufEditor(t)

	ed.RunCommand("b a.txt")
	ed.ks.Buf().Insert(0, []byte("edit "))
	ed.RunCommand("b b.txt")
	ed.RunCommand("wa")
	if ed.infobar.msgErr {
		t.Fatalf("wa error: %s", ed.infobar.message)
	}
	data, _ := os.ReadFile(a)
	if string(data) != "edit aaa\n" {
		t.Fatalf("hidden buffer not written: %q", data)
	}
}

func TestBufferDelete(t *testing.T) {
	ed, a, b := setupBufEditor(t)

	deleted := ed.ActiveView().buf // b.txt
	ed.RunCommand("bd")
	if ed.infobar.msgErr {
		t.Fatalf("bd error: %s", ed.infobar.message)
	}
	if ed.ActiveView().buf == deleted {
		t.Fatal("pane still shows the deleted buffer")
	}
	if ed.ActiveView().buf.Path != a {
		t.Fatalf("pane should show the alternate buffer, got %q", ed.ActiveView().buf.Path)
	}
	if deleted.watchDone != nil {
		t.Fatal("deleted buffer's watcher still running")
	}
	ed.RunCommand("ls")
	if strings.Contains(ed.infobar.message, b) {
		t.Fatalf("deleted buffer still listed: %q", ed.infobar.message)
	}

	// bd on a modified buffer refuses.
	ed.ks.Buf().Insert(0, []byte("x"))
	ed.RunCommand("bd")
	if !ed.infobar.msgErr {
		t.Fatal("bd on modified buffer should error")
	}
}

// --- :w copy semantics (the path-collision hole) ---

func TestWriteCopyKeepsName(t *testing.T) {
	ed, a, _ := setupBufEditor(t)

	ed.RunCommand("b a.txt")
	b := ed.ks.Buf()
	b.Insert(0, []byte("edit "))
	other := filepath.Join(filepath.Dir(a), "copy.txt")
	ed.RunCommand("w " + other)
	if ed.infobar.msgErr {
		t.Fatalf("w <path>: %s", ed.infobar.message)
	}
	data, _ := os.ReadFile(other)
	if string(data) != "edit aaa\n" {
		t.Fatalf("copy content: got %q", data)
	}
	if b.Path != a {
		t.Fatalf("buffer renamed by :w <path>: %q", b.Path)
	}
	if !b.Modified() {
		t.Fatal(":w <path> must not clear the buffer's modified state")
	}
}

func TestWriteCopyToOpenFileNoCollision(t *testing.T) {
	// :w naming a file open in another buffer writes it but never creates
	// two buffers claiming one path.
	ed, a, b := setupBufEditor(t)

	cur := ed.ActiveView().buf // b.txt
	ed.ks.Buf().Insert(0, []byte("new "))
	ed.RunCommand("w " + a)
	if ed.infobar.msgErr {
		t.Fatalf("w to open path: %s", ed.infobar.message)
	}
	if cur.Path != b {
		t.Fatalf("buffer adopted a colliding path: %q", cur.Path)
	}
	if fb := ed.findBuffer(a); fb == cur {
		t.Fatal("two buffers claim one path")
	}
	data, _ := os.ReadFile(a)
	if string(data) != "new bbb\n" {
		t.Fatalf("target content: got %q", data)
	}
}

func TestUnnamedWriteToOpenPathRefused(t *testing.T) {
	ed, a, _ := setupBufEditor(t)

	ed.RunCommand("tabnew") // unnamed buffer
	ed.ks.Buf().Insert(0, []byte("scratch"))
	ed.RunCommand("w " + a)
	if !ed.infobar.msgErr {
		t.Fatal("unnamed :w to an open path must refuse")
	}
	if ed.ks.Buf().Path != "" {
		t.Fatalf("unnamed buffer adopted a colliding path: %q", ed.ks.Buf().Path)
	}
}

func TestReopenHiddenFileReusesBuffer(t *testing.T) {
	// :e of a hidden file's path resurfaces its buffer (with any unsaved
	// changes) instead of creating a duplicate.
	ed, a, _ := setupBufEditor(t)

	ed.RunCommand("b a.txt")
	hidden := ed.ActiveView().buf
	hidden.Insert(0, []byte("edit "))
	ed.RunCommand("b b.txt") // hide the modified a.txt

	ed.RunCommand("e " + a)
	if ed.ActiveView().buf != hidden {
		t.Fatal(":e of a hidden file must reuse its buffer")
	}
	if got := bufText(ed.ks); got != "edit aaa\n" {
		t.Fatalf("resurfaced buffer content: got %q", got)
	}
	// Still exactly one listed buffer for the path.
	n := 0
	for _, b := range ed.buffers {
		if samePath(b.Path, a) {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d buffers listed for %q, want 1", n, a)
	}
}
