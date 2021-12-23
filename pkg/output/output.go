// Package output provides structures for writing data to different kinds of
// storage. It provides two implementations of file output (atomic and
// non-atomic) and a generalized interface that can be adapted to other mediums
// (e.g., network).
package output

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
)

var (
	ErrNoRootFile   = errors.New("write file as root is not available")
	ErrNoAtomicFile = errors.New("write atomic file is not available")
)

// An Output represents a device that can be opened for writing, and has a
// named identifier.
type Output interface {
	Open() (io.Writer, error)
	Name() string
}

// Discard opens a null writer which writes data into the void.
type Discard struct{}

func (no *Discard) Open() (io.Writer, error) {
	return ioutil.Discard, nil
}

func (no *Discard) Name() string {
	return "Discard"
}

// A File is an Output that writes data to a file at the given path.
type File struct {
	Path string
}

// Open the file for writing. Creates the file if it does not exist, or
// truncates it.
func (fo *File) Open() (io.Writer, error) {
	return os.Create(fo.Path)
}

func (fo *File) Name() string {
	return fo.Path
}

type Stdout struct{}

func (s *Stdout) Open() (io.Writer, error) {
	// return a reference to os.Stdout with a close function that does nothing
	// so that the application doesn't actually close stdout if it tries to
	// close the "opened" writer.
	return &WriterCloser{
		Wr: os.Stdout,
		CloseFn: func() error {
			return nil
		},
	}, nil
}

func (s *Stdout) Name() string {
	return "stdout"
}

// A WriterCloser wraps a writer with an additional close function.
type WriterCloser struct {
	Wr      io.Writer
	CloseFn func() error
}

func (wc *WriterCloser) Write(p []byte) (n int, err error) {
	return wc.Wr.Write(p)
}

func (wc *WriterCloser) Close() error {
	return wc.CloseFn()
}
