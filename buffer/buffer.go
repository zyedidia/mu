package buffer

import (
	"fmt"
	"log"
	"time"

	"github.com/zyedidia/flare"
	"github.com/zyedidia/gpeg/memo"
	"github.com/zyedidia/ned/buffer/diff"
	"github.com/zyedidia/ned/buffer/text"
	"github.com/zyedidia/ned/buffer/undo"
	"golang.org/x/sync/semaphore"
)

const (
	hashCutoff = 4096 * 16
	diffCutoff = 4096 * 64
)

type Buffer struct {
	*text.Buffer
	undo *undo.UndoTree

	ModTime time.Time
	// this channel will be closed when the buffer exits.
	Exited chan bool

	in       Input
	out      Output
	modified bool
	modhash  []byte

	syntbl      memo.Table
	highlighter *flare.Highlighter
	hisem       *semaphore.Weighted
	matches     *flare.Matches
	minvalid    bool

	refs int

	Opts Options
}

type Input interface {
	Read() ([]byte, error)
	ModTime() (time.Time, error)
	Name() string
}

func NewBuffer(r Input, out Output, Opts Options) (*Buffer, error) {
	data, err := r.Read()
	if err != nil {
		return nil, err
	}
	b, err := text.NewBuffer(data, Opts.Options)
	if err != nil {
		return nil, err
	}

	buf := &Buffer{
		Buffer: b,
		in:     r,
		out:    out,
		Opts:   Opts,
		Exited: make(chan bool),
		hisem:  semaphore.NewWeighted(1),
		refs:   1,
	}
	buf.Opts.Syntax = true

	buf.undo = undo.NewTree(buf, undo.NoCutoff)

	buf.unmodified()

	if buf.Opts.Filetype == nil {
		if f, ok := buf.DetectFiletype(); ok {
			buf.Opts.Filetype = &f
		}
	}

	err = buf.LoadHighlighter()
	if err != nil {
		// TODO: return this error as a warning maybe
		log.Println(err)
	}
	go buf.InitialHighlight()

	return buf, nil
}

// marks this buffer as unmodified
func (b *Buffer) unmodified() {
	b.ModTime = time.Now()
	b.modified = false
	if b.Len() < hashCutoff {
		b.modhash = b.Hash()
	} else {
		b.modhash = nil
	}
}

// Name returns this buffer's name, indicating the output writer.
func (b *Buffer) Name() string {
	in := b.in.Name()
	out := b.out.Name()
	if in == out {
		return in
	}
	return fmt.Sprintf("%s -> %s", in, out)
}

// SetContent modifies the content of this buffer so that it is equivalent to
// newb. This is done by making a diff and applying edits so that undo history
// will be preserved across the arbitrary modification. The input must be a
// text.Buffer to ensure that the content being assigned is in the internal
// format.
func (b *Buffer) SetContent(newb *text.Buffer) {
	// If the text buffers are too large to diff, just replace the current
	// buffer with the new one. Sadly all undo history will be lost.
	if b.Len() >= diffCutoff || newb.Len() >= diffCutoff {
		b.Buffer = newb
		b.undo = undo.NewTree(b, undo.NoCutoff)
		b.modified = true
		return
	}

	// if the buffers are small enough, instead of just replacing the buffers
	// we perform a diff and apply the modifications as edits so that the undo
	// history is not lost.

	edits := diff.Diff(b, newb)

	var pos int
	for _, e := range edits {
		switch e.Kind {
		case diff.OpInsert:
			b.Insert(pos, e.Text)
			pos += e.Length
		case diff.OpDelete:
			b.Remove(pos, pos+e.Length)
		case diff.OpEqual:
			pos += e.Length
		}
	}
}

// Reload the buffer's contents from its input descriptor.
func (b *Buffer) Reload() error {
	data, err := b.in.Read()
	if err != nil {
		return err
	}
	newb, err := text.NewBuffer(data, b.Opts.Options)
	if err != nil {
		return err
	}

	// mark the buffer as unmodified when finished.
	defer b.unmodified()

	b.SetContent(newb)

	return nil
}

func (b *Buffer) Close() {
	b.refs--
	if b.refs == 0 {
		close(b.Exited)
	}
}

// provides:
// undo/redo
// search/replace
// highlighting
// saving
// serialize undo history
