package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	b := NewEmptyBuffer()
	b.Insert(0, []byte("hello world\n"))
	b.Path = path
	b.modified = false // reset from insert
	b.UndoBarrier()
	b.Insert(b.Len(), []byte("second line\n"))

	if !b.Modified() {
		t.Fatal("should be modified")
	}

	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	if b.Modified() {
		t.Fatal("should not be modified after save")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world\nsecond line\n" {
		t.Fatalf("file contents: %q", data)
	}
}

func TestSaveAs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	b := NewEmptyBuffer()
	b.Insert(0, []byte("content\n"))

	if err := b.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	if b.Path != path {
		t.Fatalf("path should be updated: got %q", b.Path)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "content\n" {
		t.Fatalf("file contents: %q", data)
	}
}

func TestSaveCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// Write initial file.
	os.WriteFile(path, []byte("original\n"), 0644)

	b := NewEmptyBuffer()
	b.Insert(0, []byte("updated\n"))
	b.Path = path

	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	// Check backup exists in ~/.config/mu/backup/.
	absPath, _ := filepath.Abs(path)
	encoded := strings.ReplaceAll(absPath, string(filepath.Separator), "%")
	if encoded[0] == '%' {
		encoded = encoded[1:]
	}
	backupPath := filepath.Join(configDir(), "backup", encoded)
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("backup not created at %s: %v", backupPath, err)
	}
	if string(backup) != "original\n" {
		t.Fatalf("backup contents: %q", backup)
	}

	// Check main file is updated.
	data, _ := os.ReadFile(path)
	if string(data) != "updated\n" {
		t.Fatalf("file contents: %q", data)
	}
}

func TestSaveNoFilename(t *testing.T) {
	b := NewEmptyBuffer()
	b.Insert(0, []byte("data"))
	if err := b.Save(); err == nil {
		t.Fatal("should error with no filename")
	}
}

func TestSaveReadonly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "readonly.txt")

	os.WriteFile(path, []byte("data"), 0444)
	defer os.Chmod(path, 0644) // cleanup

	b := NewEmptyBuffer()
	b.Insert(0, []byte("new data"))
	err := b.SaveAs(path)
	if err == nil {
		t.Fatal("should error on readonly file")
	}
}

func TestSaveCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "file.txt")

	b := NewEmptyBuffer()
	b.Insert(0, []byte("nested\n"))

	if err := b.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "nested\n" {
		t.Fatalf("file contents: %q", data)
	}
}

func TestCmdWriteIntegration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	ed := newTestEditor()
	ed.ActiveView().buf.Insert(0, []byte("from editor\n"))
	ed.ActiveView().buf.Path = path

	ed.RunCommand("w")

	if ed.infobar.msgErr {
		t.Fatalf("write error: %s", ed.infobar.message)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "from editor\n" {
		t.Fatalf("file contents: %q", data)
	}
}
