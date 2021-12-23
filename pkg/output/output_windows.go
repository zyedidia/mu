package output

import (
	"io"
)

const (
	HasAtomicFile = false
	HasRootFile   = false
)

// AtomicFile is not available on Windows. This is because Windows has no
// functionality to support atomic file writing.
type AtomicFile struct {
	Path string
}

func (afo *AtomicFile) Open() (io.Writer, error) {
	return nil, ErrNoAtomicFile
}

func (afo *AtomicFile) Name() string {
	return afo.Path
}

// RootFile is not supported on Windows.
type RootFile struct {
	RootCmd string
	Path    string
}

func (rf *RootFile) Open() (io.Writer, error) {
	return nil, ErrNoRootFile
}

func (rf *RootFile) Name() string {
	return rf.Path
}
