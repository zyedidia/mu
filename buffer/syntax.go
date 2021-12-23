package buffer

import "github.com/zyedidia/ned/buffer/ftdetect"

// The set of detectors is global for all buffers and lazily initialized.
var ds ftdetect.Detectors

// DetectFiletype analyzes the buffer name/contents to determine the filetype
// and returns the name and a boolean indicating if the it was able to guess
// the filetype.
func (b *Buffer) DetectFiletype() (string, bool) {
	if ds == nil {
		ds = ftdetect.LoadDefaultDetectors()
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
