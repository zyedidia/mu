package main

import (
	"os"
	"path/filepath"
	"testing"
)

// syntaxEditor builds an isolated editor, optionally with user options.toml
// content, and returns it with a Go file on disk.
func syntaxEditor(t *testing.T, userOpts string) (*Editor, string) {
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
	path := filepath.Join(t.TempDir(), "x.go")
	os.WriteFile(path, []byte("package main\n"), 0644)
	return ed, path
}

func TestSyntaxOptionDefault(t *testing.T) {
	ed, path := syntaxEditor(t, "")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	if ed.ActiveView().buf.syntax == nil {
		t.Fatal("syntax should be enabled by default")
	}
}

func TestSyntaxOptionDisabledInConfig(t *testing.T) {
	ed, path := syntaxEditor(t, "syntax = false\n")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	b := ed.ActiveView().buf
	if b.syntax != nil {
		t.Fatal("syntax = false should disable highlighting at open")
	}
	// Rendering degrades to plain text.
	b.HighlightRange(0, b.Len())
	if g := b.SyntaxGroup(0); g != "" {
		t.Fatalf("SyntaxGroup with syntax off = %q, want empty", g)
	}
}

func TestSetSyntaxRuntime(t *testing.T) {
	ed, path := syntaxEditor(t, "")

	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	b := ed.ActiveView().buf
	if b.syntax == nil {
		t.Fatal("syntax should start enabled")
	}

	ed.RunCommand("set syntax false")
	if ed.infobar.msgErr {
		t.Fatalf("set syntax false: %s", ed.infobar.message)
	}
	if b.syntax != nil {
		t.Fatal(":set syntax false should tear down highlighting")
	}

	ed.RunCommand("set syntax true")
	if b.syntax == nil {
		t.Fatal(":set syntax true should rebuild highlighting")
	}
}

func TestSyntaxOptionPerFiletype(t *testing.T) {
	// The option resolves per filetype: off for go, on elsewhere.
	ed, goPath := syntaxEditor(t, "[go]\nsyntax = false\n")

	if err := ed.OpenFile(goPath); err != nil {
		t.Fatal(err)
	}
	if ed.ActiveView().buf.syntax != nil {
		t.Fatal("[go] syntax = false should disable highlighting for go files")
	}

	shPath := filepath.Join(t.TempDir(), "x.sh")
	os.WriteFile(shPath, []byte("echo hi\n"), 0644)
	if err := ed.OpenFile(shPath); err != nil {
		t.Fatal(err)
	}
	if ed.ActiveView().buf.syntax == nil {
		t.Fatal("shell buffer should keep highlighting")
	}
}
