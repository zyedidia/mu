package text

import (
	"io"
	"sync"
	"unicode/utf8"
)

const cacheBufSize = 4096 * 4

// A Reader is a value that wraps an io.ReaderAt with a single-slot cache so
// that reads are more efficient.
type Reader struct {
	wrapped io.ReaderAt

	// cached data
	chunk [cacheBufSize]byte
	// size of the chunk
	nchunk  int
	invalid bool

	// the position within the reader that the chunk starts at.
	base    int
	bytebuf [1]byte

	lock sync.Mutex
}

// NewReader returns a new cached Reader wrapping the given io.ReaderAt.
func NewReader(wrapped io.ReaderAt) *Reader {
	r := &Reader{
		wrapped: wrapped,
		invalid: true,
	}
	r.refill(r.base)
	return r
}

func (r *Reader) refill(pos int) {
	// make the cache aligned
	r.base = pos - (pos % cacheBufSize)
	r.nchunk, _ = r.wrapped.ReadAt(r.chunk[:], int64(r.base))
	r.invalid = false
}

// ReadAt implements the io.ReaderAt interface.
func (r *Reader) ReadAt(p []byte, off int64) (n int, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.readAt(p, off)
}

func (r *Reader) readAt(p []byte, off int64) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	end := off + int64(len(p))

	// start of data is in the cache
	hasStart := off >= int64(r.base) && off < int64(r.base+r.nchunk) && !r.invalid

	// end of data is in the cache
	hasEnd := end > int64(r.base) && end <= int64(r.base+r.nchunk) && !r.invalid

	switch {
	case hasStart && hasEnd:
		i := int(off) - r.base
		n := copy(p, r.chunk[i:i+len(p)])
		return n, nil
	case !hasStart:
		// if we don't have the start, refill the cache so that we do.
		r.refill(int(off))
		fallthrough
	default:
		i := int(off) - r.base
		n := copy(p, r.chunk[i:min(r.nchunk, i+len(p))])
		// if we filled up p or reached the end we are done
		if n == 0 {
			return n, io.EOF
		} else if n == len(p) {
			return n, nil
		}
		// otherwise do another read
		n2, err := r.readAt(p[n:], int64(r.base+r.nchunk))
		return n + n2, err
	}
}

// DecodeRuneAt returns the rune at the offset and the size of the rune.
func (r *Reader) DecodeRuneAt(off int) (rune, int) {
	r.lock.Lock()
	defer r.lock.Unlock()

	hasStart := off >= r.base && off < r.base+r.nchunk && !r.invalid
	// a utf8 rune is at most 4 bytes.
	end := off + 4
	hasEnd := end > r.base && end <= r.base+r.nchunk && !r.invalid

	if hasStart && hasEnd {
		return utf8.DecodeRune(r.chunk[off-r.base:])
	}

	var runebuf [4]byte
	n, _ := r.readAt(runebuf[:], int64(off))
	return utf8.DecodeRune(runebuf[:n])
}

// CachedSlice is a wrapper of ReadAt that allocates a slice automatically.
func (r *Reader) CachedSlice(start, end int) []byte {
	buf := make([]byte, end-start)
	n, _ := r.ReadAt(buf, int64(start))
	return buf[:n]
}

// Invalidate ensures that the cache will refill at the next request.
func (r *Reader) Invalidate() {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.invalid = true
}

// DecodeRuneBefore returns the rune immediately before the offset and its size.
func (r *Reader) DecodeRuneBefore(off int) (rune, int) {
	r.lock.Lock()
	defer r.lock.Unlock()

	start := off - 4
	if start < 0 {
		start = 0
	}

	hasStart := start >= r.base && start < r.base+r.nchunk && !r.invalid
	hasEnd := off > r.base && off <= r.base+r.nchunk && !r.invalid

	if hasStart && hasEnd {
		return utf8.DecodeLastRune(r.chunk[start-r.base : off-r.base])
	}

	var runebuf [4]byte
	n := off - start
	r.readAt(runebuf[:n], int64(start))
	return utf8.DecodeLastRune(runebuf[:n])
}

// ByteAt returns the byte at 'pos'.
func (r *Reader) ByteAt(pos int) byte {
	r.lock.Lock()
	defer r.lock.Unlock()

	hasByte := pos >= r.base && pos < r.base+r.nchunk && !r.invalid
	// fastpath
	if hasByte {
		return r.chunk[pos-r.base]
	}
	n, _ := r.readAt(r.bytebuf[:], int64(pos))
	if n == 1 {
		return r.bytebuf[0]
	}
	return 0
}
