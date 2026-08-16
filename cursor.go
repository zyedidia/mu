package main

import (
	"unicode"
)

// Cursor represents a position in a buffer with optional selection.
// Methods use value semantics: they return a new Cursor rather than mutating.
type Cursor struct {
	Pos    int    // byte offset in buffer
	HasSel bool   // whether a selection is active
	Orig   [2]int // original anchor for extending selections
	Sel    [2]int // current selection range [start, end)
	Vx     int    // desired visual column for vertical movement
	Num    int    // cursor index in the buffer's cursor list

	// BlockSel marks the selection as a visual-block rectangle between the
	// corners Orig[0] and Pos. Sel still holds the byte span between the
	// corners, but rendering and operators use the rectangle instead.
	BlockSel bool
	// BlockEOL extends the block's right edge to the end of each line ($).
	BlockEOL bool
}

// IsWordChar returns true for characters that are part of a "word" in vim's
// sense: letters, digits, and underscore.
func IsWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// IsNotSpace returns true for any non-whitespace character (for WORD motions).
func IsNotSpace(r rune) bool {
	return !unicode.IsSpace(r)
}

// HasSelection returns whether this cursor has an active selection.
func (c Cursor) HasSelection() bool {
	return c.HasSel
}

// Selection returns the selected bytes, or nil if no selection.
func (c Cursor) Selection(b *Buffer) []byte {
	if !c.HasSel {
		return nil
	}
	return b.Slice(c.Sel[0], c.Sel[1])
}

// MoveTo moves the cursor to pos and clears the selection.
func (c Cursor) MoveTo(pos int) Cursor {
	c.HasSel = false
	c.Pos = pos
	return c
}

// SelectTo extends the selection from the original anchor to pos.
func (c Cursor) SelectTo(pos int) Cursor {
	if !c.HasSel {
		c.Orig[0] = c.Pos
		c.Orig[1] = c.Pos
	}
	c.Sel[0] = min(c.Orig[0], pos)
	c.Sel[1] = max(c.Orig[1], pos)

	c.HasSel = c.Sel[0] != c.Sel[1]
	c.Pos = pos
	return c
}

// Deselect clears the selection, moving the cursor to sel[idx].
func (c *Cursor) Deselect(idx int) {
	if c.HasSel {
		*c = c.MoveTo(c.Sel[idx])
	}
}

// Clamp ensures the cursor position is within the buffer bounds.
func (c Cursor) Clamp(b *Buffer) Cursor {
	sz := b.Len()
	c.Pos = clamp(c.Pos, 0, sz)
	if c.HasSel {
		c.Sel[0] = clamp(c.Sel[0], 0, sz)
		c.Sel[1] = clamp(c.Sel[1], 0, sz)
		c.Orig[0] = clamp(c.Orig[0], 0, sz)
		c.Orig[1] = clamp(c.Orig[1], 0, sz)
	}
	return c
}

func clamp(a, lo, hi int) int {
	if a < lo {
		return lo
	}
	if a > hi {
		return hi
	}
	return a
}

// VimClamp ensures the cursor is not on a newline in normal mode.
// If the cursor is on a '\n' and the previous character is not '\n'
// (i.e., the line is not empty), it moves back one character.
func (c Cursor) VimClamp(b *Buffer) Cursor {
	r, sz := b.DecodeRuneAt(c.Pos)
	if r != '\n' || sz == 0 {
		return c
	}
	pr, psz := b.DecodeRuneBefore(c.Pos)
	if pr == '\n' || psz == 0 {
		return c // empty line, stay
	}
	c.Pos -= psz
	return c
}

// Right moves the cursor one grapheme to the right.
func (c Cursor) Right(b *Buffer) Cursor {
	_, _, sz := b.DecodeGraphemeAt(c.Pos)
	c.Pos += sz
	return c
}

// Left moves the cursor one grapheme to the left.
func (c Cursor) Left(b *Buffer) Cursor {
	_, _, sz := b.DecodeGraphemeBefore(c.Pos)
	c.Pos -= sz
	return c
}

// LineStart moves the cursor to the beginning of the current line.
func (c Cursor) LineStart(b *Buffer) Cursor {
	from := c.Pos
	for {
		r, sz := b.DecodeRuneBefore(from)
		if r == '\n' || sz == 0 {
			c.Pos = from
			return c
		}
		from -= sz
	}
}

// LineEnd moves the cursor to the end of the current line (before '\n').
func (c Cursor) LineEnd(b *Buffer) Cursor {
	from := c.Pos
	for {
		r, sz := b.DecodeRuneAt(from)
		if r == '\n' || sz == 0 {
			c.Pos = from
			return c
		}
		from += sz
	}
}

