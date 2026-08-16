package main

import (
	"encoding/gob"
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
	NewView(b, 4).SaveCursorPos()

	b2, err := NewBuffer([]byte("hello world\n"), path)
	if err != nil {
		t.Fatal(err)
	}
	NewView(b2, 4).LoadCursorPos()
	if b2.Cursor().Pos != 5 {
		t.Fatalf("cursor pos: got %d, want 5", b2.Cursor().Pos)
	}
}

func TestPersistViewport(t *testing.T) {
	dataDirOverride = t.TempDir()
	defer func() { dataDirOverride = "" }()

	path := filepath.Join(t.TempDir(), "test.txt")
	text := "aaaaaaaaaa\nbbbbbbbbbb\ncccccccccc\ndddddddddd\n"
	os.WriteFile(path, []byte(text), 0644)

	b, _ := NewBuffer([]byte(text), path)
	v := NewView(b, 4)
	*b.Cursor() = b.Cursor().MoveTo(22)
	v.topline = 1
	v.topcol = 5
	v.stcol = 3
	v.SaveCursorPos()

	b2, _ := NewBuffer([]byte(text), path)
	v2 := NewView(b2, 4)
	v2.LoadCursorPos()
	if b2.Cursor().Pos != 22 {
		t.Fatalf("cursor pos: got %d, want 22", b2.Cursor().Pos)
	}
	if v2.topline != 1 || v2.topcol != 5 || v2.stcol != 3 {
		t.Fatalf("viewport: got (%d,%d,%d), want (1,5,3)", v2.topline, v2.topcol, v2.stcol)
	}
	if v2.savedCursor.Pos != 22 {
		t.Fatalf("savedCursor: got %d, want 22", v2.savedCursor.Pos)
	}
}

func TestPersistViewportOldFormat(t *testing.T) {
	// Cursor files written before viewport persistence contain a bare
	// cursor list; they still restore the cursor.
	dataDirOverride = t.TempDir()
	defer func() { dataDirOverride = "" }()

	path := filepath.Join(t.TempDir(), "test.txt")
	os.WriteFile(path, []byte("hello world\n"), 0644)

	f, err := os.Create(filepath.Join(dataDir(), escapePath(path)+".cursor"))
	if err != nil {
		t.Fatal(err)
	}
	if err := gob.NewEncoder(f).Encode([]Cursor{{Pos: 5}}); err != nil {
		t.Fatal(err)
	}
	f.Close()

	b, _ := NewBuffer([]byte("hello world\n"), path)
	v := NewView(b, 4)
	v.LoadCursorPos()
	if b.Cursor().Pos != 5 {
		t.Fatalf("cursor pos: got %d, want 5", b.Cursor().Pos)
	}
	if v.topline != 0 || v.topcol != 0 || v.stcol != 0 {
		t.Fatalf("viewport should default to origin, got (%d,%d,%d)", v.topline, v.topcol, v.stcol)
	}
}

func TestPersistViewportClamped(t *testing.T) {
	// A viewport saved for a longer file is clamped to the current one.
	dataDirOverride = t.TempDir()
	defer func() { dataDirOverride = "" }()

	path := filepath.Join(t.TempDir(), "test.txt")
	long := ""
	for i := 0; i < 100; i++ {
		long += "some line of text\n"
	}
	b, _ := NewBuffer([]byte(long), path)
	v := NewView(b, 4)
	*b.Cursor() = b.Cursor().MoveTo(b.Len())
	v.topline = 90
	v.topcol = 7
	v.SaveCursorPos()

	b2, _ := NewBuffer([]byte("ab\ncd\n"), path)
	v2 := NewView(b2, 4)
	v2.LoadCursorPos()
	if v2.topline != 2 {
		t.Fatalf("topline: got %d, want 2 (clamped)", v2.topline)
	}
	if v2.topcol != 0 {
		t.Fatalf("topcol: got %d, want 0 (invalid for clamped line)", v2.topcol)
	}
	if b2.Cursor().Pos != b2.Len() {
		t.Fatalf("cursor pos: got %d, want %d", b2.Cursor().Pos, b2.Len())
	}
}

func TestPersistViewportRelocateStable(t *testing.T) {
	// A restored softwrap viewport (including a mid-line topcol) survives
	// the first Relocate: the same rows show as when the session ended.
	dataDirOverride = t.TempDir()
	defer func() { dataDirOverride = "" }()

	path := filepath.Join(t.TempDir(), "test.txt")
	os.WriteFile(path, []byte(fourWrapped), 0644)

	newV := func() (*Buffer, *View) {
		b, _ := NewBuffer([]byte(fourWrapped), path)
		v := NewView(b, 4)
		v.LineNums = false
		v.GutterWidth = 0
		v.SoftWrap = true
		v.ScrollMargin = 0
		v.Resize(5, 4)
		return b, v
	}

	b, v := newV()
	*b.Cursor() = b.Cursor().MoveTo(22)
	if row := cursorScreenRow(v); row != 3 {
		t.Fatalf("cursor screen row = %d, want 3", row)
	}
	v.SaveCursorPos()

	_, v2 := newV()
	v2.LoadCursorPos()
	if v2.topline != 0 || v2.topcol != 5 {
		t.Fatalf("restored top = (%d,%d), want (0,5)", v2.topline, v2.topcol)
	}
	// Relocate keeps the restored viewport and shows the cursor on the
	// same screen row as before.
	if row := cursorScreenRow(v2); row != 3 {
		t.Fatalf("cursor screen row after restore = %d, want 3", row)
	}
	if v2.topline != 0 || v2.topcol != 5 {
		t.Fatalf("top after relocate = (%d,%d), want (0,5)", v2.topline, v2.topcol)
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
