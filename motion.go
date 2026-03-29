package main

import (
	"unicode"
)

// MotionFlags control how a motion's range is interpreted.
type MotionFlags int

const (
	Charwise  MotionFlags = 0
	Linewise  MotionFlags = 1 << iota
	Inclusive             // include the endpoint character in the range
)

// MotionDef defines a cursor motion. Fn computes a new byte offset from the
// current cursor. Count is the raw count (0 = unset). Name is used for
// special-casing (e.g. vim's cw→ce quirk).
type MotionDef struct {
	Fn    func(b *Buffer, c Cursor, count int) int
	Flags MotionFlags
	Name  string
}

// --- Motion application helpers ---

// applyMotion moves or extends selection for all cursors.
func applyMotion(ks *KeyState, m MotionDef, selecting bool) {
	b := ks.Buf()
	count := ks.RawCount()
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		newPos := m.Fn(b, c, count)
		newPos = clamp(newPos, 0, b.Len())
		if selecting {
			b.cursors[i] = c.SelectTo(newPos)
			if ks.ModeID() == ModeVisualLine {
				adjustVisualLine(b, &b.cursors[i])
			}
		} else {
			b.cursors[i] = c.MoveTo(newPos)
		}
	}
	ks.count = 0
}

// adjustVisualLine extends a cursor's selection to full lines.
func adjustVisualLine(b *Buffer, c *Cursor) {
	if !c.HasSel {
		return
	}
	sl, _ := b.LineColAt(c.Sel[0])
	el, _ := b.LineColAt(c.Sel[1])
	c.Sel[0] = b.OffsetAt(sl, 0)
	end := b.OffsetAt(el+1, 0)
	if end > b.Len() {
		end = b.Len()
	}
	c.Sel[1] = end
}

// execMotionOp executes the pending operator using a motion range.
func execMotionOp(ks *KeyState, m MotionDef) {
	b := ks.Buf()
	count := ks.RawCount()
	op := ks.Pending()
	if op == nil {
		ks.ResetAction()
		return
	}

	// Vim special case: cw acts like ce, cW acts like cE.
	if op.Key == "c" {
		switch m.Name {
		case "w":
			m = MotionDef{Fn: motionWordEnd, Flags: Inclusive, Name: "e"}
		case "W":
			m = MotionDef{Fn: motionWORDEnd, Flags: Inclusive, Name: "E"}
		}
	}

	b.UndoBarrier()
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		start := c.Pos
		end := m.Fn(b, c, count)
		if start > end {
			start, end = end, start
		}
		if m.Flags&Inclusive != 0 && end < b.Len() {
			_, _, sz := b.DecodeGraphemeAt(end)
			end += sz
		}
		if m.Flags&Linewise != 0 {
			start, end = extendToLines(b, start, end)
		}
		op.Fn(ks, b, start, end)
	}
	ks.ResetAction()
}

// extendToLines extends a byte range to full lines.
func extendToLines(b *Buffer, start, end int) (int, int) {
	sl, _ := b.LineColAt(start)
	el, _ := b.LineColAt(end)
	s := b.OffsetAt(sl, 0)
	e := b.OffsetAt(el+1, 0)
	if e > b.Len() {
		e = b.Len()
	}
	return s, e
}

// lineRange returns the byte range for count lines starting at line.
func lineRange(b *Buffer, line, count int) (int, int) {
	if count == 0 {
		count = 1
	}
	start := b.OffsetAt(line, 0)
	end := b.OffsetAt(line+count, 0)
	if end > b.Len() {
		end = b.Len()
	}
	return start, end
}

// --- Registration helpers ---

// registerMotion binds a motion in normal, visual, visual-line, and
// operator-pending modes.
func registerMotion(ks *KeyState, keys []string, m MotionDef) {
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		applyMotion(ks, m, false)
	}, keys...)
	ks.modes[ModeVisual].Bindings.Bind(func(ks *KeyState) {
		applyMotion(ks, m, true)
	}, keys...)
	ks.modes[ModeVisualLine].Bindings.Bind(func(ks *KeyState) {
		applyMotion(ks, m, true)
	}, keys...)
	ks.modes[ModeOperatorPending].Bindings.Bind(func(ks *KeyState) {
		execMotionOp(ks, m)
	}, keys...)
}

