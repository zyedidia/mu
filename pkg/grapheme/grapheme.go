package grapheme

import (
	"unicode/utf8"

	"github.com/zyedidia/mu/pkg/uniseg"
)

type Decoder interface {
	DecodeRuneAt(off int) (rune, int)
	DecodeRuneBefore(off int) (rune, int)
	Slice(start, end int) []byte
	Len() int
}

func Decode(p []byte) (rune, []rune, int) {
	cur, _, _, _ := uniseg.FirstGraphemeCluster(p, -1)
	return toRunes(cur)
}

func DecodeAt(d Decoder, off int) (rune, []rune, int, int) {
	if off < 0 {
		off = 0
	}
	size, width, _ := uniseg.FirstGraphemeClusterDecoder(d, off, -1)
	r, combc, sz := toRunes(d.Slice(off, off+size))
	return r, combc, sz, width
}

func DecodeLast(p []byte) (rune, []rune, int) {
	size := lastGrapheme(p)
	return toRunes(p[len(p)-size:])
}

func DecodeBefore(d Decoder, off int) (rune, []rune, int) {
	if off < 0 {
		return 0, nil, 0
	}
	size, _ := lastGraphemeDec(d, off)
	return toRunes(d.Slice(off-size, off))
}

func lastGraphemeDec(d Decoder, off int) (size, width int) {
	for off-size > 0 {
		sz, ok := lastGraphemeSimpleDec(d, off-size)
		size += sz
		if ok {
			break
		}
	}
	return lastGraphemeFullDec(d, off-size, off)
}

func lastGraphemeSimpleDec(d Decoder, off int) (size int, ok bool) {
	r, sz := d.DecodeRuneBefore(off)
	size += sz
	switch property(graphemeCodePoints, r) {
	case prLF:
		r, sz := d.DecodeRuneBefore(off - sz)
		if r == '\r' {
			return size + sz, true
		}
		return size, true
	case prCR:
		return size, true
	case prControl:
		return size, true
	}
	return size, false
}

func lastGraphemeFullDec(d Decoder, from, to int) (size, width int) {
	state := -1
	for from < to {
		size, width, state = uniseg.FirstGraphemeClusterDecoder(d, from, state)
		from += size
	}
	return
}

func lastGrapheme(b []byte) int {
	var size int
	for len(b)-size > 0 {
		sz, ok := lastGraphemeSimple(b[:len(b)-size])
		size += sz
		if ok {
			break
		}
	}
	return lastGraphemeFull(b[len(b)-size:])
}

func lastGraphemeSimple(b []byte) (size int, ok bool) {
	r, sz := utf8.DecodeLastRune(b)
	size += sz
	switch property(graphemeCodePoints, r) {
	case prLF:
		r, sz := utf8.DecodeLastRune(b[:len(b)-sz])
		if r == '\r' {
			return size + sz, true
		}
		return size, true
	case prCR:
		return size, true
	case prControl:
		return size, true
	}
	return size, false
}

func lastGraphemeFull(b []byte) (width int) {
	state := -1
	for len(b) > 0 {
		var cur []byte
		cur, b, _, state = uniseg.FirstGraphemeCluster(b, state)
		width = len(cur)
	}
	return
}

func DecodeInString(p string) (rune, []rune, int) {
	cur, _, _, _ := uniseg.FirstGraphemeClusterInString(p, -1)
	// TODO: don't allocate a byte slice: create a toRunesString
	return toRunes([]byte(cur))
}

func toRunes(g []byte) (rune, []rune, int) {
	var size int
	r, sz := utf8.DecodeRune(g)
	if sz == len(g) {
		return r, nil, sz
	}
	size += sz
	g = g[sz:]
	var combc []rune
	for len(g) > 0 {
		r, sz := utf8.DecodeRune(g)
		combc = append(combc, r)
		size += sz
		g = g[sz:]
	}
	return r, combc, size
}
