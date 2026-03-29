package text

import (
	"io"
	"sync"

	"golang.org/x/text/transform"
)

// A Buffer stores text data backed by a rope, with caches for efficient
// reading and line lookups. It supports loading text from other encodings and
// line endings into the internal representation of UTF-8 LF.
type Buffer struct {
	reader *Reader
	liner  *Liner
	text   *Rope

	Opts Options

	lock sync.Mutex
}

// NewBuffer loads a buffer from the given raw data, auto-detecting charset
// and line endings.
func NewBuffer(rawdata []byte, opts Options) (*Buffer, error) {
	data, err := readToUTF8LF(rawdata, &opts)
	if err != nil {
		return nil, err
	}
	return NewBufferFromUTF8(data, opts), nil
}

// NewBufferFromUTF8 creates a buffer from bytes that are already UTF-8
// encoded with LF line endings.
func NewBufferFromUTF8(utf8data []byte, opts Options) *Buffer {
	rop := NewRope(utf8data, RopeOptions{
		SplitLen:       4096,
		JoinLen:        4096 / 2,
		RebalanceRatio: 1.2,
		LineSep:        []byte{'\n'},
	})
	return &Buffer{
		reader: NewReader(rop),
		liner:  NewLiner(rop),
		text:   rop,
		Opts:   opts,
	}
}

// GetLine returns the bytes for line i, without the trailing '\n'.
func (b *Buffer) GetLine(i int) []byte {
	start := b.liner.OffsetAt(i, 0)
	end := b.liner.OffsetAt(i+1, 0)
	if end > start && end <= b.Len() && b.ByteAt(end-1) == '\n' {
		end--
	}
	return b.Slice(start, end)
}

// OffsetAt returns the absolute byte offset corresponding to the given
// line/col pair.
func (b *Buffer) OffsetAt(line, col int) int {
	return b.liner.OffsetAt(line, col)
}

// LineColAt converts an absolute byte offset to a line/col pair.
func (b *Buffer) LineColAt(pos int) (line, col int) {
	return b.liner.LineColAt(pos)
}

// LineLen returns the number of bytes in line 'i', excluding the '\n'.
func (b *Buffer) LineLen(i int) int {
	start := b.OffsetAt(i, 0)
	end := b.OffsetAt(i+1, 0)
	if end > start && end <= b.Len() && b.ByteAt(end-1) == '\n' {
		end--
	}
	return end - start
}

// Insert inserts 'val' at byte position 'pos'.
func (b *Buffer) Insert(pos int, val []byte) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.text.Insert(pos, val)
	b.reader.Invalidate()
	b.liner.InvalidateLiner()
}

// Write appends p to the end of the buffer.
func (b *Buffer) Write(p []byte) (int, error) {
	b.Insert(b.Len(), p)
	return len(p), nil
}

// Remove removes the byte range [start:end).
func (b *Buffer) Remove(start, end int) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.text.Remove(start, end)
	b.reader.Invalidate()
	b.liner.InvalidateLiner()
}

// WriteTo writes the buffer contents to the given writer, converting back
// to the original line ending and charset encoding.
func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	if b.Opts.Endings != nil && *b.Opts.Endings == CRLF {
		w = transform.NewWriter(w, ToCRLF{})
	}
	if b.Opts.Charset != nil && *b.Opts.Charset != utf8enc {
		w = transform.NewWriter(w, (*b.Opts.Charset).NewEncoder())
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	return b.text.WriteTo(w)
}

// Bytes returns the entire buffer as a byte slice.
func (b *Buffer) Bytes() []byte {
	buf := make([]byte, b.Len())
	n, _ := b.text.ReadAt(buf, 0)
	return buf[:n]
}

// Len returns the number of bytes in the buffer.
func (b *Buffer) Len() int {
	return b.text.Len()
}

// Size returns the number of bytes as int64.
func (b *Buffer) Size() int64 {
	return int64(b.Len())
}

// NumLines returns the number of lines in the buffer.
func (b *Buffer) NumLines() int {
	return b.text.NumLines()
}

// Slice returns the bytes in the range [start:end).
func (b *Buffer) Slice(start, end int) []byte {
	return b.reader.CachedSlice(start, end)
}

// ReadAt implements the io.ReaderAt interface.
func (b *Buffer) ReadAt(p []byte, off int64) (int, error) {
	return b.reader.ReadAt(p, off)
}

// ByteAt returns the byte at position pos.
func (b *Buffer) ByteAt(pos int) byte {
	return b.reader.ByteAt(pos)
}

// DecodeRuneAt returns the rune at position off and its byte size.
func (b *Buffer) DecodeRuneAt(off int) (rune, int) {
	return b.reader.DecodeRuneAt(off)
}

// DecodeRuneBefore returns the rune immediately before the offset and its size.
func (b *Buffer) DecodeRuneBefore(off int) (rune, int) {
	return b.reader.DecodeRuneBefore(off)
}
