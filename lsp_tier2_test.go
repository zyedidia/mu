package main

import (
	"testing"
	"time"
)

// gr opens a references palette; selecting an entry jumps to it.
func TestLspFindReferencesPalette(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.Path = "/tmp/x.go"
	b.text.Insert(0, []byte("hi\nthere\n"))
	b.lspServer = s

	start := time.Now()
	ed.lspFindReferences()
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("references blocked on the LSP request")
	}
	drainUntil(t, ed, "references palette", func() bool {
		return ed.palette.active && len(ed.palette.items) == 1
	})
	ed.dispatchKey(KeyEnter)
	if got := b.Cursor().Pos; got != 0 {
		t.Fatalf("jumped to %d, want 0", got)
	}
}

// :lsp-symbols lists the current document's symbols; selecting one jumps to
// its selection range.
func TestLspDocumentSymbolsPalette(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.Path = "/tmp/x.go"
	b.text.Insert(0, []byte("func foo() {\n\treturn\n}\n"))
	b.lspServer = s

	ed.lspDocumentSymbols()
	drainUntil(t, ed, "symbols palette", func() bool {
		return ed.palette.active && len(ed.palette.items) == 1
	})
	if got := ed.palette.items[0].label; got == "" {
		t.Fatal("symbol label empty")
	}
	ed.dispatchKey(KeyEnter)
	// selectionRange in the fake starts at line 0, character 5 ("foo").
	if got := b.Cursor().Pos; got != 5 {
		t.Fatalf("jumped to %d, want 5", got)
	}
}

// :lsp-rename prefills the prompt with the word under the cursor and
// applies the resulting workspace edit.
func TestLspRenamePrompt(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.Path = "/tmp/x.go"
	b.text.Insert(0, []byte("hi\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)
	b.lspServer = s

	ed.lspRename()
	if !ed.infobar.active || string(ed.infobar.input) != "hi" {
		t.Fatalf("rename prompt not prefilled: active=%v input=%q", ed.infobar.active, string(ed.infobar.input))
	}
	ed.infobar.HandleKey(KeyEnter)
	drainUntil(t, ed, "rename applied", func() bool {
		return string(b.Slice(0, b.Len())) == "renamed\n"
	})
}

// <C-k> in insert mode requests signature help and shows it in the infobar,
// without blocking the event loop.
func TestLspSignatureHelpInsertMode(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.Path = "/tmp/x.go"
	b.lspServer = s
	ed.ks.SetMode(ModeInsert)

	start := time.Now()
	ed.lspSignatureHelpAt()
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("signature help blocked on the LSP request")
	}
	drainUntil(t, ed, "signature help", func() bool {
		return ed.infobar.message == "foo(a, [b int])"
	})
}