// registerCharMotion binds a char-argument motion (f, t, F, T) in all motion
// modes.
func registerCharMotion(ks *KeyState, key string, fn func(b *Buffer, c Cursor, count int, ch rune) int, flags MotionFlags) {
	handler := func(ks *KeyState) {
		ks.WaitForChar(func(ks *KeyState, ch string) {
			rs := []rune(ch)
			if len(rs) == 0 {
				return
			}
			m := MotionDef{
				Fn: func(b *Buffer, c Cursor, count int) int {
					return fn(b, c, count, rs[0])
				},
				Flags: flags,
			}
			if ks.Pending() != nil {
				execMotionOp(ks, m)
			} else if ks.ModeID() == ModeVisual || ks.ModeID() == ModeVisualLine {
				applyMotion(ks, m, true)
			} else {
				applyMotion(ks, m, false)
			}
		})
	}
	for _, mode := range []ModeID{ModeNormal, ModeVisual, ModeVisualLine, ModeOperatorPending} {
		ks.modes[mode].Bindings.Bind(handler, key)
	}
}

// --- Motion implementations ---

func motionLeft(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	pos := c.Pos
	for i := 0; i < count; i++ {
		_, _, sz := b.DecodeGraphemeBefore(pos)
		if sz == 0 {
			break
		}
		pos -= sz
		// Don't cross line boundary in normal mode.
		r, _ := b.DecodeRuneAt(pos)
		if r == '\n' {
			pos += sz
			break
		}
	}
	return pos
}

func motionRight(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	pos := c.Pos
	for i := 0; i < count; i++ {
		r, _, sz := b.DecodeGraphemeAt(pos)
		if sz == 0 || r == '\n' {
			break
		}
		pos += sz
	}
	return pos
}

func motionDown(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	line, col := b.LineColAt(c.Pos)
	line += count
	if line > b.NumLines() {
		line = b.NumLines()
	}
	ll := b.LineLen(line)
	if col > ll {
		col = ll
	}
	return b.OffsetAt(line, col)
}

func motionUp(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	line, col := b.LineColAt(c.Pos)
	line -= count
	if line < 0 {
		line = 0
	}
	ll := b.LineLen(line)
	if col > ll {
		col = ll
	}
	return b.OffsetAt(line, col)
}

func motionLineStart(_ *Buffer, c Cursor, _ int) int {
	return c.Pos // will be overridden
}

func motionBOL(b *Buffer, c Cursor, _ int) int {
	return c.LineStart(b).Pos
}

func motionFirstNonBlank(b *Buffer, c Cursor, _ int) int {
	pos := c.LineStart(b).Pos
	for {
		r, _, sz := b.DecodeGraphemeAt(pos)
		if r == '\n' || sz == 0 || !unicode.IsSpace(r) {
			break
		}
		pos += sz
	}
	return pos
}

func motionEOL(b *Buffer, c Cursor, _ int) int {
	return c.LineEnd(b).Pos
}

func motionFileTop(b *Buffer, _ Cursor, count int) int {
	if count > 0 {
		line := count - 1
		if line > b.NumLines() {
			line = b.NumLines()
		}
		return b.OffsetAt(line, 0)
	}
	return 0
}

func motionFileBottom(b *Buffer, _ Cursor, count int) int {
	if count > 0 {
		line := count - 1
		if line > b.NumLines() {
			line = b.NumLines()
		}
		return b.OffsetAt(line, 0)
	}
	return b.Len()
}

func motionWordRight(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		c = c.WordRight(b, IsWordChar)
	}
	return c.Pos
}

func motionWordLeft(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		c = c.WordLeft(b, IsWordChar)
	}
	return c.Pos
}

func motionWordEnd(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		c = c.WordEnd(b, IsWordChar)
	}
	return c.Pos
}

func motionWORDRight(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		c = c.WordRight(b, IsNotSpace)
	}
	return c.Pos
}

func motionWORDLeft(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		c = c.WordLeft(b, IsNotSpace)
	}
	return c.Pos
}

func motionWORDEnd(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		c = c.WordEnd(b, IsNotSpace)
	}
	return c.Pos
}

func motionFindChar(b *Buffer, c Cursor, count int, ch rune) int {
	if count == 0 {
		count = 1
	}
	pos := c.Pos
	for i := 0; i < count; i++ {
		_, _, sz := b.DecodeGraphemeAt(pos)
		pos += sz
		for {
			r, _, sz := b.DecodeGraphemeAt(pos)
			if r == '\n' || sz == 0 {
				return c.Pos // not found
			}
			if r == ch {
				break
			}
			pos += sz
		}
	}
	return pos
}

func motionFindCharBack(b *Buffer, c Cursor, count int, ch rune) int {
	if count == 0 {
		count = 1
	}
	pos := c.Pos
	for i := 0; i < count; i++ {
		_, _, sz := b.DecodeGraphemeBefore(pos)
		pos -= sz
		for {
			r, _, sz := b.DecodeGraphemeBefore(pos)
			if r == '\n' || sz == 0 {
				return c.Pos
			}
			pos -= sz
			if r == ch {
				break
			}
		}
	}
	return pos
}

