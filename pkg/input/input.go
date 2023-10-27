package input

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zyedidia/mu/pkg/home"
	"github.com/zyedidia/mu/pkg/input/parallel"
)

// Input is an interface for defining sources of input data. This may include
// file reads via various mechanisms or over the network, or other forms of
// data (memory buffers or pipes).
type Input interface {
	// Read the entire contents of the input source into a byte slice.
	Read() ([]byte, error)
	// ModTime returns the time when the most recent modification was made to
	// this input source.
	ModTime() (time.Time, error)
	// Name returns the name of this input source
	Name() string
	// FullName returns the full name if possible of this input source. If
	// there are multiple references to the same input source they should
	// always have the same full name (for example, absolute file path).
	FullName() string
}

// A File is an input source for a local file. If the file does not exist,
// reading from it will return an empty byte slice rather than an error.
type File struct {
	Path string
}

func NewFile(path string) (*File, error) {
	p, err := home.Expand(path)
	return &File{
		Path: p,
	}, err
}

// Read opens the file, and reads its contents using all available CPUs.
func (f *File) Read() ([]byte, error) {
	file, err := os.Open(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}
	b := make([]byte, fi.Size())
	n, err := parallel.ReadFull(file, b, runtime.NumCPU())
	return b[:n], err
}

// ModTime returns the most recent modification time of this file reported by
// the file system.
func (f *File) ModTime() (time.Time, error) {
	fi, err := os.Stat(f.Path)
	if err != nil {
		return time.Now(), err
	}
	return fi.ModTime(), nil
}

func (f *File) Name() string {
	return f.Path
}

func (f *File) FullName() string {
	p, _ := filepath.Abs(f.Path)
	return p
}

// A Reader is a general input source that reads from a SizedReaderAt.
type Reader struct {
	rs   SizedReaderAt
	name string
	time time.Time
}

// A SizedReaderAt is a ReaderAt which also supports a Size function.
type SizedReaderAt interface {
	ReadAt(p []byte, off int64) (n int, err error)
	Size() int64
}

type ModTimer interface {
	ModTime() (time.Time, error)
}

// NewReader creates a new reader that wraps a SizedReaderAt.
func NewReader(r SizedReaderAt, name string) *Reader {
	return &Reader{
		rs:   r,
		name: name,
		time: time.Now(),
	}
}

func (r *Reader) Read() ([]byte, error) {
	b := make([]byte, r.rs.Size())
	n, err := parallel.ReadFull(r.rs, b, runtime.NumCPU())
	return b[:n], err
}

func (r *Reader) Name() string {
	return r.name
}

func (r *Reader) FullName() string {
	return r.Name()
}

// If the SizedReaderAt supports the ModTime() function, that will be called,
// otherwise the time of the creation of the Reader will be used.
func (r *Reader) ModTime() (time.Time, error) {
	if mt, ok := r.rs.(ModTimer); ok {
		return mt.ModTime()
	}
	return r.time, nil
}

type Stdin struct {
	time time.Time
}

func NewStdin() *Stdin {
	return &Stdin{
		time: time.Now(),
	}
}

func (s *Stdin) Read() ([]byte, error) {
	return ioutil.ReadAll(os.Stdin)
}

func (s *Stdin) Name() string {
	return "stdin"
}

func (s *Stdin) ModTime() (time.Time, error) {
	return s.time, nil
}

func (s *Stdin) FullName() string {
	return s.Name()
}

type Empty struct {
	time time.Time
}

func NewEmpty() *Empty {
	return &Empty{
		time: time.Now(),
	}
}

func (e *Empty) Read() ([]byte, error) {
	return []byte{}, nil
}

func (e *Empty) Name() string {
	return "empty"
}

func (e *Empty) ModTime() (time.Time, error) {
	return e.time, nil
}

func (e *Empty) FullName() string {
	return e.Name()
}

func EscapePath(path string) string {
	return strings.ReplaceAll(path, string(os.PathSeparator), "%")
}
