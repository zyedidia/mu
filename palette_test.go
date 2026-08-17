package main

import (
	"os"
	"path/filepath"
	"testing"
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
