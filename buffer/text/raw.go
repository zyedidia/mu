package text

import (
	"io"
)

// RawText is the interface used for the underlying buffer data structure that
// stores the actual bytes of the buffer. This may be implemented as an array
// of bytes, buffer gap, rope, piece table, or something else.
type RawText interface {
	Insert(start int, value []byte)
	Remove(start, end int)
	ReadAt(p []byte, off int64) (int, error)

	OffsetAt(line, col int) int
	LineColAt(pos int) (line, col int)

	Len() int
	NumLines() int

	io.WriterTo
}
