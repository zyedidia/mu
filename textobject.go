package main

import (
	"unicode"
)

// TextObjectDef defines a text object that computes a byte range around a
// position. The range is [start, end) exclusive.
type TextObjectDef struct {
	Fn func(b *Buffer, pos int, count int) (start, end int)
}

// --- Text object application ---

// execTextObjOp executes the pending operator using a text object range.
func execTextObjOp(ks *KeyState, to TextObjectDef) {
	b := ks.Buf()
	op := ks.Pending()
	if op == nil {
		ks.ResetAction()
		return
	}
	count := ks.RawCount()
	if count == 0 {
		count = 1
	}
	b.UndoBarrier()
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		start, end := to.Fn(b, c.Pos, count)
		if start < end {
			op.Fn(ks, b, start, end)
		}
	}
	ks.ResetAction()
}

// applyTextObjVisual adjusts the visual selection to the text object range.
func applyTextObjVisual(ks *KeyState, to TextObjectDef) {
	b := ks.Buf()
	count := ks.RawCount()
	if count == 0 {
		count = 1
	}
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		start, end := to.Fn(b, c.Pos, count)
		if start < end {
			b.cursors[i].Orig = [2]int{start, start}
			b.cursors[i] = b.cursors[i].SelectTo(end)
			if ks.ModeID() == ModeVisualLine {
				adjustVisualLine(b, &b.cursors[i])
			}
		}
	}
	ks.count = 0
}

// registerTextObject binds a text object under a prefix key (e.g. "i" or "a")
// in operator-pending and visual modes.
func registerTextObject(ks *KeyState, prefix string, key string, to TextObjectDef) {
	ks.modes[ModeOperatorPending].Bindings.Bind(func(ks *KeyState) {
		execTextObjOp(ks, to)
	}, prefix, key)
	ks.modes[ModeVisual].Bindings.Bind(func(ks *KeyState) {
		applyTextObjVisual(ks, to)
	}, prefix, key)
	ks.modes[ModeVisualLine].Bindings.Bind(func(ks *KeyState) {
		applyTextObjVisual(ks, to)
	}, prefix, key)
}

// --- Text object implementations ---

func toInnerWord(b *Buffer, pos int, count int) (int, int) {
	c := Cursor{Pos: pos}
	start := c.WordStart(b, IsWordChar).Pos
	end := pos
	for i := 0; i < count; i++ {
		ec := Cursor{Pos: end}.WordEnd(b, IsWordChar)
		_, _, sz := b.DecodeGraphemeAt(ec.Pos)
		end = ec.Pos + sz
	}
	return start, end
}

func toAroundWord(b *Buffer, pos int, count int) (int, int) {
	start, end := toInnerWord(b, pos, count)
	// Include trailing whitespace.
	for end < b.Len() {
		r, _, sz := b.DecodeGraphemeAt(end)
		if !unicode.IsSpace(r) || r == '\n' || sz == 0 {
			break
		}
		end += sz
	}
	return start, end
}

func toInnerWORD(b *Buffer, pos int, count int) (int, int) {
	c := Cursor{Pos: pos}
	start := c.WordStart(b, IsNotSpace).Pos
	end := pos
	for i := 0; i < count; i++ {
		ec := Cursor{Pos: end}.WordEnd(b, IsNotSpace)
		_, _, sz := b.DecodeGraphemeAt(ec.Pos)
		end = ec.Pos + sz
	}
	return start, end
}

func toAroundWORD(b *Buffer, pos int, count int) (int, int) {
	start, end := toInnerWORD(b, pos, count)
	for end < b.Len() {
		r, _, sz := b.DecodeGraphemeAt(end)
		if !unicode.IsSpace(r) || r == '\n' || sz == 0 {
			break
		}
		end += sz
	}
	return start, end
}

func makeInnerDelim(open, close rune) func(b *Buffer, pos int, count int) (int, int) {
	return func(b *Buffer, pos int, _ int) (int, int) {
		// Search backward for opening delimiter.
		start := pos
		depth := 0
		found := false
		for start > 0 {
			r, sz := b.DecodeRuneBefore(start)
			start -= sz
			if r == close && close != open {
				depth++
			}
			if r == open {
				if depth == 0 {
					start += sz // don't include delimiter
					found = true
					break
				}
				depth--
			}
		}
		if !found {
			return pos, pos
		}

		// Search forward for closing delimiter.
		end := pos
		depth = 0
		found = false
		for end < b.Len() {
			r, _, sz := b.DecodeGraphemeAt(end)
			if r == open && close != open {
				depth++
			}
			if r == close {
				if depth == 0 {
					found = true
					break // don't include delimiter
				}
				depth--
			}
			end += sz
		}
		if !found {
			return pos, pos
		}
		return start, end
	}
}

func makeAroundDelim(open, close rune) func(b *Buffer, pos int, count int) (int, int) {
	inner := makeInnerDelim(open, close)
	return func(b *Buffer, pos int, count int) (int, int) {
		start, end := inner(b, pos, count)
		if start == end {
			return start, end
		}
		// Include the delimiters.
		if start > 0 {
			_, sz := b.DecodeRuneBefore(start)
			start -= sz
		}
		if end < b.Len() {
			_, _, sz := b.DecodeGraphemeAt(end)
			end += sz
		}
		return start, end
	}
}

