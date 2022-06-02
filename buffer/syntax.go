package buffer

import (
	"context"

	"github.com/zyedidia/ftdetect"
	"github.com/zyedidia/gpeg/memo"
	"github.com/zyedidia/gpeg/vm"

	_ "embed"
)

// The set of detectors is global for all buffers and lazily initialized.
var ds ftdetect.Detectors

// LoadFtdetect returns a set of detectors for many programming languages.
func (b *Buffer) LoadFtdetect() ftdetect.Detectors {
	return b.cfg.LoadDetectors()
}

// DetectFiletype analyzes the buffer name/contents to determine the filetype
// and returns the name and a boolean indicating if the it was able to guess
// the filetype.
func (b *Buffer) DetectFiletype() (string, bool) {
	if ds == nil {
		ds = b.LoadFtdetect()
	}

	d := ds.Detect(b.in.Name(), b.GetLine(0))
	if d == nil {
		return "", false
	}
	return d.Name, true
}

func detectFtEarly(cfg Config, in Input) (string, bool) {
	if ds == nil {
		ds = cfg.LoadDetectors()
	}
	d := ds.Detect(in.Name(), nil)
	if d == nil {
		return "", false
	}
	return d.Name, true
}

// Filetype returns this buffer's filetype.
func (b *BufferData) Filetype() string {
	if ft, ok := b.GetStrOpt("filetype"); ok {
		return ft
	}
	return "unknown"
}

// LoadHighlighter initializes the syntax highlighting memoization table and
// loads the current filetype's highlighter.
func (b *BufferData) LoadHighlighter() error {
	if !b.MustGetBoolOpt("syntax") {
		b.highlighter = nil
		b.syntbl = memo.NoneTable{}
		return nil
	}

	b.syntbl = memo.NewTreeTable(512)
	h, err := b.cfg.LoadHighlighter(b.Filetype())
	if err != nil {
		return err
	}
	b.highlighter = h
	return nil
}

// InitialHighlight performs an initial highlight. It does not generate any
// matches but it fills the memoization table with syntax information so that
// subsequent rehighlights are fast. This function acquires the highlighting
// semaphore so that it can be safely run from a separate thread. Once complete
// it sends a notification to the general notification channel so that a redraw
// can occur when initial highlighting is complete.
func (b *BufferData) InitialHighlight() {
	b.hisem.Acquire(context.Background(), 1)
	if b.highlighter == nil {
		return
	}

	b.highlighter.HighlightFunc(b.Buffer.Text(), b.syntbl, nil, &vm.Interval{})
	b.hisem.Release(1)

	Notify <- struct{}{}
}
