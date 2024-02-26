package buf

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/zyedidia/mu/buffer"
)

// TODO: need virtual cursors to handle visual x
func (bp *BufPane) CursorUp(c buffer.Cursor) buffer.Cursor {
	line, _ := bp.LineColAt(c.Pos)
	c.Pos = bp.VisualLoc(line-1, c.Vx)
	bp.vertical = true
	return c
}

func (bp *BufPane) CursorDown(c buffer.Cursor) buffer.Cursor {
	line, _ := bp.LineColAt(c.Pos)
	c.Pos = bp.VisualLoc(line+1, c.Vx)
	bp.vertical = true
	return c
}

func (bp *BufPane) RecalcVX(c *buffer.Cursor) {
	line, col := bp.Buffer.LineColAt(c.Pos)
	vl := bp.bLoc2vLoc(bLoc{line: line, col: col})
	c.Vx = vl.col
}

func (bp *BufPane) Up(from int) int {
	c := bp.CursorUp(bp.GetCursorAt(from))
	return c.Pos
}

func (bp *BufPane) Down(from int) int {
	c := bp.CursorDown(bp.GetCursorAt(from))
	return c.Pos
}

func (bp *BufPane) Left(from int) int {
	c := bp.GetCursorAt(from).Left(bp.Buffer)
	return c.Pos
}

func (bp *BufPane) Right(from int) int {
	c := bp.GetCursorAt(from).Right(bp.Buffer)
	return c.Pos
}

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
func isNotSpace(r rune) bool {
	return !unicode.IsSpace(r)
}

func (bp *BufPane) WordLeft(from int) int {
	c := bp.GetCursorAt(from).WordLeft(bp.Buffer, isWord)
	return c.Pos
}

func (bp *BufPane) WordRight(from int) int {
	c := bp.GetCursorAt(from).WordRight(bp.Buffer, isWord)
	return c.Pos
}

func (bp *BufPane) WordLeftWS(from int) int {
	c := bp.GetCursorAt(from).WordLeft(bp.Buffer, isNotSpace)
	return c.Pos
}

func (bp *BufPane) WordRightWS(from int) int {
	c := bp.GetCursorAt(from).WordRight(bp.Buffer, isNotSpace)
	return c.Pos
}

func (bp *BufPane) WordEnd(from int) int {
	c := bp.GetCursorAt(from).WordEnd(bp.Buffer, isWord)
	return c.Pos
}

func (bp *BufPane) WordEndWS(from int) int {
	c := bp.GetCursorAt(from).WordEnd(bp.Buffer, isNotSpace)
	return c.Pos
}

func (bp *BufPane) FindChar(c rune, from int) int {
	_, sz := bp.DecodeRuneAt(from)
	p := from + sz
	for {
		r, sz := bp.DecodeRuneAt(p)
		if r == '\n' || sz == 0 {
			return from
		} else if r == c {
			return p
		}
		p += sz
	}
}

func (bp *BufPane) FindCharBack(c rune, from int) int {
	p := from
	for {
		r, sz := bp.DecodeRuneBefore(p)
		if r == '\n' || sz == 0 {
			return from
		} else if r == c {
			return p - sz
		}
		p -= sz
	}
}

func (bp *BufPane) TillChar(c rune, from int) int {
	_, sz := bp.DecodeRuneAt(from)
	last := sz
	p := from + sz
	for {
		r, sz := bp.DecodeRuneAt(p)
		if r == '\n' || sz == 0 {
			return from
		} else if r == c {
			return p - last
		}
		last = sz
		p += sz
	}
}

func (bp *BufPane) TillCharBack(c rune, from int) int {
	p := from
	for {
		r, sz := bp.DecodeRuneBefore(p)
		if r == '\n' || sz == 0 {
			return from
		} else if r == c {
			return p
		}
		p -= sz
	}
}

func (bp *BufPane) LineStart(from int) int {
	for {
		r, sz := bp.DecodeRuneBefore(from)
		if r == '\n' || sz == 0 {
			return from
		}
		from -= sz
	}
}

func (bp *BufPane) LineEnd(from int) int {
	for {
		r, sz := bp.DecodeRuneAt(from)
		if r == '\n' || sz == 0 {
			return from
		}
		from += sz
	}
}

func (bp *BufPane) NextLineStart(from int) int {
	for {
		r, sz := bp.DecodeRuneAt(from)
		if r == '\n' || sz == 0 {
			return from + 1
		}
		from += sz
	}
}

func (bp *BufPane) VimClamp(from int) int {
	r, sz := bp.DecodeRuneAt(from)
	if r != '\n' || sz == 0 {
		return from
	} else {
		b, sz := bp.DecodeRuneBefore(from)
		if b == '\n' || sz == 0 {
			return from
		}
	}
	return from - 1
}

// --- Cursors ---

func (bp *BufPane) MoveTo(pos int) {
	c := bp.Cursor()
	*c = c.MoveTo(pos)
	if !bp.vertical {
		bp.RecalcVX(c)
	}
	bp.vertical = false
}

func (bp *BufPane) SelectWithModeTo(pos int) {
	c := bp.Cursor()
	*c = c.SelectWithModeTo(pos, bp.Buffer, isWord)
	if !bp.vertical {
		bp.RecalcVX(c)
	}
	bp.vertical = false
}

func (bp *BufPane) SelectTo(pos int) {
	c := bp.Cursor()
	*c = c.SelectTo(pos)
	if !bp.vertical {
		bp.RecalcVX(c)
	}
	bp.vertical = false
}

func (bp *BufPane) CursorPos() int {
	return bp.Cursor().Pos
}
func (bp *BufPane) CursorCol() int {
	_, c := bp.LineColAt(bp.Cursor().Pos)
	return c + 1
}
func (bp *BufPane) CursorLine() int {
	l, _ := bp.LineColAt(bp.Cursor().Pos)
	return l + 1
}

func (bp *BufPane) CursorRange() []int {
	sel := bp.Cursor().Sel
	return []int{sel[0], sel[1]}
}

func (bp *BufPane) CursorHasSelection() bool {
	return bp.Cursor().HasSelection()
}

func (bp *BufPane) CursorSelection() string {
	return string(bp.Cursor().Selection(bp.Buffer))
}

func (bp *BufPane) VisualPos(loc string) int {
	// TODO: we assume the location comes in as a list {x, y} but we should
	// actually check that, or even better use the TclObj interface directly
	// (looks like tcl.AsList is not working right).
	parts := strings.Split(loc[1:len(loc)-1], " ")
	x, _ := strconv.Atoi(parts[0])
	y, _ := strconv.Atoi(parts[1])

	line, col := bp.MouseLoc(x, y)
	return bp.OffsetAt(line, col)
}

func (bp *BufPane) SpawnCursor(at int) {
	bp.Buffer.SpawnCursor(at)
	bp.RecalcVX(bp.Cursor())
}
