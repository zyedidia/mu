package buffer

import (
	"encoding/gob"
)

// An Edit represents an edit to the document that can be undone.
type Edit struct {
	Start, End int
	Text       []byte
	Sub        []byte
}

func init() {
	gob.Register(Edit{})
}

// Do modifies the base text buffer to remove the deleted range and insert
// text.
func (e *Edit) Do(base interface{}) {
	buf := base.(*Buffer)
	e.Sub = buf.Buffer.Slice(e.Start, e.End)
	buf.Buffer.Remove(e.Start, e.End)
	buf.Buffer.Insert(e.Start, e.Text)
}

// Undo modifies the base text buffer to remove Text and insert the substring.
func (e *Edit) Undo(base interface{}) {
	buf := base.(*Buffer)
	buf.Buffer.Remove(e.Start, e.Start+len(e.Text))
	buf.Buffer.Insert(e.Start, e.Sub)
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
	b.undo.Apply(e)
	b.modified = true
}

// Undo the previous modification.
func (b *Buffer) Undo() {
	b.undo.Undo()
}

// UndoBarrier records a barrier for the undo so that the next event will not
// coalesce with the previous event.
func (b *Buffer) UndoBarrier() {
	b.undo.Barrier()
}

// Redo the most recent modification.
func (b *Buffer) Redo() {
	b.undo.RedoMostRecent()
}
