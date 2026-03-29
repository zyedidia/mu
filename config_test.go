package main

import (
	"testing"
)

func TestLoadOptions(t *testing.T) {
	data := []byte(`
autoindent = true
tabsize = 4
theme = "monokai"

[makefile]
tabstospaces = false

["glob:*.md"]
softwrap = true
`)
	opts, err := LoadOptions(data)
	if err != nil {
		t.Fatal(err)
	}

	if v, ok := opts.top["autoindent"].(bool); !ok || !v {
		t.Fatal("autoindent should be true")
	}
	if v, ok := opts.top["tabsize"].(int64); !ok || v != 4 {
		t.Fatalf("tabsize: got %v", opts.top["tabsize"])
	}
}

func TestOptionsResolve(t *testing.T) {
	data := []byte(`
tabsize = 4
tabstospaces = true
theme = "monokai"

[makefile]
tabstospaces = false

["glob:*.md"]
softwrap = true
`)
	opts, err := LoadOptions(data)
	if err != nil {
		t.Fatal(err)
	}

	// Default resolution (no filetype match).
	m := opts.Resolve("foo.go", "go")
	if v, ok := GetOptBool(m, "tabstospaces"); !ok || !v {
		t.Fatal("default tabstospaces should be true")
	}
	// theme is global, should not appear in buffer options.
	if _, ok := m["theme"]; ok {
		t.Fatal("theme should not be in buffer options")
	}

	// Filetype match.
	m = opts.Resolve("Makefile", "makefile")
	if v, ok := GetOptBool(m, "tabstospaces"); !ok || v {
		t.Fatal("makefile tabstospaces should be false")
	}

	// Glob match.
	m = opts.Resolve("README.md", "markdown")
	if v, ok := GetOptBool(m, "softwrap"); !ok || !v {
		t.Fatal("*.md softwrap should be true")
	}
}

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	// Should have a theme set (embedded default or user override).
	if cfg.GlobalStrOpt("theme") == "" {
		t.Fatal("theme should not be empty")
	}
	// tabsize should be set from defaults.
	tabsize, ok := GetOptInt(cfg.BufferOptions("foo.go", "go"), "tabsize")
	if !ok || tabsize == 0 {
		t.Fatalf("tabsize: got %d, ok=%v", tabsize, ok)
	}
}

func TestSetGlobalOpt(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	// Valid change.
	if err := cfg.SetGlobalOpt("theme", "gruvbox"); err != nil {
		t.Fatal(err)
	}
	if cfg.GlobalStrOpt("theme") != "gruvbox" {
		t.Fatal("theme should be gruvbox")
	}
	// Type mismatch.
	if err := cfg.SetGlobalOpt("theme", 42); err == nil {
		t.Fatal("should reject int for string option")
	}
}

func TestLoadLspLanguages(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	langs, err := cfg.LoadLspLanguages()
	if err != nil {
		t.Fatal(err)
	}
	goLang, ok := langs["go"]
	if !ok {
		t.Fatal("missing go language")
	}
	if goLang.Command != "gopls" {
		t.Fatalf("go command: got %q", goLang.Command)
	}
}
