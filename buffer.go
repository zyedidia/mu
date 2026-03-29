package main

import (
	"encoding/gob"

	"github.com/zyedidia/mu/text"
)

func init() {
	gob.RegisterName("mu.Edit", &Edit{})
}

// Buffer is the editor-level buffer that wraps a text.Buffer with undo history,
// multiple cursors, and modification tracking.
type Buffer struct {
	text *text.Buffer

	undo    *UndoTree[*Buffer, Cursor]
	cursors []Cursor
	cur     int // active cursor index

	modified    bool
	Path        string
	diagnostics []Diagnostic
}

// NewBuffer creates a new editor buffer from raw file data, auto-detecting
// charset and line endings.
func NewBuffer(data []byte, path string) (*Buffer, error) {
	tb, err := text.NewBuffer(data, text.Options{})
	if err != nil {
		return nil, err
	}
	return newBufferFromText(tb, path), nil
}

// NewBufferFromText creates a new editor buffer from an existing text.Buffer.
func NewBufferFromText(tb *text.Buffer, path string) *Buffer {
	return newBufferFromText(tb, path)
}

func newBufferFromText(tb *text.Buffer, path string) *Buffer {
	b := &Buffer{
		text:    tb,
		cursors: []Cursor{{Num: 0}},
		cur:     0,
		Path:    path,
	}
	b.undo = NewUndoTree[*Buffer, Cursor](b, NoCutoff)
	return b
}

// NewEmptyBuffer creates an empty editor buffer.
func NewEmptyBuffer() *Buffer {
	tb := text.NewBufferFromUTF8([]byte{}, text.Options{})
	return newBufferFromText(tb, "")
}

// --- Delegated text.Buffer methods ---

// Len returns the number of bytes in the buffer.
func (b *Buffer) Len() int { return b.text.Len() }

// NumLines returns the number of lines in the buffer.
func (b *Buffer) NumLines() int { return b.text.NumLines() }

// Slice returns the bytes in range [start:end).
func (b *Buffer) Slice(start, end int) []byte { return b.text.Slice(start, end) }

// ByteAt returns the byte at position pos.
func (b *Buffer) ByteAt(pos int) byte { return b.text.ByteAt(pos) }

// DecodeRuneAt returns the rune at offset and its byte size.
func (b *Buffer) DecodeRuneAt(off int) (rune, int) { return b.text.DecodeRuneAt(off) }

// DecodeRuneBefore returns the rune before offset and its byte size.
func (b *Buffer) DecodeRuneBefore(off int) (rune, int) { return b.text.DecodeRuneBefore(off) }

// OffsetAt returns the byte offset for a line/col pair.
func (b *Buffer) OffsetAt(line, col int) int { return b.text.OffsetAt(line, col) }

// LineColAt returns the line/col pair for a byte offset.
func (b *Buffer) LineColAt(pos int) (int, int) { return b.text.LineColAt(pos) }

// LineLen returns the length of line i in bytes, excluding the '\n'.
func (b *Buffer) LineLen(i int) int { return b.text.LineLen(i) }

// GetLine returns the bytes for line i without the trailing '\n'.
func (b *Buffer) GetLine(i int) []byte { return b.text.GetLine(i) }

// Modified returns whether the buffer has been modified since last save.
func (b *Buffer) Modified() bool { return b.modified }

// ClearModified marks the buffer as unmodified.
func (b *Buffer) ClearModified() { b.modified = false }

// Text returns the underlying text.Buffer.
func (b *Buffer) Text() *text.Buffer { return b.text }

// --- Edit operations ---

// Edit represents a modification to the buffer that can be undone.
type Edit struct {
	Start, End int
	Text       []byte // bytes to insert
	Sub        []byte // bytes that were removed (populated by Do)
	C          Cursor // cursor state at time of edit
}

func (e *Edit) State() Cursor {
	return e.C
}

// Do applies this edit to the buffer.
func (e *Edit) Do(buf *Buffer) {
	e.Sub = buf.text.Slice(e.Start, e.End)
	buf.applyEdit(e.Start, e.End, e.Text)
}

// Undo reverts this edit on the buffer.
func (e *Edit) Undo(buf *Buffer) {
	buf.applyEdit(e.Start, e.Start+len(e.Text), e.Sub)
}

// applyEdit performs a raw edit on the text buffer and adjusts all cursor
// positions accordingly.
func (b *Buffer) applyEdit(start, end int, val []byte) {
	b.text.Remove(start, end)
	b.text.Insert(start, val)
	b.modified = true

	// Adjust all cursor positions for the edit.
	insLen := len(val)
	delLen := end - start
	for i, c := range b.cursors {
		b.cursors[i].Pos = adjustPos(c.Pos, start, delLen, insLen)
		b.cursors[i].Orig[0] = adjustPos(c.Orig[0], start, delLen, insLen)
		b.cursors[i].Orig[1] = adjustPos(c.Orig[1], start, delLen, insLen)
		b.cursors[i].Sel[0] = adjustPos(c.Sel[0], start, delLen, insLen)
		b.cursors[i].Sel[1] = adjustPos(c.Sel[1], start, delLen, insLen)
	}
}

// adjustPos updates a position after a delete+insert at start.
func adjustPos(p, start, delLen, insLen int) int {
	end := start + delLen
	// Adjust for deletion.
	if p >= start && p < end {
		p = start
	} else if p >= end {
		p -= delLen
	}
	// Adjust for insertion.
	if p >= start {
		p += insLen
	}
	return p
}

// Insert inserts val at pos, recording the change in the undo tree.
func (b *Buffer) Insert(pos int, val []byte) {
	b.DoEdit(&Edit{
		Start: pos,
		End:   pos,
		Text:  val,
	})
}

// Remove removes the range [start:end), recording the change in the undo tree.
func (b *Buffer) Remove(start, end int) {
	b.DoEdit(&Edit{
		Start: start,
		End:   end,
		Text:  []byte{},
	})
}

// DoEdit applies an edit and records it in the undo tree.
func (b *Buffer) DoEdit(e *Edit) {
	e.C = *b.Cursor()
	b.undo.Apply(e)
}

// Undo reverts the most recent edit.
func (b *Buffer) Undo() {
	c, ok := b.undo.PrevState()
	b.undo.Undo()
	if ok {
		b.PutCursor(c)
	}
}

// Redo reapplies the most recently undone edit.
func (b *Buffer) Redo() {
	if ep, ok := b.undo.MostRecent(); ok {
		b.PutCursor(b.undo.NextState(ep))
		b.undo.Redo(ep)
	}
}

// UndoBarrier prevents the next edit from coalescing with the previous one.
func (b *Buffer) UndoBarrier() {
	b.undo.Barrier()
}
