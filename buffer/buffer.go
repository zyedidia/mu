package buffer

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/zyedidia/flare"
	"github.com/zyedidia/gpeg/memo"
	"github.com/zyedidia/mu/buffer/diff"
	"github.com/zyedidia/mu/buffer/text"
	"github.com/zyedidia/mu/buffer/text/endings"
	"github.com/zyedidia/mu/buffer/undo"
	"github.com/zyedidia/mu/pkg/input"
	"github.com/zyedidia/mu/pkg/output"
	"golang.org/x/sync/semaphore"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
)

const (
	hashCutoff = 4096 * 16
	diffCutoff = 4096 * 64
)

type BufferData struct {
	*text.Buffer
	undo    *undo.UndoTree[*BufferData, Cursor]
	parents []*Buffer

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
	redraw      chan struct{}

	refs int

	cfg     Config
	Options map[string]interface{}
}

type Buffer struct {
	*BufferData

	cursors []Cursor
	cur     int
}

type Input interface {
	Read() ([]byte, error)
	ModTime() (time.Time, error)
	Name() string
	FullName() string
}

type ShareFn func(name string) (*BufferData, Cursor)

func NewBuffer(in Input, out Output, cfg Config, redraw chan struct{}, share ShareFn) (b *Buffer, err error) {
	if share != nil {
		dat, c := share(in.FullName())
		if dat != nil {
			b := &Buffer{
				BufferData: dat,
				cursors:    []Cursor{c},
				cur:        0,
			}
			dat.refs++
			dat.parents = append(dat.parents, b)
			return b, nil
		}
	}
	b = &Buffer{
		cursors: loadCursors(cfg.CacheFS(),
			input.EscapePath(in.FullName())+".cursors"),
		cur: 0,
	}
	dat, err := NewBufferData(in, out, cfg, redraw, b)
	if err != nil {
		return nil, err
	}
	b.BufferData = dat
	return b, nil
}

func NewEmptyBuffer(cfg Config, redraw chan struct{}) *Buffer {
	b, err := NewBuffer(input.NewEmpty(), &output.Discard{}, cfg, redraw, nil)
	if err != nil {
		panic(fmt.Sprintf("opening an empty buffer caused an error: %v", err))
	}
	return b
}

func NewNamedEmptyBuffer(name string, cfg Config, redraw chan struct{}) *Buffer {
	b, err := NewBuffer(
		input.NewReader(strings.NewReader(""), name), &output.Discard{},
		cfg, redraw, nil,
	)
	if err != nil {
		panic(fmt.Sprintf("opening an empty buffer caused an error: %v", err))
	}
	return b
}

func NewBufferData(r Input, out Output, cfg Config, redraw chan struct{}, parent *Buffer) (*BufferData, error) {
	data, err := r.Read()
	if err != nil {
		return nil, err
	}

	ftdtct, ftok := detectFtEarly(cfg, r)
	if !ftok {
		ftdtct = "none"
	}

	opts := cfg.GetBufferOptions(r.Name(), ftdtct)
	b, err := text.NewBuffer(data, getTextOpts(opts))
	if err != nil {
		return nil, err
	}

	buf := &BufferData{
		Buffer:  b,
		in:      r,
		out:     out,
		cfg:     cfg,
		Exited:  make(chan bool),
		hisem:   semaphore.NewWeighted(1),
		refs:    1,
		Options: cfg.GetBufferOptions(r.Name(), ftdtct),
		redraw:  redraw,
		parents: []*Buffer{parent},
	}

	buf.loadUndo(cfg.CacheFS(), input.EscapePath(r.FullName())+".undo")

	buf.unmodified()

	// if the filetype is not specified by the buffer options, use the detected
	// one
	if buf.Options["filetype"] == nil {
		if ftok {
			buf.Options["filetype"] = ftdtct
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

func getTextOpts(opts map[string]interface{}) text.Options {
	var charset *encoding.Encoding
	if chopt, ok := GetOpt[string](opts, "encoding"); ok {
		enc, err := htmlindex.Get(chopt)
		if err != nil {
			log.Printf("invalid charset (%s): %v\n", chopt, err)
		} else {
			charset = &enc
		}
	}
	var ends *endings.Type
	if endopt, ok := GetOpt[string](opts, "endings"); ok {
		var endsv endings.Type
		switch endopt {
		case "crlf", "CRLF", "dos":
			endsv = endings.LF
			ends = &endsv
		case "lf", "LF", "unix":
			endsv = endings.CRLF
			ends = &endsv
		}
	}

	return text.Options{
		Charset: charset,
		Endings: ends,
	}
}

// marks this buffer as unmodified
func (b *BufferData) unmodified() {
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
	if b.HasOutput() {
		return b.out.Name()
	}
	return b.in.Name()
}

func (b *Buffer) FullName() string {
	if b.HasOutput() {
		return b.out.FullName()
	}
	return b.in.FullName()
}

func (b *Buffer) HasOutput() bool {
	_, ok := b.out.(*output.Discard)
	return !ok
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
		b.undo = undo.NewTree[*BufferData, Cursor](b.BufferData, undo.NoCutoff)
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
	newb, err := text.NewBuffer(data, b.Opts)
	if err != nil {
		return err
	}

	// mark the buffer as unmodified when finished.
	defer b.unmodified()

	b.SetContent(newb)

	return nil
}

func (b *Buffer) Close() {
	if !b.Modified() {
		b.SerializeCursors(b.cfg.CacheFS(), input.EscapePath(b.FullName())+".cursors")
	}
	b.BufferData.refs--
	if b.BufferData.refs == 0 {
		close(b.Exited)
	}
}

// provides:
// undo/redo
// search/replace
// highlighting
// saving
// serialize undo history
