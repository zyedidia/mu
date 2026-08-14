package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistCursor(t *testing.T) {
	dataDirOverride = t.TempDir()
	defer func() { dataDirOverride = "" }()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world\n"), 0644)

	b, err := NewBuffer([]byte("hello world\n"), path)
	if err != nil {
		t.Fatal(err)
	}
	*b.Cursor() = b.Cursor().MoveTo(5)
	b.SaveCursorPos()

	b2, err := NewBuffer([]byte("hello world\n"), path)
	if err != nil {
		t.Fatal(err)
	}
	b2.LoadCursorPos()
	if b2.Cursor().Pos != 5 {
		t.Fatalf("cursor pos: got %d, want 5", b2.Cursor().Pos)
	}
}

func TestPersistHistory(t *testing.T) {
	dataDirOverride = t.TempDir()
	defer func() { dataDirOverride = "" }()

	ib := NewInfoBar()
	ib.StartPrompt(":", func(string) {})
	for _, r := range "quit" {
		ib.HandleKey(string(r))
	}
	ib.HandleKey(KeyEnter)

	ib.SaveHistory()

	ib2 := NewInfoBar()
	ib2.LoadHistory()
	if len(ib2.history[":"]) != 1 || ib2.history[":"][0] != "quit" {
		t.Fatalf(": history: %v", ib2.history[":"])
	}
}

func TestPersistUndo(t *testing.T) {
	dataDirOverride = t.TempDir()
	defer func() { dataDirOverride = "" }()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("original\n"), 0644)

	b, err := NewBuffer([]byte("original\n"), path)
	if err != nil {
		t.Fatal(err)
	}
	b.UndoBarrier()
	b.Insert(0, []byte("new "))
	// Simulate saving the buffer to disk so the history is persistable.
	os.WriteFile(path, []byte("new original\n"), 0644)
	b.markUnmodified()
	b.SaveUndoHistory()

	b2, err := NewBuffer([]byte("new original\n"), path)
	if err != nil {
		t.Fatal(err)
	}
	b2.LoadUndoHistory()
	b2.Undo()
	got := string(b2.Slice(0, b2.Len()))
	if got != "original\n" {
		t.Fatalf("after undo: %q", got)
	}
}

// A modified buffer's undo history must not be persisted: its tree
// references states that were never written to the file.
func TestPersistUndoModified(t *testing.T) {
	dataDirOverride = t.TempDir()
	defer func() { dataDirOverride = "" }()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("original\n"), 0644)

	b, err := NewBuffer([]byte("original\n"), path)
	if err != nil {
		t.Fatal(err)
	}
	b.UndoBarrier()
	b.Insert(b.Len(), []byte("x"))
	// Quit without saving: the buffer is modified, so no history is written.
	b.SaveUndoHistory()

	undoPath := filepath.Join(dataDirOverride, escapePath(path)+".undo")
	if _, err := os.Stat(undoPath); err == nil {
		t.Fatal("undo history for a modified buffer should not be saved")
	}
}

// Loading undo history whose content hash doesn't match the buffer must
// discard it. Previously a stale tree could reference offsets beyond the
// buffer end, panicking on the first undo.
func TestPersistUndoStale(t *testing.T) {
	dataDirOverride = t.TempDir()
	defer func() { dataDirOverride = "" }()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("0123456789\n"), 0644)

	b, err := NewBuffer([]byte("0123456789\n"), path)
	if err != nil {
		t.Fatal(err)
	}
	b.UndoBarrier()
	b.Insert(b.Len(), []byte("x"))
	b.markUnmodified()
	b.SaveUndoHistory()

	// Reopen with different (shorter) contents but the same mtime, as when
	// the edits above were never actually written to disk.
	b2, err := NewBuffer([]byte("01234\n"), path)
	if err != nil {
		t.Fatal(err)
	}
	b2.LoadUndoHistory()
	b2.Undo() // must be a no-op, not a panic
	got := string(b2.Slice(0, b2.Len()))
	if got != "01234\n" {
		t.Fatalf("stale undo history applied: %q", got)
	}
}

func TestEscapePath(t *testing.T) {
	got := escapePath("/home/user/file.txt")
	if got != "home%user%file.txt" {
		t.Fatalf("escapePath: got %q", got)
	}
}
