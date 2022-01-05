package buffer

import (
	"unicode"
	"unicode/utf8"

	"github.com/zyedidia/generic"
)

// Unicode is annoying (but better than anything else). A "code point" (rune in
// Go-speak) may need up to 4 bytes to represent it. In general, a code point
// will represent a complete character, but this is not always the case. A
// character with accents may be made up of multiple code points (the code
// point for the original character, and additional code points for each
// accent/marking).  The functions below are meant to help deal with these
// additional "combining" code points. In underlying operations (search,
// replace, etc...), micro will treat a character with combining code points as
// just the original code point.  For rendering, micro will display the
// combining characters. It's not perfect but it's pretty good.

var minMark = rune(unicode.Mark.R16[0].Lo)

func isMark(r rune) bool {
	// Fast path
	if r < minMark {
		return false
	}
	return unicode.In(r, unicode.Mark)
}

// DecodeGraphemeAt returns the grapheme information at the given offset, along
// with the size of the grapheme. A grapheme consists of a rune, and an
// arbitrary number of combining runes, such as accents or other markings to be
// applied to the visualization of the rune.
func (b *Buffer) DecodeGraphemeAt(off int) (rune, []rune, int) {
	r, size := b.DecodeRuneAt(off)
	off += size
	next, s := b.DecodeRuneAt(off)

	var combc []rune
	for isMark(next) {
		combc = append(combc, next)
		size += s
		off += s

		next, s = b.DecodeRuneAt(off)
	}
	return r, combc, size
}

// DecodeRuneBefore returns the rune immediately before the offset and the size
// of the rune.
func (b *Buffer) DecodeRuneBefore(off int) (rune, int) {
	s := b.Slice(off-generic.Min(4, off), off)
	return utf8.DecodeLastRune(s)
}

func (b *Buffer) DecodeGraphemeBefore(off int) (rune, []rune, int) {
	var size int
	r, s := b.DecodeRuneBefore(off)

	var combc []rune
	for isMark(r) {
		combc = append(combc, r)
		size += s
		off -= s

		r, s = b.DecodeRuneBefore(off)
	}
	return r, combc, size + s
}

// UnicodeLoc converts a unicode (line, col) pair to byte position, where col
// is the number of graphemes within the line.
func (b *Buffer) UnicodeLoc(line, col int) int {
	off := b.OffsetAt(line, 0)
	n := 0

	for n < col {
		_, _, sz := b.DecodeGraphemeAt(off)
		off += sz
		n++
	}

	return off
}

// VisualLoc converts a visual (line, col) pair to byte position where col is the visual
// column to go to within the line, taking into account characters with large
// widths.
func (b *Buffer) VisualLoc(line, col int, displayer RuneVisualizer) int {
	if line >= b.NumLines() {
		return b.Len()
	}

	off := b.OffsetAt(line, 0)
	n := 0

	for n < col {
		r, sz := b.DecodeRuneAt(off)
		if r == '\n' || sz == 0 {
			return off
		}
		off += sz
		n += displayer.Size(r, n)
	}

	return off
}
