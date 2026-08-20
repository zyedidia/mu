package main

import (
	"os"
	"path/filepath"
	"testing"
)

// configureView wires Buffer.beforeSave to applyFormatOnSave for every
// buffer it sets up, regardless of filetype or whether an LSP server ends
// up attached.
func TestConfigureViewWiresBeforeSave(t *testing.T) {
	ed, a, _ := setupBufEditor(t)
	if b := ed.findBuffer(a); b == nil || b.beforeSave == nil {
		t.Fatal("beforeSave not wired by configureView")
	}
}

// newAutoformatBuffer builds a buffer backed by a real temp file (so Save
// actually writes to disk) with beforeSave wired exactly as configureView
// does, and an LSP server attached — without going through OpenFile, so
// filetype-based LSP auto-attach can't race the test's own fake server (the
// same convention every other LSP test in this file follows).
func newAutoformatBuffer(t *testing.T, ed *Editor, s *LspServer, content string) (*Buffer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.go")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	b := ed.ActiveView().buf
	b.Path = path
	b.text.Insert(0, []byte(content))
	b.beforeSave = func(bb *Buffer) { ed.applyFormatOnSave(bb) }
	b.lspServer = s
	return b, path
}

// autoformat defaults to off: saving does not reformat the buffer or the
// file on disk.
func TestAutoformatDisabledByDefault(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b, path := newAutoformatBuffer(t, ed, s, "abcdef\n")

	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "abcdef\n" {
		t.Fatalf("file changed with autoformat off: got %q", data)
	}
}

// With autoformat on, saving formats the buffer first, and the formatted
// content (not the pre-format content) is what lands on disk.
func TestAutoformatAppliesEditsOnSave(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	ed.RunCommand("set autoformat true")
	b, path := newAutoformatBuffer(t, ed, s, "abcdef\n")

	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	// The fake always replies with one fixed edit: replace [0,3) with "fmt".
	want := "fmtdef\n"
	if got := string(b.Slice(0, b.Len())); got != want {
		t.Fatalf("buffer after save: got %q, want %q", got, want)
	}
	data, _ := os.ReadFile(path)
	if string(data) != want {
		t.Fatalf("file after save: got %q, want %q", data, want)
	}
}

// autoformat with no LSP server attached is a no-op, not an error.
func TestAutoformatNoServerIsNoop(t *testing.T) {
	ed := newTestEditor()
	ed.RunCommand("set autoformat true")

	path := filepath.Join(t.TempDir(), "x.go")
	os.WriteFile(path, []byte("abcdef\n"), 0644)
	b := ed.ActiveView().buf
	b.Path = path
	b.text.Insert(0, []byte("abcdef\n"))
	b.beforeSave = func(bb *Buffer) { ed.applyFormatOnSave(bb) }

	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "abcdef\n" {
		t.Fatalf("file changed without an LSP server: got %q", data)
	}
}
