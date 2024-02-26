package buffer

import (
	"unicode/utf16"
	"unicode/utf8"

	"github.com/zyedidia/generic"
	"github.com/zyedidia/mu/pkg/grapheme"
	"go.lsp.dev/protocol"
)

// DecodeGraphemeAt returns the grapheme information at the given offset, along
// with the size of the grapheme. A grapheme consists of a rune, and an
// arbitrary number of combining runes, such as accents or other markings to be
// applied to the visualization of the rune.
func (b *Buffer) DecodeGraphemeAt(off int) (rune, []rune, int) {
	r, comb, size, _ := grapheme.DecodeAt(b, off)
	return r, comb, size
}

func (b *Buffer) DecodeGraphemeWidthAt(off int) (rune, []rune, int, int) {
	return grapheme.DecodeAt(b, off)
}

// DecodeRuneBefore returns the rune immediately before the offset and the size
// of the rune.
func (b *Buffer) DecodeRuneBefore(off int) (rune, int) {
	s := b.Slice(off-generic.Min(4, off), off)
	return utf8.DecodeLastRune(s)
}

func (b *Buffer) DecodeGraphemeBefore(off int) (rune, []rune, int) {
	return grapheme.DecodeBefore(b, off)
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
func (b *Buffer) VisualLoc(line, col int) int {
	if line >= b.NumLines() {
		return b.Len()
	}

	off := b.OffsetAt(line, 0)
	n := 0

	for n < col {
		r, _, sz, width := b.DecodeGraphemeWidthAt(off)
		if r == '\n' || sz == 0 {
			return off
		}
		off += sz
		n += b.vis.Size(r, n, width)
	}

	return off
}

func (b *BufferData) Utf16Loc(line, col8 int) (int, int) {
	start := b.OffsetAt(line, 0)
	total8 := 0
	total16 := 0
	for total8 < col8 {
		r, sz := b.DecodeRuneAt(start + total8)
		total8 += sz
		total16 += len(utf16.Encode([]rune{r}))
	}
	return line, total16
}

func (b *BufferData) Utf8Loc(line, col16 int) (int, int) {
	start := b.OffsetAt(line, 0)
	total8 := 0
	total16 := 0
	for total16 < col16 {
		r, sz := b.DecodeRuneAt(start + total8)
		total8 += sz
		total16 += len(utf16.Encode([]rune{r}))
	}
	return line, total8
}

func (b *BufferData) LspPosition(line, col int) protocol.Position {
	line, col = b.Utf16Loc(line, col)
	return protocol.Position{
		Line:      uint32(line),
		Character: uint32(col),
	}
}
