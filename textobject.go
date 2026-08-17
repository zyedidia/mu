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
			} else if ks.ModeID() == ModeVisualBlock {
				// Keep the cursor on the object's last character so the
				// block corner stays on the object's final line.
				cc := &b.cursors[i]
				if _, _, sz := b.DecodeGraphemeBefore(cc.Pos); sz > 0 {
					cc.Pos -= sz
				}
				*cc = cc.VimClamp(b)
			}
		}
	}
	ks.ClearCounts()
}

// registerTextObject binds a text object under a prefix key (e.g. "i" or "a")
// in operator-pending and visual modes.
func registerTextObject(ks *KeyState, prefix string, key string, to TextObjectDef) {
	ks.modes[ModeOperatorPending].Bindings.Bind(func(ks *KeyState) {
		execTextObjOp(ks, to)
	}, prefix, key)
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine, ModeVisualBlock} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			applyTextObjVisual(ks, to)
		}, prefix, key)
	}
}

// --- Text object implementations ---

// wordClass classifies a rune for iw/aw: -1 newline (hard boundary),
// 0 whitespace, 1 word characters, 2 other symbols. An inner-word object is
// a maximal run of characters of one class (vim: iw on whitespace selects
// the whitespace run; on a symbol, the symbol run).
func wordClass(r rune) int {
	switch {
	case r == '\n':
		return -1
	case unicode.IsSpace(r):
		return 0
	case IsWordChar(r):
		return 1
	default:
		return 2
	}
}

// wORDClass classifies a rune for iW/aW: whitespace vs everything else.
func wORDClass(r rune) int {
	switch {
	case r == '\n':
		return -1
	case unicode.IsSpace(r):
		return 0
	default:
		return 1
	}
}

// innerRun returns the run of same-class characters around pos. With a
// count > 1, the range extends over the following count-1 runs.
func innerRun(b *Buffer, pos, count int, classOf func(rune) int) (int, int) {
	r, _, sz := b.DecodeGraphemeAt(pos)
	if sz == 0 || r == '\n' {
		return pos, pos
	}
	cls := classOf(r)
	start := pos
	for start > 0 {
		pr, _, psz := b.DecodeGraphemeBefore(start)
		if psz == 0 || classOf(pr) != cls {
			break
		}
		start -= psz
	}
	end := pos + sz
	for {
		nr, _, nsz := b.DecodeGraphemeAt(end)
		if nsz == 0 || classOf(nr) != cls {
			break
		}
		end += nsz
	}
	for i := 1; i < count; i++ {
		nr, _, nsz := b.DecodeGraphemeAt(end)
		if nsz == 0 || nr == '\n' {
			break
		}
		ncls := classOf(nr)
		end += nsz
		for {
			r2, _, s2 := b.DecodeGraphemeAt(end)
			if s2 == 0 || classOf(r2) != ncls {
				break
			}
			end += s2
		}
	}
	return start, end
}

// aroundRun is innerRun plus surrounding whitespace, following vim's aw
// rules: on whitespace, include the following word; otherwise include
// trailing whitespace, or leading whitespace if there is none trailing.
func aroundRun(b *Buffer, pos, count int, classOf func(rune) int) (int, int) {
	r, _, sz := b.DecodeGraphemeAt(pos)
	if sz == 0 || r == '\n' {
		return pos, pos
	}
	start, end := innerRun(b, pos, count, classOf)
	if classOf(r) == 0 {
		// On whitespace: include the following word/symbol run.
		nr, _, _ := b.DecodeGraphemeAt(end)
		if ncls := classOf(nr); ncls > 0 {
			for {
				r2, _, s2 := b.DecodeGraphemeAt(end)
				if s2 == 0 || classOf(r2) != ncls {
					break
				}
				end += s2
			}
		}
		return start, end
	}
	trailStart := end
	for {
		nr, _, nsz := b.DecodeGraphemeAt(end)
		if nsz == 0 || classOf(nr) != 0 {
			break
		}
		end += nsz
	}
	if end == trailStart {
		// No trailing whitespace: include leading whitespace instead.
		for start > 0 {
			pr, _, psz := b.DecodeGraphemeBefore(start)
			if psz == 0 || classOf(pr) != 0 {
				break
			}
			start -= psz
		}
	}
	return start, end
}

func toInnerWord(b *Buffer, pos int, count int) (int, int) {
	return innerRun(b, pos, count, wordClass)
}

func toAroundWord(b *Buffer, pos int, count int) (int, int) {
	return aroundRun(b, pos, count, wordClass)
}

func toInnerWORD(b *Buffer, pos int, count int) (int, int) {
	return innerRun(b, pos, count, wORDClass)
}

func toAroundWORD(b *Buffer, pos int, count int) (int, int) {
	return aroundRun(b, pos, count, wORDClass)
}

func makeInnerDelim(open, close rune) func(b *Buffer, pos int, count int) (int, int) {
	return func(b *Buffer, pos int, _ int) (int, int) {
		searchPos := pos
		// If cursor is on the opening delimiter, step inside.
		if r, _, sz := b.DecodeGraphemeAt(pos); r == open && sz > 0 {
			searchPos = pos + sz
		}
		// If cursor is on the closing delimiter, search backward from before it.
		if r, _, _ := b.DecodeGraphemeAt(pos); r == close {
			searchPos = pos
		}

		// Search backward for opening delimiter.
		start := searchPos
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
		end := searchPos
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
		// Vim pairs quotes from the start of the line: the 1st with the 2nd,
		// the 3rd with the 4th, and so on. Collect quote positions on the
		// cursor's line, then pick the pair containing (or following) pos.
		line, _ := b.LineColAt(pos)
		p := b.OffsetAt(line, 0)
		var qpos []int
		for {
			r, _, sz := b.DecodeGraphemeAt(p)
			if sz == 0 || r == '\n' {
				break
			}
			if r == quote {
				qpos = append(qpos, p)
			}
			p += sz
		}
		for i := 0; i+1 < len(qpos); i += 2 {
			open, close := qpos[i], qpos[i+1]
			if pos <= close {
				_, _, osz := b.DecodeGraphemeAt(open)
				return open + osz, close
			}
		}
		return pos, pos
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
	RegisterVisualBlock(ks)
	RegisterComments(ks)
	RegisterFormat(ks)
	RegisterMacros(ks)
	RegisterMultiCursor(ks)
	RegisterTextObjects(ks)
}
