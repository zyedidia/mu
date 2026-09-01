package main

import (
	"os"
	"path/filepath"
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

// --- tabstospaces ---

// tabStateWithOpts is a KeyState whose active view carries the given
// resolved options, the way the editor sets them up per buffer.
func tabStateWithOpts(text string, opts map[string]any) (*KeyState, *View) {
	b := NewEmptyBuffer()
	if text != "" {
		b.text.Insert(0, []byte(text))
	}
	*b.Cursor() = b.Cursor().MoveTo(0)
	v := NewView(b, 4)
	v.Opts = opts
	ks := NewKeyState(b, NewRegisterSet())
	SetupBindings(ks)
	ks.activeView = func() *View { return v }
	return ks, v
}

// Pressing Tab inserts indentation, so it follows tabstospaces rather than
// always writing a tab character; with the option on it aligns to the next
// tab stop, as a real tab would have.
func TestTabInsertHonorsTabsToSpaces(t *testing.T) {
	tests := []struct {
		name string
		opts map[string]any
		text string
		pos  int
		want string
	}{
		{"spaces from column 0", map[string]any{"tabstospaces": true, "tabsize": 4}, "ab\n", 0, "    "},
		{"spaces to the next stop", map[string]any{"tabstospaces": true, "tabsize": 4}, "ab\n", 2, "  "},
		{"spaces on a stop", map[string]any{"tabstospaces": true, "tabsize": 2}, "abcd\n", 4, "  "},
		{"past an existing tab", map[string]any{"tabstospaces": true, "tabsize": 4}, "\tab\n", 2, "   "},
		{"tab character when off", map[string]any{"tabstospaces": false, "tabsize": 4}, "ab\n", 1, "\t"},
		{"tab character when unset", map[string]any{"tabsize": 4}, "ab\n", 1, "\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ks, _ := tabStateWithOpts(tt.text, tt.opts)
			if got := string(tabInsert(ks, ks.Buf(), tt.pos)); got != tt.want {
				t.Fatalf("tabInsert at %d = %q, want %q", tt.pos, got, tt.want)
			}
		})
	}
}

