package main

import (
	"os"
	"path/filepath"
	"testing"
)

// lspOptionEditor builds an isolated editor whose LSP manager launches
// "cat" for Go files (a harmless stand-in server that exits on stdin EOF),
// optionally with user options.toml content, and returns it with a Go file
// on disk.
func lspOptionEditor(t *testing.T, userOpts string) (*Editor, string) {
	t.Helper()
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	if userOpts != "" {
		os.WriteFile(filepath.Join(configDirOverride, "options.toml"), []byte(userOpts), 0644)
	}
	ed := newTestEditor()
	ed.lspManager = NewLspManager(map[string]LspLanguage{
		"go": {Command: "cat", Ft: "go"},
	}, lspCallbacks{})
	t.Cleanup(ed.lspManager.ShutdownAll)

	path := filepath.Join(t.TempDir(), "x.go")
	os.WriteFile(path, []byte("package main\n"), 0644)
	return ed, path
}

func TestLspOptionDefault(t *testing.T) {
	ed, path := lspOptionEditor(t, "")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	if ed.ActiveView().buf.lspServer == nil {
		t.Fatal("lsp should attach by default")
	}
}

func TestLspOptionDisabledInConfig(t *testing.T) {
	ed, path := lspOptionEditor(t, "lsp = false\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	if ed.ActiveView().buf.lspServer != nil {
		t.Fatal("lsp = false should prevent attaching at open")
	}
	if len(ed.lspManager.servers) != 0 {
		t.Fatal("no server process should have been started")
	}
}

func TestLspOptionPerFiletype(t *testing.T) {
	ed, path := lspOptionEditor(t, "[go]\nlsp = false\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	if ed.ActiveView().buf.lspServer != nil {
		t.Fatal("[go] lsp = false should prevent attaching for Go buffers")
	}
}

func TestSetLspRuntime(t *testing.T) {
	ed, path := lspOptionEditor(t, "")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	b := ed.ActiveView().buf
	if b.lspServer == nil {
		t.Fatal("server should be attached before the test starts")
	}
	b.AddDiagnostic(0, 0, "stale", DiagError)

	// Disabling detaches the buffer, drops its diagnostics, and shuts
	// down the now-unused server.
	ed.RunCommand("set lsp false")
	if b.lspServer != nil {
		t.Fatal("set lsp false should detach the server")
	}
	if len(b.GetDiagnostics()) != 0 {
		t.Fatal("stale diagnostics should be cleared on detach")
	}
	if len(ed.lspManager.servers) != 0 {
		t.Fatal("unused server should be removed from the manager")
	}

	// Re-enabling attaches a fresh server.
	ed.RunCommand("set lsp true")
	if b.lspServer == nil {
		t.Fatal("set lsp true should re-attach a server")
	}
	if len(ed.lspManager.servers) != 1 {
		t.Fatalf("manager servers = %d, want 1", len(ed.lspManager.servers))
	}
}
