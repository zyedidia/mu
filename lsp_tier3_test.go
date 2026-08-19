package main

import "testing"

// :lsp-workspace-symbols prompts for a query, then lists matching symbols
// across the workspace; selecting one jumps to it.
func TestLspWorkspaceSymbolsPalette(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.Path = "/tmp/x.go"
	b.text.Insert(0, []byte("hi\n"))
	b.lspServer = s

	ed.lspWorkspaceSymbols()
	if !ed.infobar.active || ed.infobar.prompt != "Workspace Symbol> " {
		t.Fatalf("workspace symbol prompt not started: active=%v prompt=%q", ed.infobar.active, ed.infobar.prompt)
	}
	for _, r := range "foo" {
		ed.dispatchKey(string(r))
	}
	ed.dispatchKey(KeyEnter)
	drainUntil(t, ed, "workspace symbols palette", func() bool {
		return ed.palette.active && len(ed.palette.items) == 1
	})
	ed.dispatchKey(KeyEnter)
	// fooSymbol's location starts at line 0, character 0.
	if got := b.Cursor().Pos; got != 0 {
		t.Fatalf("jumped to %d, want 0", got)
	}
}

// :lsp-incoming-calls resolves the call-hierarchy item under the cursor and
// lists its callers; selecting one jumps to the caller's definition.
func TestLspIncomingCallsPalette(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.Path = "/tmp/x.go"
	b.text.Insert(0, []byte("func target() {}\n"))
	b.lspServer = s

	ed.lspCallHierarchy(true)
	drainUntil(t, ed, "incoming calls palette", func() bool {
		return ed.palette.active && len(ed.palette.items) == 1
	})
	if got := ed.palette.items[0].label; got == "" {
		t.Fatal("incoming call label empty")
	}
	ed.dispatchKey(KeyEnter)
	// caller's selectionRange starts at line 1, character 0.
	line, _ := b.LineColAt(b.Cursor().Pos)
	if line != 1 {
		t.Fatalf("jumped to line %d, want 1", line)
	}
}

// :lsp-outgoing-calls resolves the call-hierarchy item under the cursor and
// lists what it calls; selecting one jumps to the callee's definition.
func TestLspOutgoingCallsPalette(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.Path = "/tmp/x.go"
	b.text.Insert(0, []byte("func target() {}\ncallee()\ncallee()\n"))
	b.lspServer = s

	ed.lspCallHierarchy(false)
	drainUntil(t, ed, "outgoing calls palette", func() bool {
		return ed.palette.active && len(ed.palette.items) == 1
	})
	ed.dispatchKey(KeyEnter)
	// callee's selectionRange starts at line 2, character 0.
	line, _ := b.LineColAt(b.Cursor().Pos)
	if line != 2 {
		t.Fatalf("jumped to line %d, want 2", line)
	}
}

// :lsp-inlay-hints fetches hints for the whole buffer and stores them for
// gutter display; the cursor-line lookup then surfaces the hint text.
func TestLspInlayHintsCommand(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.Path = "/tmp/x.go"
	b.text.Insert(0, []byte("let x = 1\n"))
	b.lspServer = s

	ed.lspRefreshInlayHints()
	drainUntil(t, ed, "inlay hints", func() bool {
		_, ok := b.GetInlayHintAt(0)
		return ok
	})
	hint, ok := b.GetInlayHintAt(0)
	if !ok || hint.Text != ": int" {
		t.Fatalf("inlay hint at line 0: got %+v, ok=%v", hint, ok)
	}
}

// The new tier-3 palette modes route to the same asynchronous LSP flows as
// their dedicated entry points, and the root palette (Ctrl-P) lists them.
func TestPaletteTier3ModesRouteToLsp(t *testing.T) {
	for _, mode := range []string{"workspace-symbols", "incoming-calls", "outgoing-calls"} {
		t.Run(mode, func(t *testing.T) {
			fake := &fakeLspServer{}
			s := startFakeLspServer(fake, lspCallbacks{}, nil)
			waitReady(t, s)

			ed := newTestEditor()
			b := ed.ActiveView().buf
			b.Path = "/tmp/x.go"
			b.text.Insert(0, []byte("func target() {}\n"))
			b.lspServer = s

			if err := ed.startPalette(mode); err != nil {
				t.Fatal(err)
			}
			if mode == "workspace-symbols" {
				if !ed.infobar.active || ed.infobar.prompt != "Workspace Symbol> " {
					t.Fatalf("did not route to workspace symbol prompt: active=%v prompt=%q", ed.infobar.active, ed.infobar.prompt)
				}
				return
			}
			drainUntil(t, ed, mode+" palette", func() bool {
				return ed.palette.active && len(ed.palette.items) == 1
			})
		})
	}
}

func TestPaletteRootMenuListsTier3Modes(t *testing.T) {
	ed := newTestEditor()
	if err := ed.startPalette(""); err != nil {
		t.Fatal(err)
	}
	labels := make(map[string]bool)
	for _, it := range ed.palette.items {
		labels[it.label] = true
	}
	for _, want := range []string{
		"Workspace Symbols — fuzzy search symbols across the workspace",
		"Incoming Calls — functions that call the symbol under cursor",
		"Outgoing Calls — functions called by the symbol under cursor",
	} {
		if !labels[want] {
			t.Fatalf("root palette missing %q: %v", want, ed.palette.items)
		}
	}
}
