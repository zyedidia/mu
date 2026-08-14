package main

import (
	"bytes"
	"io"
	"regexp"
	"testing"
	"time"

	"github.com/zyedidia/gpeg/memo"
	"github.com/zyedidia/mu/text"
	lsp "go.lsp.dev/protocol"
)

// Regression tests for issues found in review of the audit fixes.

// A visual-mode register prefix must not leak into the next operation.
func TestVisualRegisterPrefixDoesNotLeak(t *testing.T) {
	ks := newVimState("hello\nworld\n")

	feedKeys(ks, "vll\"ay") // reg a = "hel"
	feedKeys(ks, "dd")      // must go to the unnamed register only
	if got := string(ks.regs.Get('a').Content); got != "hel" {
		t.Fatalf("register a clobbered by dd: %q", got)
	}

	// Same leak through Escape from visual mode.
	ks = newVimState("hello\n")
	feedKeys(ks, "v\"a")
	feedSpecial(ks, KeyEscape)
	feedKeys(ks, "x")
	if got := string(ks.regs.Get('a').Content); got != "" {
		t.Fatalf("register a clobbered after visual escape: %q", got)
	}
}

// A failed motion aborts the whole operation without touching the buffer
// or the registers (vim behavior).
func TestFailedMotionAbortsOperator(t *testing.T) {
	ks := newVimState("abc\n")
	ks.regs.Set(RegDefault, []byte("keep"), false)

	feedKeys(ks, "d%") // no bracket under cursor
	if bufText(ks) != "abc\n" {
		t.Fatalf("d%% with no bracket modified buffer: %q", bufText(ks))
	}
	if got := string(ks.regs.Get(RegDefault).Content); got != "keep" {
		t.Fatalf("d%% with no bracket clobbered register: %q", got)
	}

	feedKeys(ks, "dfz") // no 'z' on the line
	if bufText(ks) != "abc\n" {
		t.Fatalf("dfz modified buffer: %q", bufText(ks))
	}
	if got := string(ks.regs.Get(RegDefault).Content); got != "keep" {
		t.Fatalf("dfz clobbered register: %q", got)
	}

	feedKeys(ks, "dh") // at column 0
	if bufText(ks) != "abc\n" {
		t.Fatalf("dh at column 0 modified buffer: %q", bufText(ks))
	}
	if got := string(ks.regs.Get(RegDefault).Content); got != "keep" {
		t.Fatalf("dh at column 0 clobbered register: %q", got)
	}
}

// dk on the first line and dj at the bottom do nothing (failed motion).
func TestVerticalDeleteAtBufferEdges(t *testing.T) {
	ks := newVimState("aa\nbb\n")

	feedKeys(ks, "dk") // on the first line
	if bufText(ks) != "aa\nbb\n" {
		t.Fatalf("dk on first line: got %q", bufText(ks))
	}

	feedKeys(ks, "G") // to the bottom
	feedKeys(ks, "dj")
	if bufText(ks) != "aa\nbb\n" {
		t.Fatalf("dj at bottom: got %q", bufText(ks))
	}
}

// ^ must not match at the phantom position after a trailing newline.
func TestSubstituteCaretNoPhantomLine(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("one\ntwo\nthree\n"))

	ed.RunCommand("s {^} > all")
	got := string(b.Slice(0, b.Len()))
	if got != ">one\n>two\n>three\n" {
		t.Fatalf("s/^/>/: got %q, want %q", got, ">one\n>two\n>three\n")
	}
}

// ^ must not match at EOF of a file without a trailing newline.
func TestFindDownCaretNoEOFMatch(t *testing.T) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte("abc"))
	re := regexp.MustCompile("(?m)^")

	loc := b.FindDown(re, 1) // wraps; the only line start is 0
	if loc == nil || loc[0] != 0 {
		t.Fatalf("FindDown(^, 1): loc=%v, want start 0", loc)
	}
}

// A large pure append is the cheap case of the diff and must stay within
// the budget (preserving undo history on reload).
func TestDiffBoundedLargeAppend(t *testing.T) {
	base := bytes.Repeat([]byte("line of text\n"), 8000) // ~100KB
	grown := append(append([]byte{}, base...), bytes.Repeat([]byte("more\n"), 10000)...)

	edits, ok := DiffBounded(byteIndexer(base), byteIndexer(grown), maxReloadDiffNodes)
	if !ok {
		t.Fatal("DiffBounded gave up on a pure append")
	}
	if len(edits) == 0 {
		t.Fatal("no edits returned")
	}
}

