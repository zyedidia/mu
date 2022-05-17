package buffer

import (
	"context"

	"github.com/zyedidia/flare"
	"github.com/zyedidia/ftdetect"
	"github.com/zyedidia/gpeg/memo"
	"github.com/zyedidia/gpeg/vm"

	_ "embed"
)

type SyntaxConfig interface {
	LoadDetectors() ftdetect.Detectors
	LoadHighlighter(name string) (*flare.Highlighter, error)
}

var scfg SyntaxConfig

func SetSyntaxConfig(s SyntaxConfig) {
	scfg = s
}

// The set of detectors is global for all buffers and lazily initialized.
var ds ftdetect.Detectors

// LoadFtdetect returns a set of detectors for many programming languages.
func LoadFtdetect() ftdetect.Detectors {
	return scfg.LoadDetectors()
}

// DetectFiletype analyzes the buffer name/contents to determine the filetype
// and returns the name and a boolean indicating if the it was able to guess
// the filetype.
func (b *Buffer) DetectFiletype() (string, bool) {
	if ds == nil {
		ds = LoadFtdetect()
	}

	d := ds.Detect(b.in.Name(), b.GetLine(0))
	if d == nil {
		return "", false
	}
	return d.Name, true
}

// Filetype returns this buffer's filetype.
func (b *Buffer) Filetype() string {
	if b.Opts.Filetype == nil {
		return "unknown"
	}
	return *b.Opts.Filetype
}

// LoadHighlighter initializes the syntax highlighting memoization table and
// loads the current filetype's highlighter.
func (b *Buffer) LoadHighlighter() error {
	if !b.Opts.Syntax {
		b.highlighter = nil
		b.syntbl = memo.NoneTable{}
		return nil
	}

	b.syntbl = memo.NewTreeTable(512)
	h, err := scfg.LoadHighlighter(b.Filetype())
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
func (b *Buffer) InitialHighlight() {
	b.hisem.Acquire(context.Background(), 1)
	if b.highlighter == nil {
		return
	}

	b.highlighter.HighlightFunc(b.Buffer.Text(), b.syntbl, nil, &vm.Interval{})
	b.hisem.Release(1)

	Notify <- struct{}{}
}
