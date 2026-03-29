package main

import (
	"unicode/utf16"

	"github.com/zyedidia/uniseg"
)

// DecodeGraphemeAt returns the grapheme cluster at the given byte offset,
// consisting of a main rune, any combining runes, and the total byte size.
func (b *Buffer) DecodeGraphemeAt(off int) (rune, []rune, int) {
	r, comb, size, _ := uniseg.DecodeAt(b.text, off)
	return r, comb, size
}

// DecodeGraphemeWidthAt is like DecodeGraphemeAt but also returns the display
// width of the grapheme.
func (b *Buffer) DecodeGraphemeWidthAt(off int) (rune, []rune, int, int) {
	return uniseg.DecodeAt(b.text, off)
}

// DecodeGraphemeBefore returns the grapheme cluster immediately before the
// given byte offset.
func (b *Buffer) DecodeGraphemeBefore(off int) (rune, []rune, int) {
	return uniseg.DecodeBefore(b.text, off)
}

// UnicodeLoc converts a (line, col) pair where col is the number of grapheme
// clusters into a byte offset.
func (b *Buffer) UnicodeLoc(line, col int) int {
	off := b.OffsetAt(line, 0)
	n := 0
	for n < col {
		_, _, sz := b.DecodeGraphemeAt(off)
		if sz == 0 {
			break
		}
		off += sz
		n++
	}
	return off
}

// Utf16Loc converts a (line, byte-col) pair to a (line, utf16-col) pair.
// This is needed for LSP which uses UTF-16 offsets.
func (b *Buffer) Utf16Loc(line, col8 int) (int, int) {
	start := b.OffsetAt(line, 0)
	total8 := 0
	total16 := 0
	for total8 < col8 {
		r, sz := b.DecodeRuneAt(start + total8)
		if sz == 0 {
			break
		}
		total8 += sz
		total16 += len(utf16.Encode([]rune{r}))
	}
	return line, total16
}

// Utf8Loc converts a (line, utf16-col) pair to a (line, byte-col) pair.
func (b *Buffer) Utf8Loc(line, col16 int) (int, int) {
	start := b.OffsetAt(line, 0)
	total8 := 0
	total16 := 0
	for total16 < col16 {
		r, sz := b.DecodeRuneAt(start + total8)
		if sz == 0 {
			break
		}
		total8 += sz
		total16 += len(utf16.Encode([]rune{r}))
	}
	return line, total8
}