// Wholesale SetContent must resync the LSP document and reset syntax state
// instead of silently diverging.
func TestSetContentWholesaleResyncs(t *testing.T) {
	a := make([]byte, 100*1024)
	c := make([]byte, 100*1024)
	for i := range a {
		a[i] = byte('a' + i%13)
		c[i] = byte('A' + i%17)
	}
	b, _ := NewBuffer(a, "/tmp/f.txt")

	// Attach a fake LSP server whose send loop is parked (never
	// initialized), so queued messages are observable in sendq.
	_, c2sW := io.Pipe()
	s2cR, _ := io.Pipe()
	s := newLspServerIO(c2sW, s2cR)
	b.lspServer = s

	// Attach syntax state (no highlighter: reset only).
	b.syntax = &SyntaxState{syntbl: memo.NewTreeTable(512)}

	newb, _ := text.NewBuffer(c, b.text.Opts)
	genBefore := b.syntax.gen
	b.SetContent(newb)

	if len(s.sendq) != 1 {
		t.Fatalf("expected 1 queued didChange, got %d", len(s.sendq))
	}
	if b.syntax.gen == genBefore {
		t.Fatal("syntax state not reset after wholesale replacement")
	}
	if b.syntax.hlEnd != b.Len() {
		t.Fatalf("syntax window not repositioned: hlEnd=%d, len=%d", b.syntax.hlEnd, b.Len())
	}
}

// A stale background highlight (old generation) must not clear the pending
// state that a newer queued pass needs.
func TestFinishBackgroundStaleGeneration(t *testing.T) {
	live := memo.NewTreeTable(512)
	ss := &SyntaxState{
		syntbl:       live,
		gen:          2,
		bgActive:     true,
		pendingEdits: []memo.Edit{{Start: 0, End: 1, Len: 2}},
	}

	stale := memo.NewTreeTable(512)
	ss.finishBackground(stale, 1)
	if ss.syntbl != live {
		t.Fatal("stale pass replaced the live table")
	}
	if !ss.bgActive || len(ss.pendingEdits) != 1 {
		t.Fatal("stale pass cleared pending state belonging to the newer pass")
	}

	fresh := memo.NewTreeTable(512)
	ss.finishBackground(fresh, 2)
	if ss.syntbl != fresh {
		t.Fatal("current pass did not publish its table")
	}
	if ss.bgActive || ss.pendingEdits != nil {
		t.Fatal("current pass did not clear pending state")
	}
}

// Requests to a dead server fail fast instead of waiting out the timeout.
func TestLspDeadServerFailsFast(t *testing.T) {
	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()
	s := newLspServerIO(c2sW, s2cR)

	// Kill both directions and start the handshake: the receive loop's
	// EOF marks the server dead.
	s2cW.Close()
	c2sR.CloseWithError(io.ErrClosedPipe)
	s.Initialize("/tmp", nil, nil)

	deadline := time.Now().Add(3 * time.Second)
	for !s.isDead() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !s.isDead() {
		t.Fatal("server never marked dead")
	}

	start := time.Now()
	_, err := s.request("textDocument/hover", nil, lspRequestTimeout)
	if err == nil {
		t.Fatal("request to dead server succeeded")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("request to dead server took %v", elapsed)
	}
}

// Closing one of two buffers showing the same file must not send didClose
// for the URI the surviving buffer still uses.
func TestReleaseKeepsSharedLspDocumentOpen(t *testing.T) {
	ed := newTestEditor()
	path := "/tmp/mu-shared-test.go"

	_, c2sW := io.Pipe()
	s2cR, _ := io.Pipe()
	s := newLspServerIO(c2sW, s2cR) // parked send loop: sendq observable

	b1 := ed.ActiveView().buf
	b1.Path = path
	b1.lspServer = s

	// Split showing a second buffer of the same file.
	ed.VSplit(nil)
	b2, _ := NewBuffer([]byte("package main\n"), path)
	b2.lspServer = s
	ed.ActiveView().buf = b2
	ed.syncActiveBuffer()

	ed.ClosePane() // closes the pane showing b2

	if b2.lspServer != nil {
		t.Fatal("released buffer kept its server handle")
	}
	if len(s.sendq) != 0 {
		t.Fatalf("didClose was sent for a URI still open elsewhere (%d queued messages)", len(s.sendq))
	}
}

// Multiple inserts at the same position apply in array order (LSP spec).
func TestApplyTextEditsSameStartOrder(t *testing.T) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte("x\n"))

	pos := lsp.Position{Line: 0, Character: 0}
	edits := []lsp.TextEdit{
		{Range: lsp.Range{Start: pos, End: pos}, NewText: "A"},
		{Range: lsp.Range{Start: pos, End: pos}, NewText: "B"},
	}
	applyTextEdits(b, edits)
	if got := string(b.Slice(0, b.Len())); got != "ABx\n" {
		t.Fatalf("same-position inserts: got %q, want %q", got, "ABx\n")
	}
}
