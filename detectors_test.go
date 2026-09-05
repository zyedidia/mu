package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// userConfig builds an isolated Config whose config directory contains the
// given files, keyed by slash-separated path relative to the config dir.
func userConfig(t *testing.T, files map[string]string) *Config {
	t.Helper()
	dir := t.TempDir()
	configDirOverride = dir
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// --- Detectors ---

func TestDetectorsEmbeddedOnly(t *testing.T) {
	// With no user detectors the embedded set is unchanged.
	cfg := userConfig(t, nil)

	if got := DetectFiletype(cfg, "main.go", nil); got != "go" {
		t.Errorf("main.go = %q, want go", got)
	}
	if got := DetectFiletype(cfg, "s.py", nil); got != "python" {
		t.Errorf("s.py = %q, want python", got)
	}
}

func TestDetectorUserAddsFiletype(t *testing.T) {
	cfg := userConfig(t, map[string]string{
		"detectors/widget.json": `{"name":"widget","exts":[".wdg"]}`,
	})

	if got := DetectFiletype(cfg, "a.wdg", nil); got != "widget" {
		t.Errorf("a.wdg = %q, want widget", got)
	}
	// Embedded detectors are untouched by an unrelated addition.
	if got := DetectFiletype(cfg, "main.go", nil); got != "go" {
		t.Errorf("main.go = %q, want go", got)
	}
}

func TestDetectorUserAddsFilename(t *testing.T) {
	cfg := userConfig(t, map[string]string{
		"detectors/widget.json": `{"name":"widget","files":["Widgetfile"]}`,
	})

	if got := DetectFiletype(cfg, "Widgetfile", nil); got != "widget" {
		t.Errorf("Widgetfile = %q, want widget", got)
	}
}

func TestDetectorUserReplacesByName(t *testing.T) {
	// A user detector named "go" replaces the embedded one wholesale, so the
	// extensions it lists are the only ones that map to go.
	cfg := userConfig(t, map[string]string{
		"detectors/go.json": `{"name":"go","exts":[".go",".gotmpl"]}`,
	})

	if got := DetectFiletype(cfg, "main.go", nil); got != "go" {
		t.Errorf("main.go = %q, want go", got)
	}
	if got := DetectFiletype(cfg, "t.gotmpl", nil); got != "go" {
		t.Errorf("t.gotmpl = %q, want go", got)
	}
}

func TestDetectorUserDisplacesEmbeddedExtension(t *testing.T) {
	// The key case: a user detector claiming an extension an embedded detector
	// already owns, under a different name. The embedded detector must be
	// displaced from that extension — if both stayed registered, ftdetect
	// would resolve the conflict by regex and match neither, detecting nothing.
	cfg := userConfig(t, map[string]string{
		"detectors/myc.json": `{"name":"myc","exts":[".c"]}`,
	})

	if got := DetectFiletype(cfg, "prog.c", nil); got != "myc" {
		t.Errorf("prog.c = %q, want myc", got)
	}
	// The displaced detector keeps its other extensions.
	if got := DetectFiletype(cfg, "prog.C", nil); got != "c" {
		t.Errorf("prog.C = %q, want c (unclaimed extension of the same detector)", got)
	}
}

func TestDetectorUserDisplacesContestedExtension(t *testing.T) {
	// .h is claimed by two embedded detectors (c++ and objective-c), which
	// resolve it between themselves. A user detector claiming it displaces
	// both and wins outright.
	cfg := userConfig(t, nil)
	if got := DetectFiletype(cfg, "a.h", nil); got != "c++" {
		t.Errorf("embedded a.h = %q, want c++", got)
	}

	cfg = userConfig(t, map[string]string{
		"detectors/hdr.json": `{"name":"chdr","exts":[".h"]}`,
	})
	if got := DetectFiletype(cfg, "a.h", nil); got != "chdr" {
		t.Errorf("a.h = %q, want chdr", got)
	}
}

// An extension claimed by two detectors is resolved by the pair: the common
// language wins on the filename alone, and the other takes over when the
// first line proves it. Before these detectors carried regexes, ftdetect
// could pick neither and .h files got no filetype at all — and so no
// highlighting, no LSP, and no filetype options.
func TestDetectorContestedExtensionsResolve(t *testing.T) {
	cfg := userConfig(t, nil)
	tests := []struct {
		name, first, want string
	}{
		{"a.h", "", "c++"},
		{"a.h", "#include <stdio.h>", "c++"},
		{"a.h", "#import <Foundation/Foundation.h>", "objective-c"},
		{"a.h", "@interface Foo : NSObject", "objective-c"},
		{"a.H", "", "c++"},
		{"a.c", "", "c"},

		{"a.m", "", "objective-c"},
		{"a.m", "#import \"Foo.h\"", "objective-c"},
		{"a.m", "function y = f(x)", "octave"},
		{"a.m", "% a comment", "octave"},
		{"a.m", "1;", "octave"},

		{"a.mm", "", "objective-c"},
		{"a.mm", "@implementation Foo", "objective-c"},
		{"a.mm", ".PH \"title\"", "groff"},
		{"a.me", "", "groff"},
		{"a.ms", "", "groff"},
		{"an.tmac", "", "groff"},

		{"a.fs", "", "fsharp"},
		{"a.fs", "module Foo", "fsharp"},
		{"a.fs", "\\ a forth comment", "forth"},
		{"a.fs", ": square dup * ;", "forth"},
		{"a.forth", "", "forth"},

		{"a.v", "", "verilog"},
		{"a.v", "module counter(input clk);", "verilog"},
		{"a.v", "fn main() {", "v"},
		{"a.v", "pub fn add(a int) int {", "v"},
		{"a.vh", "", "verilog"},
	}
	for _, tt := range tests {
		if got := DetectFiletype(cfg, tt.name, []byte(tt.first)); got != tt.want {
			t.Errorf("%s with first line %q = %q, want %q", tt.name, tt.first, got, tt.want)
		}
	}
}

// Structural guard for the whole embedded set: ftdetect returns a detector
// for an uncontested key outright, but a key claimed by several detectors is
// resolved only by a file or header regex match, and a detector with neither
// can never win. A contested key where nobody can win silently detects
// nothing, so every one of them must still name a filetype from the
// filename alone.
func TestDetectorNoUnresolvableCollisions(t *testing.T) {
	cfg := userConfig(t, nil)
	for key, claimants := range loadDetectors(cfg) {
		if len(claimants) < 2 || key == "" {
			// The empty key holds the extensionless and regex-only
			// detectors, which are meant to need a header to match.
			continue
		}
		probe := key
		if strings.HasPrefix(key, ".") {
			probe = "sample" + key
		}
		if got := DetectFiletype(cfg, probe, nil); got == "" {
			var names []string
			for _, d := range claimants {
				names = append(names, d.Name)
			}
			t.Errorf("%q is claimed by %v but detects nothing: give one of them "+
				"a file regex that matches it (see embed/detectors/c.json)",
				key, names)
		}
	}
}

func TestDetectorHeaderOnlyDetectorsSurvive(t *testing.T) {
	// Displacing an extension must not disturb detectors that match by regex
	// rather than by key: they live in a separate bucket.
	cfg := userConfig(t, map[string]string{
		"detectors/widget.json": `{"name":"widget","exts":[".wdg"]}`,
	})

	if got := DetectFiletype(cfg, "a.wdg", nil); got != "widget" {
		t.Fatalf("a.wdg = %q, want widget", got)
	}
	if got := DetectFiletype(cfg, ".vimrc", nil); got != "vi" {
		t.Errorf(".vimrc = %q, want vi (header-only embedded detector)", got)
	}
}

func TestDetectorMalformedSkipped(t *testing.T) {
	// One unparseable file must not take out the others.
	cfg := userConfig(t, map[string]string{
		"detectors/broken.json": `{"name":"broken",`,
		"detectors/noname.json": `{"exts":[".nn"]}`,
		"detectors/widget.json": `{"name":"widget","exts":[".wdg"]}`,
	})

	if got := DetectFiletype(cfg, "a.wdg", nil); got != "widget" {
		t.Errorf("a.wdg = %q, want widget", got)
	}
	if got := DetectFiletype(cfg, "main.go", nil); got != "go" {
		t.Errorf("main.go = %q, want go", got)
	}
}

func TestDetectorNonJSONIgnored(t *testing.T) {
	cfg := userConfig(t, map[string]string{
		"detectors/README.md":   "not a detector",
		"detectors/widget.json": `{"name":"widget","exts":[".wdg"]}`,
	})

	if got := DetectFiletype(cfg, "a.wdg", nil); got != "widget" {
		t.Errorf("a.wdg = %q, want widget", got)
	}
}

func TestDetectorUserDrivesFiletypeOnOpen(t *testing.T) {
	// End to end: a user detector sets the buffer's filetype at open time,
	// which is what makes a user highlighter reachable.
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	os.MkdirAll(filepath.Join(configDirOverride, "detectors"), 0755)
	os.WriteFile(filepath.Join(configDirOverride, "detectors", "widget.json"),
		[]byte(`{"name":"widget","exts":[".wdg"]}`), 0644)

	ed := newTestEditor()
	path := filepath.Join(t.TempDir(), "a.wdg")
	os.WriteFile(path, []byte("hello\n"), 0644)
	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	if got := ed.ActiveView().buf.Filetype; got != "widget" {
		t.Errorf("buffer filetype = %q, want widget", got)
	}
}

// TestDetectorEnablesUserHighlighter closes the loop the detector support was
// for: before it, a user could drop a highlighter in the config directory but
// nothing would ever detect its filetype, so it could never be reached.
func TestDetectorEnablesUserHighlighter(t *testing.T) {
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	for _, d := range []string{"detectors", "highlighters"} {
		if err := os.MkdirAll(filepath.Join(configDirOverride, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(configDirOverride, "detectors", "widget.json"),
		[]byte(`{"name":"widget","exts":[".wdg"]}`), 0644)
	os.WriteFile(filepath.Join(configDirOverride, "highlighters", "widget.lang"),
		[]byte("ws <- space+\nkeyword <- cap{words{\"WIDGET\"}, \"keyword\"}\ntoken <- ws / keyword\n"), 0644)

	ed := newTestEditor()
	path := filepath.Join(t.TempDir(), "a.wdg")
	os.WriteFile(path, []byte("WIDGET\n"), 0644)
	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}

	b := ed.ActiveView().buf
	if b.Filetype != "widget" {
		t.Fatalf("filetype = %q, want widget", b.Filetype)
	}
	if b.syntax == nil {
		t.Fatal("user highlighter was not loaded for the user filetype")
	}
	b.HighlightRange(0, b.Len())
	if g := b.SyntaxGroup(0); g != "keyword" {
		t.Errorf("SyntaxGroup(0) = %q, want keyword", g)
	}
}

// --- Theme completion ---

func TestCompleteThemeNameEmbeddedOnly(t *testing.T) {
	cfg := userConfig(t, nil)

	names := completeThemeName(cfg, "mo")
	if !contains(names, "monokai") || !contains(names, "molokai") {
		t.Errorf("completeThemeName(mo) = %v, want monokai and molokai", names)
	}
}

func TestCompleteThemeNameIncludesUserThemes(t *testing.T) {
	cfg := userConfig(t, map[string]string{
		"themes/mytheme.yaml": "default:\n  fg: '#ffffff'\n  bg: '#000000'\n",
	})

	names := completeThemeName(cfg, "")
	if !contains(names, "mytheme") {
		t.Errorf("completeThemeName = %v, want it to include mytheme", names)
	}
	// Embedded themes are still offered.
	if !contains(names, "monokai") {
		t.Errorf("completeThemeName = %v, want it to include monokai", names)
	}
}

func TestCompleteThemeNameDeduplicates(t *testing.T) {
	// A user theme shadowing an embedded one is offered once, not twice.
	cfg := userConfig(t, map[string]string{
		"themes/monokai.yaml": "default:\n  fg: '#ffffff'\n  bg: '#000000'\n",
	})

	names := completeThemeName(cfg, "monokai")
	if n := count(names, "monokai"); n != 1 {
		t.Errorf("monokai appears %d times in %v, want 1", n, names)
	}
}

func TestCompleteThemeNamePrefixFilters(t *testing.T) {
	cfg := userConfig(t, map[string]string{
		"themes/zebra.yaml": "default:\n  fg: '#ffffff'\n  bg: '#000000'\n",
	})

	names := completeThemeName(cfg, "ze")
	if len(names) != 1 || names[0] != "zebra" {
		t.Errorf("completeThemeName(ze) = %v, want [zebra]", names)
	}
	if got := completeThemeName(cfg, "nosuch"); len(got) != 0 {
		t.Errorf("completeThemeName(nosuch) = %v, want empty", got)
	}
}

func TestCompleteThemeNameSorted(t *testing.T) {
	cfg := userConfig(t, map[string]string{
		"themes/aaa.yaml": "default:\n  fg: '#ffffff'\n  bg: '#000000'\n",
		"themes/zzz.yaml": "default:\n  fg: '#ffffff'\n  bg: '#000000'\n",
	})

	names := completeThemeName(cfg, "")
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("completeThemeName not sorted: %v", names)
		}
	}
	if names[0] != "aaa" || names[len(names)-1] != "zzz" {
		t.Errorf("completeThemeName = %v, want aaa first and zzz last", names)
	}
}

// TestCompletedThemesLoad ties completion to loading: everything offered as a
// completion must actually be loadable.
func TestCompletedThemesLoad(t *testing.T) {
	cfg := userConfig(t, map[string]string{
		"themes/mytheme.yaml": "default:\n  fg: '#ffffff'\n  bg: '#000000'\n",
	})

	names := completeThemeName(cfg, "")
	if len(names) < 2 {
		t.Fatalf("expected several themes, got %v", names)
	}
	for _, n := range names {
		if _, err := cfg.LoadTheme(n); err != nil {
			t.Errorf("completion offered %q but LoadTheme failed: %v", n, err)
		}
	}
}

func contains(xs []string, s string) bool {
	return count(xs, s) > 0
}

func count(xs []string, s string) int {
	n := 0
	for _, x := range xs {
		if x == s {
			n++
		}
	}
	return n
}
