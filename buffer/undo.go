package buffer

import (
	"encoding/gob"

	"github.com/zyedidia/gpeg/memo"
)

// An Edit represents an edit to the document that can be undone.
type Edit struct {
	Start, End int
	Text       []byte
	Sub        []byte
	C          Cursor
}

func init() {
	gob.Register(Edit{})
}

func (e *Edit) State() Cursor {
	return e.C
}

// Do modifies the base text buffer to remove the deleted range and insert
// text.
func (e *Edit) Do(buf *Buffer) {
	e.Sub = buf.Buffer.Slice(e.Start, e.End)
	buf.edit(e.Start, e.End, e.Text)
}

// Undo modifies the base text buffer to remove Text and insert the substring.
func (e *Edit) Undo(buf *Buffer) {
	buf.edit(e.Start, e.Start+len(e.Text), e.Sub)
}

func (b *Buffer) edit(start, end int, val []byte) {
	b.Buffer.Remove(start, end)
	b.Buffer.Insert(start, val)
	b.modified = true
	b.minvalid = true

	for i, c := range b.cursors {
		p := c.Pos
		// move for deletion
		if c.Pos >= start && c.Pos < end {
			p = start
		} else if c.Pos >= start {
			p -= end - start
		}
		// move for insertion
		if p >= start {
			p += len(val)
		}
		b.cursors[i].Pos = p
	}

	b.syntbl.ApplyEdit(memo.Edit{
		Start: start,
		End:   end,
		Len:   len(val),
	})
}

// Insert val at pos and apply the change to the undo tree.
func (b *Buffer) Insert(pos int, val []byte) {
	b.Edit(&Edit{
		Start: pos,
		End:   pos,
		Text:  val,
	})
}

// Remove the range [start:end) and apply the change to the undo tree.
func (b *Buffer) Remove(start, end int) {
	b.Edit(&Edit{
		Start: start,
		End:   end,
		Text:  []byte{},
	})
}

// Edit applies the given edit to the buffer and undo tree.
func (b *Buffer) Edit(e *Edit) {
	e.C = *b.Cursor()
	b.undo.Apply(e)
}

// Undo the previous modification.
func (b *Buffer) Undo() {
	c, ok := b.undo.PrevState()
	b.undo.Undo()
	if ok {
		b.PutCursor(c)
	}
}

// UndoBarrier records a barrier for the undo so that the next event will not
// coalesce with the previous event.
func (b *Buffer) UndoBarrier() {
	b.undo.Barrier()
}

// Redo the most recent modification.
func (b *Buffer) Redo() {
	if ep, ok := b.undo.MostRecent(); ok {
		b.PutCursor(b.undo.NextState(ep))
		b.undo.Redo(ep)
	}
}
