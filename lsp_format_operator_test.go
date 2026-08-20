package main

import (
	"encoding/json"
	"testing"

	lsp "go.lsp.dev/protocol"
)

// newLspOperatorEditor builds a test editor with the = format operator bound
// (registerLspBindings isn't part of newTestEditor's default setup) and an
// LSP server attached to its active buffer.
func newLspOperatorEditor(t *testing.T, text string) (*Editor, *Buffer, *fakeLspServer) {
	t.Helper()
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	ed.registerLspBindings()
	b := ed.ActiveView().buf
	b.Path = "/tmp/x.go"
	b.text.Insert(0, []byte(text))
	*b.Cursor() = b.Cursor().MoveTo(0)
	b.lspServer = s
	return ed, b, fake
}

func rangeFormattingRange(t *testing.T, fake *fakeLspServer) lsp.Range {
	t.Helper()
	raw, ok := fake.paramsFor("textDocument/rangeFormatting")
	if !ok {
		t.Fatal("no textDocument/rangeFormatting request received")
	}
	var params lsp.DocumentRangeFormattingParams
	if err := json.Unmarshal(raw, &params); err != nil {
		t.Fatalf("decode rangeFormatting params: %v", err)
	}
	return params.Range
}

// =w takes a motion: it formats from the cursor to the start of the next
// word, like vim's = operator composing with any motion.
func TestLspFormatOperatorMotion(t *testing.T) {
	ed, b, fake := newLspOperatorEditor(t, "hello world\n")

	ed.dispatchKey("=")
	ed.dispatchKey("w")
	drainUntil(t, ed, "range formatting request", func() bool {
		_, ok := fake.paramsFor("textDocument/rangeFormatting")
		return ok
	})

	rng := rangeFormattingRange(t, fake)
	if rng.Start.Line != 0 || rng.Start.Character != 0 || rng.End.Line != 0 || rng.End.Character != 6 {
		t.Fatalf("=w range: got %+v, want [0,0]-[0,6]", rng)
	}
	// The fake always replies with a fixed edit regardless of the
	// requested range; confirm it actually landed.
	drainUntil(t, ed, "edit applied", func() bool {
		return string(b.Slice(0, b.Len())) == "hillo world\n"
	})
}

// == formats the current line (the doubled-operator convention, like dd/yy).
func TestLspFormatOperatorDoubled(t *testing.T) {
	ed, b, fake := newLspOperatorEditor(t, "hello world\nsecond line\n")

	ed.dispatchKey("=")
	ed.dispatchKey("=")
	drainUntil(t, ed, "range formatting request", func() bool {
		_, ok := fake.paramsFor("textDocument/rangeFormatting")
		return ok
	})

	rng := rangeFormattingRange(t, fake)
	if rng.Start.Line != 0 || rng.End.Line != 1 {
		t.Fatalf("== range: got %+v, want line 0 through start of line 1", rng)
	}
	drainUntil(t, ed, "edit applied", func() bool {
		return string(b.Slice(0, b.Len())) == "hillo world\nsecond line\n"
	})
}

// A visual selection formats just the selected range and returns to normal
// mode.
func TestLspFormatOperatorVisualMode(t *testing.T) {
	ed, b, fake := newLspOperatorEditor(t, "hello world\n")

	ed.dispatchKey("v")
	for i := 0; i < 4; i++ {
		ed.dispatchKey("l")
	}
	ed.dispatchKey("=")
	drainUntil(t, ed, "range formatting request", func() bool {
		_, ok := fake.paramsFor("textDocument/rangeFormatting")
		return ok
	})

	rng := rangeFormattingRange(t, fake)
	if rng.Start.Character != 0 || rng.End.Character != 5 {
		t.Fatalf("visual = range: got %+v, want [0,0]-[0,5]", rng)
	}
	if ed.ks.ModeID() != ModeNormal {
		t.Fatalf("visual = should return to normal mode, got %v", ed.ks.ModeID())
	}
	drainUntil(t, ed, "edit applied", func() bool {
		return string(b.Slice(0, b.Len())) == "hillo world\n"
	})
}

// Without an LSP server attached, = reports an error rather than panicking.
func TestLspFormatOperatorNoServer(t *testing.T) {
	ed := newTestEditor()
	ed.registerLspBindings()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("hello world\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	ed.dispatchKey("=")
	ed.dispatchKey("w")
	if !ed.infobar.msgErr {
		t.Fatal("expected an error message with no LSP server")
	}
}
