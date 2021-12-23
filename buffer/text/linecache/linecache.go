package linecache

import (
	"sort"
)

// A LineLooker is a value that supports converting between byte offsets
// and line/col pairs.
type LineLooker interface {
	LineColAt(off int) (line, col int)
	OffsetAt(line, col int) (off int)
	Len() int
	NumLines() int
	IndexAllFunc(start, end int, sep []byte, fn func(idx int) bool)
}

// cache 32 lines at a time.
const cachesz = 32

// A Liner is a cache that wraps a LineLooker so that frequent requests
// to the same locations are cached.
type Liner struct {
	wrapped LineLooker

	// base line number in the cache
	base int

	lines  [cachesz]line
	nlines int

	totlen   int
	totlines int

	invalid bool
}

type line struct {
	idx    int
	num    int
	length int
}

func NewLiner(wrapped LineLooker) *Liner {
	c := &Liner{
		wrapped:  wrapped,
		invalid:  true,
		totlen:   -1,
		totlines: -1,
	}
	c.refill(c.base)
	return c
}

func (c *Liner) refill(n int) {
	// make the cache aligned
	c.invalid = false

	c.base = n - (n % cachesz)
	c.nlines = 0
	c.totlen = c.wrapped.Len()
	c.totlines = c.wrapped.NumLines()

	start := c.wrapped.OffsetAt(c.base, 0)

	c.nlines = 0
	lastidx := start
	c.wrapped.IndexAllFunc(start, c.wrapped.Len(), []byte{'\n'}, func(idx int) bool {
		c.lines[c.nlines] = line{
			idx:    lastidx,
			num:    c.base + c.nlines,
			length: idx - lastidx,
		}
		lastidx = idx + 1
		c.nlines++
		return c.nlines >= cachesz
	})

	// for i := range c.lines {
	// 	if c.base+i >= c.totlines {
	// 		break
	// 	}
	// 	off := c.wrapped.OffsetAt(i+c.base, 0)
	// 	c.lines[i] = line{
	// 		idx: off,
	// 		num: i + c.base,
	// 	}
	// 	c.nlines++
	// }
}

// LineColAt converts a byte offset to a line/col pair.
func (c *Liner) LineColAt(off int) (int, int) {
	if c.nlines <= 0 || c.invalid {
		line, col := c.wrapped.LineColAt(off)
		c.refill(line)
		return line, col
	}

	minl := c.lines[0]
	maxl := c.lines[c.nlines-1]

	minidx := minl.idx
	maxidx := maxl.idx + maxl.length

	if off < minidx || off > maxidx {
		line, col := c.wrapped.LineColAt(off)
		c.refill(line)
		return line, col
	}

	// the offset is in our cache so binary search for it
	i := sort.Search(c.nlines, func(i int) bool {
		return c.lines[i].idx > off
	})

	line := c.lines[i-1].num
	col := off - c.lines[i-1].idx
	return line, col
}

// OffsetAt converts a line/col pair to a byte offset.
func (c *Liner) OffsetAt(line, col int) int {
	if line < 0 {
		return 0
	}

	if line < c.base || line >= c.base+c.nlines || c.invalid {
		// fill the cache so that the request will be in it
		c.refill(line)
	}

	if line < c.base || line >= c.base+c.nlines {
		// still out of range
		return c.wrapped.OffsetAt(line, col)
	}

	ldata := c.lines[line-c.base]
	return ldata.idx + col
}

// Invalidate ensures the next request will refill the cache.
func (c *Liner) Invalidate() {
	c.invalid = true
	c.totlines = -1
	c.totlen = -1
}

// NumLines returns the number of lines in the wrapped looker.
func (c *Liner) NumLines() int {
	if c.totlines < 0 {
		c.totlines = c.wrapped.NumLines()
	}
	return c.totlines
}

// Len returns the number of bytes in the wrapped looker.
func (c *Liner) Len() int {
	if c.totlen < 0 {
		c.totlen = c.wrapped.Len()
	}
	return c.totlen
}
