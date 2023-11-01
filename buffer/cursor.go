package buffer

import (
	"encoding/gob"
	"fmt"
	"unicode"

	"github.com/zyedidia/mu/config"
)

type Cursor struct {
	Pos    int
	HasSel bool
	Orig   [2]int
	Sel    [2]int
	Vx     int
	Num    int
}

func (b *Buffer) NumCursors() int {
	return len(b.cursors)
}

func (b *Buffer) Cursors() []Cursor {
	return b.cursors
}

func (b *Buffer) SwitchCursor(idx int) error {
	if idx >= 0 && idx < len(b.cursors) {
		b.cur = idx
		return nil
	}
	return fmt.Errorf("invalid cursor: %d", idx)
}

func (b *Buffer) SpawnCursor(at int) {
	b.cursors = append(b.cursors, b.GetCursorAt(at))
}

func (b *Buffer) RemoveCursors() {
	b.cursors = b.cursors[:1]
	b.cur = 0
}

func (b *Buffer) PutCursor(c Cursor) {
	if c.Num >= 0 && c.Num < len(b.cursors) {
		b.cursors[c.Num] = c
	}
}

func (b *Buffer) GetCursorAt(pos int) Cursor {
	for _, c := range b.cursors {
		if c.Pos == pos {
			return c
		}
	}
	return SpawnCursorAt(pos)
}

func (b *Buffer) Cursor() *Cursor {
	return &b.cursors[b.cur]
}

func SpawnCursorAt(pos int) Cursor {
	return Cursor{
		Pos: pos,
	}
}

func SpawnCursorSelect(start, end int) Cursor {
	return Cursor{
		HasSel: true,
		Orig:   [2]int{start, end},
		Sel:    [2]int{start, end},
	}
}
func (c Cursor) At(b *Buffer) rune {
	r, _ := b.DecodeRuneAt(c.Pos)
	return r
}

func (c Cursor) HasSelection() bool {
	return c.HasSel
}

func (c Cursor) Selection(b *Buffer) []byte {
	if !c.HasSel {
		return nil
	}
	return b.Slice(c.Sel[0], c.Sel[1])
}

func (c Cursor) MoveTo(pos int) Cursor {
	c.HasSel = false
	c.Pos = pos
	return c
}

func (c Cursor) SelectTo(pos int) Cursor {
	if !c.HasSel {
		c.Orig[0] = c.Pos
		c.Orig[1] = c.Pos
		c.Sel[0] = c.Pos
		c.Sel[1] = c.Pos
	}

	c.HasSel = true
	if pos < c.Orig[0] {
		c.Sel[0] = pos
	} else if pos > c.Orig[1] {
		c.Sel[1] = pos
	} else if pos > c.Orig[0] {
		c.Sel[1] = c.Orig[1]
	} else if pos < c.Orig[1] {
		c.Sel[0] = c.Orig[0]
	}
	c.Pos = pos
	return c
}

func (c Cursor) Clamp(b *Buffer) Cursor {
	sz := int(b.Size())

	if c.HasSel {
		c.Pos = clamp(c.Pos, 0, sz-1)
	} else {
		c.Orig[0] = clamp(c.Orig[0], 0, sz-1)
		c.Orig[1] = clamp(c.Orig[1], 0, sz-1)
		c.Sel[0] = clamp(c.Sel[0], 0, sz-1)
		c.Sel[1] = clamp(c.Sel[1], 0, sz-1)
	}
	return c
}

func (c *Cursor) Deselect(idx int) {
	if c.HasSel {
		*c = c.MoveTo(c.Sel[idx])
	}
}

func (c Cursor) Right(b *Buffer) Cursor {
	_, _, sz := b.DecodeGraphemeAt(c.Pos)
	c.Pos += sz
	return c
}

func (c Cursor) Left(b *Buffer) Cursor {
	_, _, sz := b.DecodeGraphemeBefore(c.Pos)
	c.Pos -= sz
	return c
}

func (c Cursor) RightVim(b *Buffer) Cursor {
	l, col := b.LineColAt(c.Pos)
	if col == b.LineLen(l)-1 {
		return c
	}
	return c.Right(b)
}

func (c Cursor) LeftVim(b *Buffer) Cursor {
	_, col := b.LineColAt(c.Pos)
	if col == 0 {
		return c
	}
	return c.Left(b)
}

func clamp(a, min, max int) int {
	if a > max {
		a = max
	}
	if a < min {
		a = min
	}
	return a
}

func (c Cursor) WordRight(b *Buffer, wordc func(r rune) bool) Cursor {
	p := c.Pos

	consume := func(s int, fn func(r rune) bool) int {
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

	var s int
	// consume space
	s = consume(p, unicode.IsSpace)
	if s != 0 {
		c.Pos = p + s
		return c
	}
	// consume word characters
	s = consume(p, wordc)
	if s != 0 {
		p += s
	} else {
		// if on a symbol (non-word), consume the symbol
		_, _, sz := b.DecodeGraphemeAt(p)
		p += sz
	}
	// consume space characters
	s = consume(p, unicode.IsSpace)
	c.Pos = p + s
	return c
}

func (c Cursor) WordLeft(b *Buffer, wordc func(r rune) bool) Cursor {
	p := c.Pos
	consume := func(s int, fn func(r rune) bool) int {
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
	var s int
	s = consume(p, unicode.IsSpace)
	p -= s
	// consume word chars
	s = consume(p, wordc)
	if s != 0 {
		p -= s
		c.Pos = p
		return c
	}
	_, _, sz := b.DecodeGraphemeBefore(p)
	c.Pos = p - sz
	return c
}

func (c Cursor) WordEnd(b *Buffer, wordc func(r rune) bool) Cursor {
	p := c.Pos
	_, _, sz := b.DecodeGraphemeAt(p)
	p += sz

	consume := func(s int, fn func(r rune) bool) int {
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

	var s int
	s = consume(p, unicode.IsSpace)
	p += s
	s = consume(p, wordc)
	if s != 0 {
		p += s
	} else {
		_, _, sz := b.DecodeGraphemeAt(p)
		p += sz
	}
	_, _, sz = b.DecodeGraphemeBefore(p)
	p -= sz

	c.Pos = p
	return c
}

func (b *Buffer) SerializeCursors(fs config.WriteFS, fname string) error {
	f, err := fs.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := gob.NewEncoder(f)
	return enc.Encode(b.cursors)
}

func loadCursors(fs config.WriteFS, fname string) (cursors []Cursor) {
	f, err := fs.Open(fname)
	if err != nil {
		return []Cursor{{}}
	}
	defer f.Close()
	dec := gob.NewDecoder(f)
	err = dec.Decode(&cursors)
	if err != nil {
		return []Cursor{{}}
	}
	return cursors
}
