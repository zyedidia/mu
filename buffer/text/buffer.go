package text

import (
	"io"
	"sync"

	"github.com/zyedidia/ned/buffer/text/cache"
	"github.com/zyedidia/ned/buffer/text/endings"
	"github.com/zyedidia/ned/buffer/text/linecache"
	"github.com/zyedidia/ned/buffer/text/linerope"
	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

// A Buffer stores the text data behind an interface, and tracks a set of marks
// in the document. It supports functions for loading text from other
// representations (encodings/line endings) into the internal representation of
// UTF-8 LF.
type Buffer struct {
	*cache.Reader
	*linecache.Liner
	text *linerope.Node

	Opts Options

	// currently this lock protects writes from Insert/Remove and reads from
	// WriteTo from colliding. This is because the only code running from a
	// separate thread at the moment is the backup, which calls WriteTo. In the
	// future, we may need to use this lock to protect ReadAt and Slice as
	// well.
	lock sync.Mutex
}

// Options that can be chosen by the user. If they are left as nil, they will
// be auto-detected and auto-assigned.
type Options struct {
	Charset *encoding.Encoding
	Endings *endings.Type
}

// NewBuffer loads a buffer from the given reader.
func NewBuffer(rawdata []byte, opts Options) (*Buffer, error) {
	data, err := readToUTF8LF(rawdata, &opts)
	if err != nil {
		return nil, err
	}
	return NewBufferFromUTF8(data, opts), nil
}

// NewBufferFromUTF8 creates a buffer from bytes that are known to be UTF8
// encoded.
func NewBufferFromUTF8(utf8data []byte, opts Options) *Buffer {
	rop := linerope.New(utf8data, linerope.Options{
		SplitLen:       4096,
		JoinLen:        4096 / 2,
		RebalanceRatio: 1.2,
		LineSep:        []byte{'\n'},
	})
	return &Buffer{
		Reader: cache.NewReader(rop),
		Liner:  linecache.NewLiner(rop),
		text:   rop,
		Opts:   opts,
	}
}

// GetLine returns the bytes for line i, without the '\n'.
func (b *Buffer) GetLine(i int) []byte {
	start := b.Liner.OffsetAt(i, 0)
	end := b.Liner.OffsetAt(i+1, 0) - 1
	if end < start {
		end = start
	}

	return b.Slice(start, end)
}

// OffsetAt returns the absolute byte offset corresponding to the given
// line/col pair.
func (b *Buffer) OffsetAt(line, col int) int {
	return b.Liner.OffsetAt(line, col)
}

// LineColAt converts an absolute byte offset to a line/col pair.
func (b *Buffer) LineColAt(pos int) (line, col int) {
	return b.Liner.LineColAt(pos)
}

// LineLen returns the number of bytes in line 'i', excluding the '\n'.
func (b *Buffer) LineLen(i int) int {
	start := b.OffsetAt(i, 0)
	end := b.OffsetAt(i+1, 0) - 1
	if end < start {
		end = start
	}
	return end - start
}

// Insert 'val' at 'pos'.
func (b *Buffer) Insert(pos int, val []byte) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.text.Insert(pos, val)
	b.Reader.Invalidate()
	b.Liner.Invalidate()
}

// Remove the range [start:end).
func (b *Buffer) Remove(start, end int) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.text.Remove(start, end)
	b.Reader.Invalidate()
	b.Liner.Invalidate()
}

// WriteTo writes the contents of this buffer to the given writer, and
// re-converts the internal representation back to the external representation
// defined by the options used in this buffer.
func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	if b.Opts.Endings != nil && *b.Opts.Endings == endings.CRLF {
		w = transform.NewWriter(w, endings.ToCRLF{})
	}
	if b.Opts.Charset != nil && *b.Opts.Charset != utf8enc {
		w = transform.NewWriter(w, (*b.Opts.Charset).NewEncoder())
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	return b.text.WriteTo(w)
}

// Bytes returns this buffer as a slice of bytes. This function may perform a
// large allocation if the buffer is large. Please avoid this function if
// possible.
func (b *Buffer) Bytes() []byte {
	buf := make([]byte, b.Len())
	n, _ := b.text.ReadAt(buf, 0)
	return buf[:n]
}

// Text returns the uncached rope underlying this buffer.
// TODO: remove this function.
func (b *Buffer) Text() *linerope.Node {
	return b.text
}

// Len returns the number of bytes in this buffer.
func (b *Buffer) Len() int {
	return b.text.Len()
}

// Size returns the number of bytes in this buffer.
func (b *Buffer) Size() int64 {
	return int64(b.Len())
}

// NumLines returns the number of lines in the buffer.
func (b *Buffer) NumLines() int {
	return b.text.NumLines()
}