func motionTillChar(b *Buffer, c Cursor, count int, ch rune) int {
	pos := motionFindChar(b, c, count, ch)
	if pos != c.Pos {
		_, _, sz := b.DecodeGraphemeBefore(pos)
		pos -= sz
	}
	return pos
}

func motionTillCharBack(b *Buffer, c Cursor, count int, ch rune) int {
	pos := motionFindCharBack(b, c, count, ch)
	if pos != c.Pos {
		_, _, sz := b.DecodeGraphemeAt(pos)
		pos += sz
	}
	return pos
}

func motionParaUp(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	line, _ := b.LineColAt(c.Pos)
	for i := 0; i < count; i++ {
		for line > 0 && b.LineLen(line) == 0 {
			line--
		}
		for line > 0 && b.LineLen(line) > 0 {
			line--
		}
	}
	return b.OffsetAt(line, 0)
}

func motionParaDown(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	line, _ := b.LineColAt(c.Pos)
	numLines := b.NumLines()
	for i := 0; i < count; i++ {
		for line < numLines && b.LineLen(line) == 0 {
			line++
		}
		for line < numLines && b.LineLen(line) > 0 {
			line++
		}
	}
	return b.OffsetAt(line, 0)
}

// --- Registration ---

// RegisterMotions registers all motion bindings.
func RegisterMotions(ks *KeyState) {
	registerMotion(ks, []string{"h"}, MotionDef{Fn: motionLeft})
	registerMotion(ks, []string{KeyLeft}, MotionDef{Fn: motionLeft})
	registerMotion(ks, []string{"l"}, MotionDef{Fn: motionRight})
	registerMotion(ks, []string{KeyRight}, MotionDef{Fn: motionRight})
	registerMotion(ks, []string{"j"}, MotionDef{Fn: motionDown})
	registerMotion(ks, []string{KeyDown}, MotionDef{Fn: motionDown})
	registerMotion(ks, []string{"k"}, MotionDef{Fn: motionUp})
	registerMotion(ks, []string{KeyUp}, MotionDef{Fn: motionUp})

	registerMotion(ks, []string{"0"}, MotionDef{Fn: motionBOL})
	registerMotion(ks, []string{"^"}, MotionDef{Fn: motionFirstNonBlank})
	registerMotion(ks, []string{KeyHome}, MotionDef{Fn: motionBOL})
	registerMotion(ks, []string{"$"}, MotionDef{Fn: motionEOL})
	registerMotion(ks, []string{KeyEnd}, MotionDef{Fn: motionEOL})

	registerMotion(ks, []string{"g", "g"}, MotionDef{Fn: motionFileTop})
	registerMotion(ks, []string{"G"}, MotionDef{Fn: motionFileBottom})

	registerMotion(ks, []string{"w"}, MotionDef{Fn: motionWordRight, Name: "w"})
	registerMotion(ks, []string{"b"}, MotionDef{Fn: motionWordLeft, Name: "b"})
	registerMotion(ks, []string{"e"}, MotionDef{Fn: motionWordEnd, Flags: Inclusive, Name: "e"})
	registerMotion(ks, []string{"W"}, MotionDef{Fn: motionWORDRight, Name: "W"})
	registerMotion(ks, []string{"B"}, MotionDef{Fn: motionWORDLeft, Name: "B"})
	registerMotion(ks, []string{"E"}, MotionDef{Fn: motionWORDEnd, Flags: Inclusive, Name: "E"})

	registerMotion(ks, []string{"{"}, MotionDef{Fn: motionParaUp})
	registerMotion(ks, []string{"}"}, MotionDef{Fn: motionParaDown})

	// Ctrl-D / Ctrl-U: half-page down/up. These need the view height, so
	// the motion function captures it dynamically via the KeyState's buffer.
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		halfPage := 10 // fallback
		if ks.halfPageSize != nil {
			halfPage = ks.halfPageSize()
		}
		applyMotion(ks, MotionDef{Fn: func(b *Buffer, c Cursor, count int) int {
			if count == 0 {
				count = 1
			}
			return motionDown(b, c, halfPage*count)
		}}, false)
	}, "<C-d>")

	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		halfPage := 10
		if ks.halfPageSize != nil {
			halfPage = ks.halfPageSize()
		}
		applyMotion(ks, MotionDef{Fn: func(b *Buffer, c Cursor, count int) int {
			if count == 0 {
				count = 1
			}
			return motionUp(b, c, halfPage*count)
		}}, false)
	}, "<C-u>")

	registerCharMotion(ks, "f", motionFindChar, Inclusive)
	registerCharMotion(ks, "F", motionFindCharBack, Charwise)
	registerCharMotion(ks, "t", motionTillChar, Inclusive)
	registerCharMotion(ks, "T", motionTillCharBack, Charwise)
}