// WordRight moves the cursor to the start of the next word. The wordc function
// determines what counts as a word character.
func (c Cursor) WordRight(b *Buffer, wordc func(rune) bool) Cursor {
	p := c.Pos

	consume := func(s int, fn func(rune) bool) int {
		consumed := 0
		for {
			r, _, sz := b.DecodeGraphemeAt(s + consumed)
			if !fn(r) || sz == 0 {
				break
			}
			consumed += sz
		}
		return consumed
	}

	// consume space
	s := consume(p, unicode.IsSpace)
	if s != 0 {
		c.Pos = p + s
		return c
	}
	// consume word characters
	s = consume(p, wordc)
	if s != 0 {
		p += s
	} else {
		// on a symbol: consume it
		_, _, sz := b.DecodeGraphemeAt(p)
		p += sz
	}
	// consume trailing space
	s = consume(p, unicode.IsSpace)
	c.Pos = p + s
	return c
}

// WordLeft moves the cursor to the start of the previous word.
func (c Cursor) WordLeft(b *Buffer, wordc func(rune) bool) Cursor {
	p := c.Pos

	consume := func(s int, fn func(rune) bool) int {
		consumed := 0
		for {
			r, _, sz := b.DecodeGraphemeBefore(s - consumed)
			if !fn(r) || sz == 0 {
				break
			}
			consumed += sz
		}
		return consumed
	}

	// consume space
	s := consume(p, unicode.IsSpace)
	p -= s
	// consume word chars
	s = consume(p, wordc)
	if s != 0 {
		c.Pos = p - s
		return c
	}
	// on a symbol: skip it
	_, _, sz := b.DecodeGraphemeBefore(p)
	c.Pos = p - sz
	return c
}

// WordStart moves the cursor to the start of the current word.
func (c Cursor) WordStart(b *Buffer, wordc func(rune) bool) Cursor {
	from := c.Pos
	consumed := 0
	for {
		r, _, sz := b.DecodeGraphemeBefore(from - consumed)
		if !wordc(r) || sz == 0 {
			break
		}
		consumed += sz
	}
	c.Pos = from - consumed
	return c
}

// WordEnd moves the cursor to the end of the current/next word.
func (c Cursor) WordEnd(b *Buffer, wordc func(rune) bool) Cursor {
	p := c.Pos
	_, _, sz := b.DecodeGraphemeAt(p)
	p += sz

	consume := func(s int, fn func(rune) bool) int {
		consumed := 0
		for {
			r, _, sz := b.DecodeGraphemeAt(s + consumed)
			if !fn(r) || sz == 0 {
				break
			}
			consumed += sz
		}
		return consumed
	}

	// skip whitespace
	s := consume(p, unicode.IsSpace)
	p += s
	// consume word chars
	s = consume(p, wordc)
	if s != 0 {
		p += s
	} else {
		// consume a symbol
		_, _, sz := b.DecodeGraphemeAt(p)
		p += sz
	}
	// back up one grapheme to land on the last char of the word
	_, _, sz = b.DecodeGraphemeBefore(p)
	c.Pos = p - sz
	return c
}

// Buffer multi-cursor management.

// Cursor returns a pointer to the active cursor.
func (b *Buffer) Cursor() *Cursor {
	return &b.cursors[b.cur]
}

// Cursors returns all cursors.
func (b *Buffer) Cursors() []Cursor {
	return b.cursors
}

// NumCursors returns the number of cursors.
func (b *Buffer) NumCursors() int {
	return len(b.cursors)
}

// SwitchCursor changes the active cursor.
func (b *Buffer) SwitchCursor(idx int) {
	if idx >= 0 && idx < len(b.cursors) {
		b.cur = idx
	}
}

// SpawnCursor adds a new cursor at the given position and makes it active.
func (b *Buffer) SpawnCursor(at int) {
	c := Cursor{Pos: at, Num: len(b.cursors)}
	b.cursors = append(b.cursors, c)
	b.cur = len(b.cursors) - 1
}

// RemoveCursors removes all cursors except the primary one.
func (b *Buffer) RemoveCursors() {
	b.cursors = b.cursors[:1]
	b.cur = 0
}

// PutCursor restores a cursor at its numbered position.
func (b *Buffer) PutCursor(c Cursor) {
	if c.Num >= 0 && c.Num < len(b.cursors) {
		b.cursors[c.Num] = c
	}
}
