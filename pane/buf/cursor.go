package buf

import (
	"github.com/zyedidia/ned/buffer"
)

type Cursor struct {
	Pos int

	HasSel bool
	Orig   [2]int
	Sel    [2]int
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

func (c Cursor) At(b *buffer.Buffer) rune {
	r, _ := b.DecodeRuneAt(c.Pos)
	return r
}

func (c Cursor) HasSelection() bool {
	return c.HasSel
}

func (c Cursor) Selection(b *buffer.Buffer) []byte {
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
	return c
}

func (c Cursor) Clamp(b *buffer.Buffer) Cursor {
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

func (c Cursor) Deselect(idx int) Cursor {
	if c.HasSel {
		c.MoveTo(c.Sel[idx])
	}
	return c
}

func (c Cursor) Right(b *buffer.Buffer) Cursor {
	c = c.Deselect(1)
	_, _, sz := b.DecodeGraphemeAt(c.Pos)
	c.Pos += sz
	return c
}

func (c Cursor) Left(b *buffer.Buffer) Cursor {
	c = c.Deselect(0)
	_, _, sz := b.DecodeGraphemeBefore(c.Pos)
	c.Pos -= sz
	return c
}

// TODO: need virtual cursors to handle visual x
func (c Cursor) Up(b *buffer.Buffer) Cursor {
	c = c.Deselect(0)
	line, col := b.LineColAt(c.Pos)
	c.Pos = b.OffsetAt(line-1, col)
	return c
}

func (c Cursor) Down(b *buffer.Buffer) Cursor {
	c.Deselect(1)
	line, col := b.LineColAt(c.Pos)
	c.Pos = b.OffsetAt(line+1, col)
	return c
}

//
// func (c Cursor) WordLeft(b *buffer.Buffer) Cursor {
//
// }
//
// func (c Cursor) WordRight(b *buffer.Buffer) Cursor {
//
// }

func clamp(a, min, max int) int {
	if a > max {
		a = max
	}
	if a < min {
		a = min
	}
	return a
}