// The insert-mode Tab binding goes through the same path, so an editor with
// tabstospaces on never writes a tab character.
func TestInsertModeTabInsertsSpaces(t *testing.T) {
	dir := t.TempDir()
	configDirOverride = dir
	dataDirOverride = t.TempDir()
	t.Cleanup(func() { configDirOverride, dataDirOverride = "", "" })
	if err := os.WriteFile(filepath.Join(dir, "options.toml"),
		[]byte("tabstospaces = true\ntabsize = 4\n\n[go]\n  tabstospaces = false\n"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct{ file, want string }{
		{"note.txt", "    x\n"},
		{"main.go", "\tx\n"}, // [go] turns the option off
	} {
		t.Run(tt.file, func(t *testing.T) {
			p := filepath.Join(dir, tt.file)
			if err := os.WriteFile(p, []byte("x\n"), 0644); err != nil {
				t.Fatal(err)
			}
			ed := newTestEditor()
			ed.registerCompletionBindings()
			if err := ed.OpenFile(p); err != nil {
				t.Fatal(err)
			}
			ed.dispatchKey("i")
			ed.dispatchKey(KeyTab)

			b := ed.ActiveView().buf
			if got := string(b.text.Slice(0, b.Len())); got != tt.want {
				t.Fatalf("i<Tab> in %s = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

// A buffer that gains a file name only when it is saved has to pick up the
// options for what it turned out to be: until it does, a [filetype] section
// (tabstospaces among them) never applies to it.
func TestSaveAsAppliesFiletypeOptions(t *testing.T) {
	dir := t.TempDir()
	configDirOverride = dir
	dataDirOverride = t.TempDir()
	t.Cleanup(func() { configDirOverride, dataDirOverride = "", "" })
	if err := os.WriteFile(filepath.Join(dir, "options.toml"),
		[]byte("tabstospaces = true\ntabsize = 4\n\n[go]\n  tabstospaces = false\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ed := newTestEditor()
	ed.registerCompletionBindings()
	v := ed.ActiveView()
	v.buf.Path = ""
	v.buf.Filetype = ""
	ed.applyViewOptions(v)
	if tts, _ := GetOptBool(v.Opts, "tabstospaces"); !tts {
		t.Fatal("an unnamed buffer should take the global tabstospaces")
	}

	ed.RunCommand("w " + filepath.Join(dir, "saved.go"))

	if v.buf.Filetype != "go" {
		t.Fatalf("filetype after saving as .go = %q, want go (infobar: %q)", v.buf.Filetype, ed.infobar.message)
	}
	if tts, ok := GetOptBool(ed.ActiveView().Opts, "tabstospaces"); !ok || tts {
		t.Fatalf("tabstospaces after saving as .go = %v, want the [go] section's false", tts)
	}
	ed.dispatchKey("i")
	ed.dispatchKey(KeyTab)
	b := v.buf
	if got := string(b.text.Slice(0, b.Len())); got != "\t" {
		t.Fatalf("i<Tab> after saving as .go = %q, want a tab", got)
	}
}

// Saving under a name of the same type leaves the buffer's setup alone.
func TestSaveAsKeepsFiletypeWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	configDirOverride = dir
	dataDirOverride = t.TempDir()
	t.Cleanup(func() { configDirOverride, dataDirOverride = "", "" })

	p := filepath.Join(dir, "a.go")
	if err := os.WriteFile(p, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ed := newTestEditor()
	if err := ed.OpenFile(p); err != nil {
		t.Fatal(err)
	}
	b := ed.ActiveView().buf
	syntax := b.syntax

	ed.RunCommand("w")

	if b.Filetype != "go" {
		t.Fatalf("filetype = %q, want go", b.Filetype)
	}
	if b.syntax != syntax {
		t.Fatal("a plain save re-initialized the highlighter")
	}
}

// --- indentation detection ---

func TestDetectIndent(t *testing.T) {
	tests := []struct {
		name string
		text string
		want indentStyle
	}{
		{"tabs", "func x() {\n\tif y {\n\t\tz()\n\t}\n}\n", indentTabs},
		{"spaces", "def x():\n    if y:\n        z()\n", indentSpaces},
		{"no indentation", "one\ntwo\nthree\n", indentUnknown},
		{"empty", "", indentUnknown},
		{"mostly tabs", "\ta\n\tb\n c\n", indentTabs},
		{"mostly spaces", "  a\n  b\n\tc\n", indentSpaces},
		{"even split", "\ta\n b\n", indentUnknown},
		// " * ..." lines up under a block comment's opener; counting them
		// would call a tab-indented C file space-indented.
		{"block comment continuation", "/*\n * one\n * two\n */\n\tcode()\n", indentTabs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewEmptyBuffer()
			if tt.text != "" {
				b.text.Insert(0, []byte(tt.text))
			}
			if got := detectIndent(b); got != tt.want {
				t.Fatalf("detectIndent = %d, want %d", got, tt.want)
			}
		})
	}
}

// The detected style is cached with the contents and rescanned when they
// are replaced.
func TestIndentStyleCacheResetOnReload(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("\ta\n\tb\n"), 0644); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	b, err := NewBuffer(data, p)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.IndentStyle(); got != indentTabs {
		t.Fatalf("IndentStyle = %d, want tabs", got)
	}

	if err := os.WriteFile(p, []byte("    a\n    b\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := b.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := b.IndentStyle(); got != indentSpaces {
		t.Fatalf("IndentStyle after reload = %d, want spaces", got)
	}
}

// The precedence: a ":set" beats a section, a section beats the file's own
// indentation, and that beats the top-level default.
func TestIndentDetectionPrecedence(t *testing.T) {
	dir := t.TempDir()
	configDirOverride = dir
	dataDirOverride = t.TempDir()
	t.Cleanup(func() { configDirOverride, dataDirOverride = "", "" })
	if err := os.WriteFile(filepath.Join(dir, "options.toml"),
		[]byte("tabstospaces = true\ntabsize = 4\n\n[go]\n  tabstospaces = false\n"), 0644); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tabbed := "func x() {\n\ty()\n}\n"
	spaced := "def x():\n    y()\n"

	tests := []struct {
		name string
		file string
		text string
		want bool
	}{
		{"file's tabs beat the default", "tabs.txt", tabbed, false},
		{"file's spaces agree with the default", "spaces.txt", spaced, true},
		{"no indentation leaves the default", "flat.txt", "one\ntwo\n", true},
		// The [go] section names this file's type, so it wins even though
		// the file itself is indented with spaces.
		{"section beats the file", "spacey.go", "package main\n\nfunc x() {\n    y()\n}\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := newTestEditor()
			if err := ed.OpenFile(write(tt.file, tt.text)); err != nil {
				t.Fatal(err)
			}
			if got, ok := GetOptBool(ed.ActiveView().Opts, "tabstospaces"); !ok || got != tt.want {
				t.Fatalf("tabstospaces = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("set beats the file", func(t *testing.T) {
		ed := newTestEditor()
		if err := ed.OpenFile(write("tabs2.txt", tabbed)); err != nil {
			t.Fatal(err)
		}
		if got, _ := GetOptBool(ed.ActiveView().Opts, "tabstospaces"); got {
			t.Fatal("setup: the tab-indented file should have detected tabs")
		}
		ed.RunCommand("set tabstospaces true")
		if got, _ := GetOptBool(ed.ActiveView().Opts, "tabstospaces"); !got {
			t.Fatal(":set tabstospaces true was overruled by detection")
		}
	})

	t.Run("detectindent off", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, "options.toml"),
			[]byte("tabstospaces = true\ndetectindent = false\n"), 0644); err != nil {
			t.Fatal(err)
		}
		ed := newTestEditor()
		if err := ed.OpenFile(write("tabs3.txt", tabbed)); err != nil {
			t.Fatal(err)
		}
		if got, _ := GetOptBool(ed.ActiveView().Opts, "tabstospaces"); !got {
			t.Fatal("detection ran with detectindent = false")
		}
	})
}

// Detection reaches what actually types the indentation.
func TestDetectedIndentDrivesTabKey(t *testing.T) {
	dir := t.TempDir()
	configDirOverride = dir
	dataDirOverride = t.TempDir()
	t.Cleanup(func() { configDirOverride, dataDirOverride = "", "" })
	if err := os.WriteFile(filepath.Join(dir, "options.toml"),
		[]byte("tabstospaces = true\ntabsize = 4\n"), 0644); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "tabbed.txt")
	if err := os.WriteFile(p, []byte("a\n\tb\n\tc\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ed := newTestEditor()
	ed.registerCompletionBindings()
	if err := ed.OpenFile(p); err != nil {
		t.Fatal(err)
	}
	ed.dispatchKey("i")
	ed.dispatchKey(KeyTab)

	b := ed.ActiveView().buf
	if got := string(b.text.Slice(0, b.Len())); got != "\ta\n\tb\n\tc\n" {
		t.Fatalf("i<Tab> in a tab-indented file = %q, want a tab", got)
	}
}

// Backspace inside indentation takes out a whole level, the way vim does
// with softtabstop: indentation typed with one Tab comes back out with one
// Backspace rather than four presses.
func TestBackspaceDeletesIndentLevel(t *testing.T) {
	spaces := map[string]any{"tabstospaces": true, "tabsize": 4}
	tabs := map[string]any{"tabstospaces": false, "tabsize": 4}

	tests := []struct {
		name    string
		text    string
		pos     int
		opts    map[string]any
		presses int
		want    string
		wantPos int
	}{
		{"a whole level at once", "    x\n", 4, spaces, 1, "x\n", 0},
		{"back to the previous stop", "        x\n", 8, spaces, 1, "    x\n", 4},
		{"level by level", "        x\n", 8, spaces, 2, "x\n", 0},
		{"part way through a level", "      x\n", 6, spaces, 1, "    x\n", 4},
		{"less than a level", "  x\n", 2, spaces, 1, "x\n", 0},
		// Past the indentation it is an ordinary backspace again.
		{"after the first non-blank", "    ab\n", 6, spaces, 1, "    a\n", 5},
		// A tab is already a level, and mixed indentation is left alone.
		{"tab indentation", "\t\tx\n", 2, tabs, 1, "\tx\n", 1},
		{"spaces after a tab", "\t  x\n", 3, spaces, 1, "\t x\n", 2},
		// Nothing to unindent: joins the lines as before.
		{"at the start of a line", "x\n    y\n", 2, spaces, 1, "x    y\n", 1},
		{"with tabstospaces off", "    x\n", 4, tabs, 1, "   x\n", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ks, _ := tabStateWithOpts(tt.text, tt.opts)
			b := ks.Buf()
			*b.Cursor() = b.Cursor().MoveTo(tt.pos)
			ks.SetMode(ModeInsert)
			for i := 0; i < tt.presses; i++ {
				ks.HandleKey(KeyBacksp)
			}
			if got := string(b.text.Slice(0, b.Len())); got != tt.want {
				t.Fatalf("text = %q, want %q", got, tt.want)
			}
			if got := b.Cursor().Pos; got != tt.wantPos {
				t.Fatalf("cursor at %d, want %d", got, tt.wantPos)
			}
		})
	}
}

// Every cursor unindents its own line.
func TestBackspaceIndentMultiCursor(t *testing.T) {
	ks, _ := tabStateWithOpts("    a\n    b\n", map[string]any{"tabstospaces": true, "tabsize": 4})
	b := ks.Buf()
	*b.Cursor() = b.Cursor().MoveTo(4)
	b.SpawnCursor(10) // after the second line's indent
	if b.NumCursors() != 2 {
		t.Fatalf("cursors = %d, want 2", b.NumCursors())
	}

	ks.SetMode(ModeInsert)
	ks.HandleKey(KeyBacksp)

	if got := string(b.text.Slice(0, b.Len())); got != "a\nb\n" {
		t.Fatalf("text = %q, want %q", got, "a\nb\n")
	}
}

// The indent level follows tabsize.
func TestBackspaceIndentTabSize(t *testing.T) {
	ks, _ := tabStateWithOpts("  x\n", map[string]any{"tabstospaces": true, "tabsize": 2})
	b := ks.Buf()
	*b.Cursor() = b.Cursor().MoveTo(2)
	ks.SetMode(ModeInsert)
	ks.HandleKey(KeyBacksp)
	if got := string(b.text.Slice(0, b.Len())); got != "x\n" {
		t.Fatalf("text = %q, want %q", got, "x\n")
	}
}
