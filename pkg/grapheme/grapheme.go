package grapheme

import (
	"unicode"
	"unicode/utf8"
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
// combining characters. It's not perfect but it's good enough.

var minMark = rune(unicode.Mark.R16[0].Lo)

func isMark(r rune) bool {
	// Fast path
	if r < minMark {
		return false
	}
	return unicode.In(r, unicode.Mark)
}

func Decode(p []byte) (rune, []rune, int) {
	r, size := utf8.DecodeRune(p)
	p = p[size:]
	next, s := utf8.DecodeRune(p)

	var combc []rune
	for isMark(next) {
		combc = append(combc, next)
		size += s
		p = p[s:]

		next, s = utf8.DecodeRune(p)
	}
	return r, combc, size
}

func DecodeInString(p string) (rune, []rune, int) {
	r, size := utf8.DecodeRuneInString(p)
	p = p[size:]
	next, s := utf8.DecodeRuneInString(p)

	var combc []rune
	for isMark(next) {
		combc = append(combc, next)
		size += s
		p = p[s:]

		next, s = utf8.DecodeRuneInString(p)
	}
	return r, combc, size
}
