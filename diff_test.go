package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zyedidia/mu/text"
)

type byteSlice []byte

func (b byteSlice) ByteAt(pos int) byte { return b[pos] }
func (b byteSlice) Len() int            { return len(b) }

func TestDiffIdentical(t *testing.T) {
	edits := Diff(byteSlice("hello"), byteSlice("hello"))
	for _, e := range edits {
		if e.Kind != DiffEqual {
			t.Fatalf("identical strings should produce only Equal edits, got %v", e.Kind)
		}
	}
}

func TestDiffInsert(t *testing.T) {
	edits := Diff(byteSlice("ac"), byteSlice("abc"))
	hasInsert := false
	for _, e := range edits {
		if e.Kind == DiffInsert {
			hasInsert = true
			if string(e.Text) != "b" {
				t.Fatalf("insert text: got %q, want %q", e.Text, "b")
			}
		}
	}
	if !hasInsert {
		t.Fatal("should have an insert edit")
	}
}

func TestDiffDelete(t *testing.T) {
	edits := Diff(byteSlice("abc"), byteSlice("ac"))
	hasDelete := false
	for _, e := range edits {
		if e.Kind == DiffDelete {
			hasDelete = true
		}
	}
	if !hasDelete {
		t.Fatal("should have a delete edit")
	}
}

func TestDiffApply(t *testing.T) {
	from := "hello world"
	to := "hello brave new world"

	edits := Diff(byteSlice(from), byteSlice(to))

	// Apply edits to a buffer and verify result.
	b := NewEmptyBuffer()
	b.Insert(0, []byte(from))
	b.modified = false

	b.UndoBarrier()
	pos := 0
	for _, e := range edits {
		switch e.Kind {
		case DiffInsert:
			b.Insert(pos, e.Text)
			pos += e.Length
		case DiffDelete:
			b.Remove(pos, pos+e.Length)
		case DiffEqual:
			pos += e.Length
		}
	}

	got := string(b.Slice(0, b.Len()))
	if got != to {
		t.Fatalf("after apply: got %q, want %q", got, to)
	}
}

func TestSetContentPreservesUndo(t *testing.T) {
	b := NewEmptyBuffer()
	b.Insert(0, []byte("original"))
	b.UndoBarrier()
	b.Insert(8, []byte(" text"))
	// Buffer is now "original text"

	newb := text.NewBufferFromUTF8([]byte("modified text"), text.Options{})
	b.SetContent(newb)

	got := string(b.Slice(0, b.Len()))
	if got != "modified text" {
		t.Fatalf("after SetContent: got %q", got)
	}

	// Undo should work — the diff edits are in the undo tree.
	b.Undo()
	got = string(b.Slice(0, b.Len()))
	// After undo, we should get back to the previous state.
	if got == "modified text" {
		t.Fatal("undo should change the buffer")
	}
}

func TestReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	os.WriteFile(path, []byte("version 1\n"), 0644)

	b, err := NewBuffer([]byte("version 1\n"), path)
	if err != nil {
		t.Fatal(err)
	}
	b.updateModTime()

	// Simulate external modification. Set modTime to the past so the
	// new write is guaranteed to be newer (filesystem mtime granularity
	// can be 1 second).
	b.modTime = time.Now().Add(-2 * time.Second)
	os.WriteFile(path, []byte("version 2\n"), 0644)

	if !b.ExternallyModified() {
		t.Fatal("should detect external modification")
	}

	if err := b.Reload(); err != nil {
		t.Fatal(err)
	}

	got := string(b.Slice(0, b.Len()))
	if got != "version 2\n" {
		t.Fatalf("after reload: got %q", got)
	}

	if b.ExternallyModified() {
		t.Fatal("should not be externally modified after reload")
	}
}

func TestReloadPreservesUndo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	os.WriteFile(path, []byte("aaa\n"), 0644)

	b, err := NewBuffer([]byte("aaa\n"), path)
	if err != nil {
		t.Fatal(err)
	}
	b.UndoBarrier()
	b.Insert(0, []byte("bbb "))
	// Buffer: "bbb aaa\n"

	// External change.
	os.WriteFile(path, []byte("ccc\n"), 0644)
	b.Reload()

	got := string(b.Slice(0, b.Len()))
	if got != "ccc\n" {
		t.Fatalf("after reload: got %q", got)
	}

	// Undo should still work.
	b.Undo()
	got = string(b.Slice(0, b.Len()))
	if got == "ccc\n" {
		t.Fatal("undo after reload should change buffer")
	}
}
