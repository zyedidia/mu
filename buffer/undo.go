package buffer

import (
	"encoding/gob"
	"log"

	"github.com/zyedidia/gpeg/memo"
	"github.com/zyedidia/mu/buffer/undo"
	"github.com/zyedidia/mu/config"
	"go.lsp.dev/protocol"
)

// An Edit represents an edit to the document that can be undone.
type Edit struct {
	Start, End int
	Text       []byte
	Sub        []byte
	C          Cursor
}

func init() {
	gob.RegisterName("github.com/zyedidia/mu/buffer.Edit", &Edit{})
}

func (e *Edit) State() Cursor {
	return e.C
}

// Do modifies the base text buffer to remove the deleted range and insert
// text.
func (e *Edit) Do(buf *BufferData) {
	e.Sub = buf.Buffer.Slice(e.Start, e.End)
	buf.edit(e.Start, e.End, e.Text)
}

// Undo modifies the base text buffer to remove Text and insert the substring.
func (e *Edit) Undo(buf *BufferData) {
	buf.edit(e.Start, e.Start+len(e.Text), e.Sub)
}

func (b *BufferData) edit(start, end int, val []byte) {
	b.lspSendEdit(start, end, val)

	b.Buffer.Remove(start, end)
	b.Buffer.Insert(start, val)
	b.modified = true
	b.minvalid = true

	move := func(p int, inclusive bool) int {
		// move for deletion
		if p >= start && p < end {
			p = start
		} else if p >= start {
			p -= end - start
		}
		// move for insertion
		if p > start || (inclusive && p == start) {
			p += len(val)
		}
		return p
	}

	for _, pb := range b.parents {
		for i, c := range pb.cursors {
			pb.cursors[i].Orig[0] = move(c.Orig[0], true)
			pb.cursors[i].Orig[1] = move(c.Orig[1], true)
			pb.cursors[i].Sel[0] = move(c.Sel[0], true)
			pb.cursors[i].Sel[1] = move(c.Sel[1], true)
			pb.cursors[i].Pos = move(c.Pos, true)
			pb.cursors[i].Complete = move(c.Complete, false)
		}
	}

	b.syntbl.ApplyEdit(memo.Edit{
		Start: start,
		End:   end,
		Len:   len(val),
	})
}

func (b *BufferData) lspSendEdit(start, end int, text []byte) {
	if b.Lsp == nil {
		return
	}

	b.lspVersion++
	change := protocol.TextDocumentContentChangeEvent{
		Range: protocol.Range{
			Start: b.LspPosition(b.LineColAt(start)),
			End:   b.LspPosition(b.LineColAt(end)),
		},
		Text: string(text),
	}

	b.Lsp.DidChange(b.in.FullName(), b.lspVersion, []protocol.TextDocumentContentChangeEvent{change})
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

func (b *Buffer) FromLspPosition(pos protocol.Position) int {
	return b.OffsetAt(b.Utf8Loc(int(pos.Line), int(pos.Character)))
}

func (b *Buffer) ApplyLspEdits(edits []protocol.TextEdit) {
	for _, e := range edits {
		start := b.FromLspPosition(e.Range.Start)
		end := b.FromLspPosition(e.Range.End)
		b.Remove(start, end)
		if len(e.NewText) != 0 {
			b.Insert(start, []byte(e.NewText))
		}
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

func (b *BufferData) SerializeUndo(fs config.WriteFS, fname string) error {
	f, err := fs.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()
	return b.undo.WriteTo(f)
}

func (b *BufferData) loadUndo(fs config.WriteFS, fname string) {
	f, err := fs.Open(fname)
	if err != nil {
		log.Println("error loading undo", err)
		b.undo = undo.NewTree[*BufferData, Cursor](b, undo.NoCutoff)
		return
	}
	defer f.Close()
	t, err := undo.FromReader[*BufferData, Cursor](f, b, undo.NoCutoff)
	if err != nil {
		log.Println("error loading undo", err)
		b.undo = undo.NewTree[*BufferData, Cursor](b, undo.NoCutoff)
		return
	}
	b.undo = t
}