func makeInnerQuote(quote rune) func(b *Buffer, pos int, count int) (int, int) {
	return func(b *Buffer, pos int, _ int) (int, int) {
		// Find opening quote backward.
		start := pos
		found := false
		for start > 0 {
			r, sz := b.DecodeRuneBefore(start)
			if r == '\n' {
				break
			}
			start -= sz
			if r == quote {
				// start is now AT the opening quote; advance past it
				// for the inner variant.
				_, _, qsz := b.DecodeGraphemeAt(start)
				start += qsz
				found = true
				break
			}
		}
		if !found {
			return pos, pos
		}
		// Find closing quote forward.
		end := pos
		found = false
		for end < b.Len() {
			r, _, sz := b.DecodeGraphemeAt(end)
			if r == '\n' {
				break
			}
			if r == quote && end != start-1 {
				// end is AT the closing quote; exclusive range, so don't advance.
				found = true
				break
			}
			end += sz
		}
		if !found {
			return pos, pos
		}
		return start, end
	}
}

func makeAroundQuote(quote rune) func(b *Buffer, pos int, count int) (int, int) {
	inner := makeInnerQuote(quote)
	return func(b *Buffer, pos int, count int) (int, int) {
		start, end := inner(b, pos, count)
		if start == end {
			return start, end
		}
		if start > 0 {
			_, sz := b.DecodeRuneBefore(start)
			start -= sz
		}
		if end < b.Len() {
			_, _, sz := b.DecodeGraphemeAt(end)
			end += sz
		}
		return start, end
	}
}

func toInnerParagraph(b *Buffer, pos int, _ int) (int, int) {
	line, _ := b.LineColAt(pos)
	// Find start of paragraph (first blank line above).
	sl := line
	for sl > 0 && b.LineLen(sl-1) > 0 {
		sl--
	}
	// Find end of paragraph (first blank line below).
	el := line
	for el < b.NumLines() && b.LineLen(el) > 0 {
		el++
	}
	start := b.OffsetAt(sl, 0)
	end := b.OffsetAt(el, 0)
	if end > b.Len() {
		end = b.Len()
	}
	return start, end
}

func toAroundParagraph(b *Buffer, pos int, count int) (int, int) {
	start, end := toInnerParagraph(b, pos, count)
	// Include trailing blank lines.
	for {
		el, _ := b.LineColAt(end)
		if el >= b.NumLines() || b.LineLen(el) > 0 {
			break
		}
		end = b.OffsetAt(el+1, 0)
		if end > b.Len() {
			end = b.Len()
			break
		}
	}
	return start, end
}

// --- Registration ---

// RegisterTextObjects registers all text object bindings.
func RegisterTextObjects(ks *KeyState) {
	// Word objects.
	registerTextObject(ks, "i", "w", TextObjectDef{Fn: toInnerWord})
	registerTextObject(ks, "a", "w", TextObjectDef{Fn: toAroundWord})
	registerTextObject(ks, "i", "W", TextObjectDef{Fn: toInnerWORD})
	registerTextObject(ks, "a", "W", TextObjectDef{Fn: toAroundWORD})

	// Delimiter objects.
	for _, pair := range [][2]rune{{'(', ')'}, {'{', '}'}, {'[', ']'}, {'<', '>'}} {
		open, close := pair[0], pair[1]
		registerTextObject(ks, "i", string(open), TextObjectDef{Fn: makeInnerDelim(open, close)})
		registerTextObject(ks, "i", string(close), TextObjectDef{Fn: makeInnerDelim(open, close)})
		registerTextObject(ks, "a", string(open), TextObjectDef{Fn: makeAroundDelim(open, close)})
		registerTextObject(ks, "a", string(close), TextObjectDef{Fn: makeAroundDelim(open, close)})
	}

	// Quote objects.
	for _, q := range []rune{'"', '\'', '`'} {
		registerTextObject(ks, "i", string(q), TextObjectDef{Fn: makeInnerQuote(q)})
		registerTextObject(ks, "a", string(q), TextObjectDef{Fn: makeAroundQuote(q)})
	}

	// Paragraph objects.
	registerTextObject(ks, "i", "p", TextObjectDef{Fn: toInnerParagraph})
	registerTextObject(ks, "a", "p", TextObjectDef{Fn: toAroundParagraph})

	// Entire buffer objects.
	entireBuf := TextObjectDef{Fn: func(b *Buffer, pos int, count int) (int, int) {
		return 0, b.Len()
	}}
	registerTextObject(ks, "i", "e", entireBuf)
	registerTextObject(ks, "a", "e", entireBuf)
}

// SetupBindings registers all vim bindings on the given key state.
func SetupBindings(ks *KeyState) {
	RegisterMotions(ks)
	RegisterOperators(ks)
	RegisterTextObjects(ks)
}
