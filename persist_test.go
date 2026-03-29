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

func TestEscapePath(t *testing.T) {
	got := escapePath("/home/user/file.txt")
	if got != "home%user%file.txt" {
		t.Fatalf("escapePath: got %q", got)
	}
}
