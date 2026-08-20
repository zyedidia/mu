package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFuzzyScore(t *testing.T) {
	if _, ok := fuzzyScore("pgo", "palette.go"); !ok {
		t.Fatal("subsequence should match")
	}
	if _, ok := fuzzyScore("xyz", "palette.go"); ok {
		t.Fatal("unrelated query should not match")
	}
}

func TestPaletteExactBasenameRanksBeforePathSubsequence(t *testing.T) {
	items := []paletteItem{
		{label: "/var/folders/df/djsxfhc17x95674wsm_g8s980000gn/T/mu-test-2795.txt"},
		{label: "/var/folders/df/djsxfhc17x95674wsm_g8s980000gn/T/TestPaletteBufferMode1046822803/003/a.txt"},
	}
	matches := filterPaletteItems(items)("a.txt")
	if len(matches) != 2 || filepath.Base(matches[0].label) != "a.txt" {
		t.Fatalf("matches = %v", matches)
	}
}

func TestPaletteFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", ".git"), 0755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "notes.txt"), []byte("needle\n"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", ".git", "ignored"), nil, 0644)

	files := paletteFiles(dir)
	if len(files) != 2 || files[0] != "main.go" || files[1] != "sub/notes.txt" {
		t.Fatalf("files = %v", files)
	}
}

func TestPaletteBufferMode(t *testing.T) {
	ed, a, _ := setupBufEditor(t)
	if err := ed.startPalette("buffers"); err != nil {
		t.Fatal(err)
	}
	for _, r := range "a.txt" {
		ed.dispatchKey(string(r))
	}
	if len(ed.palette.items) == 0 || ed.palette.items[0].label != a {
		t.Fatalf("matches = %v", ed.palette.items)
	}
	ed.dispatchKey(KeyEnter)
	if ed.ActiveView().buf.Path != a {
		t.Fatalf("opened %q, want %q", ed.ActiveView().buf.Path, a)
	}
}

func TestPaletteTextMode(t *testing.T) {
	ed := newTestEditor()
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	os.WriteFile(path, []byte("first\nfind me\nlast\n"), 0644)

	items := ed.textItems(dir, []string{"notes.txt"}, "find")
	if len(items) != 1 || items[0].label != "notes.txt:2: find me" {
		t.Fatalf("matches = %v", items)
	}
	items[0].action()
	if ed.ActiveView().buf.Path != path || ed.ActiveView().buf.Cursor().Pos != 6 {
		t.Fatalf("opened %q at %d", ed.ActiveView().buf.Path, ed.ActiveView().buf.Cursor().Pos)
	}
}

func TestPaletteCommand(t *testing.T) {
	ed := newTestEditor()
	ed.RunCommand("palette files")
	if !ed.palette.active || ed.infobar.prompt != "Files> " {
		t.Fatal("palette command did not open file mode")
	}
	ed.infobar.Cancel()
	ed.RunCommand("palette nope")
	if !ed.infobar.msgErr {
		t.Fatal("unknown palette mode should report an error")
	}
}

func TestPaletteLspCodeActions(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.Path = "/tmp/x.go"
	b.text.Insert(0, []byte("hi\n"))
	b.lspServer = s

	start := time.Now()
	if err := ed.startPalette("actions"); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("code-action palette blocked on the LSP request")
	}
	drainUntil(t, ed, "code-action palette", func() bool {
		return ed.palette.active && len(ed.palette.items) == 1
	})
	if got := ed.palette.items[0].label; got != "Make greeting" {
		t.Fatalf("action label = %q", got)
	}
	ed.dispatchKey(KeyEnter)
	if got := string(b.Slice(0, b.Len())); got != "hello\n" {
		t.Fatalf("applied action = %q, want %q", got, "hello\n")
	}
}

// The diagnostics palette lists diagnostics across every open buffer, not
// just the current one, and jumps to the selected one (opening its buffer
// if needed).
func TestPaletteDiagnosticsMode(t *testing.T) {
	ed, a, b := setupBufEditor(t)
	ba := ed.findBuffer(a)
	bb := ed.findBuffer(b)
	ba.AddDiagnostic(0, 1, "bad a", DiagError)
	bb.AddDiagnostic(0, 2, "bad b", DiagWarning)

	if err := ed.startPalette("diagnostics"); err != nil {
		t.Fatal(err)
	}
	if len(ed.palette.items) != 2 {
		t.Fatalf("items = %v", ed.palette.items)
	}
	var gotA bool
	for _, it := range ed.palette.items {
		if it.label == fmt.Sprintf("%s:1: [error] bad a", a) {
			gotA = true
		}
	}
	if !gotA {
		t.Fatalf("missing label for a's diagnostic: %v", ed.palette.items)
	}

	// Filter down to b's diagnostic and jump to it.
	for _, r := range "bad b" {
		ed.dispatchKey(string(r))
	}
	if len(ed.palette.items) != 1 {
		t.Fatalf("filtered items = %v", ed.palette.items)
	}
	ed.dispatchKey(KeyEnter)
	if ed.ActiveView().buf != bb || ed.ActiveView().buf.Cursor().Pos != 2 {
		t.Fatalf("did not jump to b's diagnostic: buf=%q pos=%d", ed.ActiveView().buf.Path, ed.ActiveView().buf.Cursor().Pos)
	}
}

// The "references" and "symbols" palette modes route to the same
// asynchronous LSP requests as their dedicated entry points.
func TestPaletteReferencesAndSymbolsModesRouteToLsp(t *testing.T) {
	for _, mode := range []string{"references", "symbols"} {
		t.Run(mode, func(t *testing.T) {
			fake := &fakeLspServer{}
			s := startFakeLspServer(fake, lspCallbacks{}, nil)
			waitReady(t, s)

			ed := newTestEditor()
			b := ed.ActiveView().buf
			b.Path = "/tmp/x.go"
			b.text.Insert(0, []byte("hi\n"))
			b.lspServer = s

			if err := ed.startPalette(mode); err != nil {
				t.Fatal(err)
			}
			drainUntil(t, ed, mode+" palette", func() bool {
				return ed.palette.active && len(ed.palette.items) == 1
			})
		})
	}
}

// The root palette (Ctrl-P, mode "") lists the new LSP palettes alongside
// the existing ones, and selecting one routes to it.
func TestPaletteRootMenuListsLspModes(t *testing.T) {
	ed := newTestEditor()
	if err := ed.startPalette(""); err != nil {
		t.Fatal(err)
	}
	labels := make(map[string]bool)
	for _, it := range ed.palette.items {
		labels[it.label] = true
	}
	for _, want := range []string{
		"Code Actions — apply an LSP code action",
		"References — find references to symbol under cursor",
		"Symbols — jump to a symbol in this document",
		"Diagnostics — jump to a diagnostic",
	} {
		if !labels[want] {
			t.Fatalf("root palette missing %q: %v", want, ed.palette.items)
		}
	}

	// Selecting "Diagnostics" routes into diagnostics mode.
	for _, r := range "Diagnostics" {
		ed.dispatchKey(string(r))
	}
	if len(ed.palette.items) != 1 {
		t.Fatalf("filtered root items = %v", ed.palette.items)
	}
	ed.dispatchKey(KeyEnter)
	if ed.infobar.prompt != "Diagnostics> " {
		t.Fatalf("did not route to diagnostics mode: prompt = %q", ed.infobar.prompt)
	}
}
