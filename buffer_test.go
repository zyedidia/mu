package main

import (
	"bytes"
	"testing"
)

func TestBufferInsertRemove(t *testing.T) {
	b := NewEmptyBuffer()
	b.UndoBarrier()
	b.Insert(0, []byte("hello world"))
	if got := string(b.Slice(0, b.Len())); got != "hello world" {
		t.Fatalf("after insert: got %q", got)
	}
	if !b.Modified() {
		t.Fatal("buffer should be modified")
	}

	b.UndoBarrier()
	b.Remove(5, 11)
	if got := string(b.Slice(0, b.Len())); got != "hello" {
		t.Fatalf("after remove: got %q", got)
	}

	b.Undo()
	if got := string(b.Slice(0, b.Len())); got != "hello world" {
		t.Fatalf("after undo: got %q", got)
	}

	b.Redo()
	if got := string(b.Slice(0, b.Len())); got != "hello" {
		t.Fatalf("after redo: got %q", got)
	}
}

func TestBufferCursorAdjust(t *testing.T) {
	b := NewEmptyBuffer()
	b.Insert(0, []byte("hello"))
	// Cursor should be adjusted past the insertion.
	if b.Cursor().Pos != 5 {
		t.Fatalf("cursor after insert: got %d, want 5", b.Cursor().Pos)
	}

	// Spawn a second cursor at position 3.
	b.SpawnCursor(3)
	b.UndoBarrier()
	// Insert at position 1 - both cursors should shift.
	b.Insert(1, []byte("XX"))
	if b.cursors[0].Pos != 7 { // was 5, +2 from insert
		t.Fatalf("cursor 0 after insert: got %d, want 7", b.cursors[0].Pos)
	}
	if b.cursors[1].Pos != 5 { // was 3, +2 from insert
		t.Fatalf("cursor 1 after insert: got %d, want 5", b.cursors[1].Pos)
	}
}

func TestBufferCursorMovement(t *testing.T) {
	b := NewEmptyBuffer()
	b.Insert(0, []byte("hello world\nfoo bar\n"))

	c := b.Cursor().MoveTo(0)
	b.PutCursor(c)

	// Right
	c = b.Cursor().Right(b)
	if c.Pos != 1 {
		t.Fatalf("Right: got %d, want 1", c.Pos)
	}

	// LineEnd
	c = c.LineEnd(b)
	if c.Pos != 11 {
		t.Fatalf("LineEnd: got %d, want 11", c.Pos)
	}

	// LineStart
	c = c.LineStart(b)
	if c.Pos != 0 {
		t.Fatalf("LineStart: got %d, want 0", c.Pos)
	}
}

func TestBufferSelection(t *testing.T) {
	b := NewEmptyBuffer()
	b.Insert(0, []byte("hello world"))

	c := Cursor{Pos: 0}
	c = c.SelectTo(5)
	if !c.HasSelection() {
		t.Fatal("should have selection")
	}
	sel := c.Selection(b)
	if !bytes.Equal(sel, []byte("hello")) {
		t.Fatalf("selection: got %q, want %q", sel, "hello")
	}

	c.Deselect(1) // move to end of selection
	if c.HasSelection() {
		t.Fatal("should not have selection after deselect")
	}
	if c.Pos != 5 {
		t.Fatalf("after deselect: got %d, want 5", c.Pos)
	}
}

func TestBufferWordMotion(t *testing.T) {
	b := NewEmptyBuffer()
	b.Insert(0, []byte("hello world foo"))

	c := Cursor{Pos: 0}

	// WordRight should skip "hello" and land on "world"
	c = c.WordRight(b, IsWordChar)
	// Should be at start of "world" (position 6)
	if got := string(b.Slice(c.Pos, c.Pos+5)); got != "world" {
		t.Fatalf("WordRight: at %d, got %q", c.Pos, got)
	}

	// WordLeft should go back to "hello"
	c = c.WordLeft(b, IsWordChar)
	if c.Pos != 0 {
		t.Fatalf("WordLeft: got %d, want 0", c.Pos)
	}
}

func TestBufferMultiCursor(t *testing.T) {
	b := NewEmptyBuffer()
	b.Insert(0, []byte("abcdef"))

	if b.NumCursors() != 1 {
		t.Fatalf("initial cursors: got %d, want 1", b.NumCursors())
	}

	b.SpawnCursor(3)
	if b.NumCursors() != 2 {
		t.Fatalf("after spawn: got %d, want 2", b.NumCursors())
	}

	b.RemoveCursors()
	if b.NumCursors() != 1 {
		t.Fatalf("after remove: got %d, want 1", b.NumCursors())
	}
}
