package main

import (
	"os"
	"path/filepath"
	"testing"
)

// setupFileEditor creates an isolated editor and a file on disk.
func setupFileEditor(t *testing.T, content string) (*Editor, string) {
	t.Helper()
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()
	path := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(path, []byte(content), 0644)
	return ed, path
}

func TestVSplitSameFileSharesBuffer(t *testing.T) {
	ed, path := setupFileEditor(t, "hello\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	b1 := ed.ActiveView().buf
	ed.VSplit([]string{path})
	b2 := ed.ActiveView().buf
	if b1 != b2 {
		t.Fatal("vsplit of an open file must share the buffer")
	}

	// An edit made in one pane is the shared buffer's content.
	b2.Insert(0, []byte("edit "))
	ed.ActiveTab().NextPane()
	ed.syncActiveBuffer()
	if got := bufText(ed.ks); got != "edit hello\n" {
		t.Fatalf("other pane content: got %q", got)
	}
}

func TestOpenFileInTabShares(t *testing.T) {
	ed, path := setupFileEditor(t, "hello\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	b1 := ed.ActiveView().buf
	if err := ed.OpenFileInTab(path); err != nil {
		t.Fatal(err)
	}
	if ed.ActiveView().buf != b1 {
		t.Fatal("tab open of an open file must share the buffer")
	}
}

func TestTabNewCommandShares(t *testing.T) {
	ed, path := setupFileEditor(t, "hello\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	b1 := ed.ActiveView().buf
	ed.RunCommand("tabnew " + path)
	if ed.infobar.msgErr {
		t.Fatalf("tabnew error: %s", ed.infobar.message)
	}
	if ed.ActiveView().buf != b1 {
		t.Fatal("tabnew of an open file must share the buffer")
	}
}

func TestEditOtherPaneFileShares(t *testing.T) {
	// :e naming a file open in another tab reuses that buffer.
	ed, path := setupFileEditor(t, "hello\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	b1 := ed.ActiveView().buf
	ed.RunCommand("tabnew")
	if ed.ActiveView().buf == b1 {
		t.Fatal("tabnew should open a fresh buffer")
	}
	ed.RunCommand("e " + path)
	if ed.ActiveView().buf != b1 {
		t.Fatal(":e of an open file must share the buffer")
	}
}

func TestPathSpellingsShare(t *testing.T) {
	// Different spellings of the same path share (absolute comparison).
	ed, path := setupFileEditor(t, "hello\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	b1 := ed.ActiveView().buf
	alt := filepath.Join(filepath.Dir(path), ".", filepath.Base(path))
	if err := ed.OpenFileInTab(alt); err != nil {
		t.Fatal(err)
	}
	if ed.ActiveView().buf != b1 {
		t.Fatalf("path spelling %q should share the buffer of %q", alt, path)
	}
}

func TestSharedBufferSaveNoClobber(t *testing.T) {
	// Edit through one pane, save from the other: the edit reaches disk
	// (with independent buffers the stale copy would have clobbered it).
	ed, path := setupFileEditor(t, "hello\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	ed.VSplit([]string{path})
	// Edit in the split pane...
	ed.ks.Buf().Insert(0, []byte("edit "))
	// ...switch to the first pane and save from there.
	ed.ActiveTab().NextPane()
	ed.syncActiveBuffer()
	ed.RunCommand("w")
	if ed.infobar.msgErr {
		t.Fatalf("save error: %s", ed.infobar.message)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "edit hello\n" {
		t.Fatalf("disk content: got %q, want %q", data, "edit hello\n")
	}
	if ed.ks.Buf().Modified() {
		t.Fatal("shared buffer should be unmodified after save")
	}
}

// --- :e reload semantics ---

func TestEditNoArgsReloads(t *testing.T) {
	ed, path := setupFileEditor(t, "old\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("new content\n"), 0644)
	ed.RunCommand("e")
	if got := bufText(ed.ks); got != "new content\n" {
		t.Fatalf(":e reload: got %q", got)
	}
}

func TestEditSameFileReloads(t *testing.T) {
	ed, path := setupFileEditor(t, "old\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	b1 := ed.ActiveView().buf
	os.WriteFile(path, []byte("new content\n"), 0644)
	ed.RunCommand("e " + path)
	if got := bufText(ed.ks); got != "new content\n" {
		t.Fatalf(":e same file: got %q", got)
	}
	if ed.ActiveView().buf != b1 {
		t.Fatal(":e of the current file must keep the same buffer")
	}
}

func TestEditModifiedRefusesReload(t *testing.T) {
	ed, path := setupFileEditor(t, "old\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	ed.ks.Buf().Insert(0, []byte("dirty "))
	ed.RunCommand("e")
	if !ed.infobar.msgErr {
		t.Fatal(":e on a modified buffer should error")
	}
	if got := bufText(ed.ks); got != "dirty old\n" {
		t.Fatalf("buffer must be untouched: got %q", got)
	}
}
