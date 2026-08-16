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
// current cursor, or a negative value if the motion failed (no match for
// f/t/%, j at the last line, ...): a failed motion moves nothing and
// aborts a pending operator, as in vim. Count is the raw count (0 = unset).
// Name is used for special-casing (e.g. vim's cw→ce quirk).
type MotionDef struct {
	Fn       func(b *Buffer, c Cursor, count int) int
	Flags    MotionFlags
	Name     string
	Vertical bool // true for j/k: preserve Vx instead of recalculating
}

// --- Motion application helpers ---

// applyMotion moves or extends selection for all cursors.
func applyMotion(ks *KeyState, m MotionDef, selecting bool) {
	b := ks.Buf()
	count := ks.RawCount()
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		newPos := m.Fn(b, c, count)
		if newPos < 0 {
			continue // failed motion: don't move this cursor
		}
		newPos = clamp(newPos, 0, b.Len())
		if selecting {
			blockEOL := false
			if ks.ModeID() == ModeVisualBlock {
				// $ pins the block's right edge to the end of each line;
				// any other horizontal motion unpins it.
				blockEOL = c.BlockEOL
				if m.Name == "$" {
					blockEOL = true
				} else if !m.Vertical {
					blockEOL = false
				}
				if blockEOL && m.Vertical {
					newPos = (Cursor{Pos: newPos}).LineEnd(b).Pos
				}
			}
			// In charwise and block visual mode the cursor cannot rest on a
			// line's newline (vim: v$ selects up to the last character).
			if ks.ModeID() == ModeVisual || ks.ModeID() == ModeVisualBlock {
				newPos = (Cursor{Pos: newPos}).VimClamp(b).Pos
			}
			b.cursors[i] = c.SelectTo(newPos)
			if ks.ModeID() == ModeVisual {
				adjustVisualChar(b, &b.cursors[i])
			} else if ks.ModeID() == ModeVisualLine {
				adjustVisualLine(b, &b.cursors[i])
			} else if ks.ModeID() == ModeVisualBlock {
				adjustVisualChar(b, &b.cursors[i])
				b.cursors[i].BlockSel = true
				b.cursors[i].BlockEOL = blockEOL
			}
		} else {
			b.cursors[i] = c.MoveTo(newPos)
			if ks.ModeID() == ModeNormal {
				b.cursors[i] = b.cursors[i].VimClamp(b)
			}
		}
	}
	if m.Vertical {
		ks.vertical = true
	}
	// Pure motions in normal mode are not repeatable — discard recording.
	if !selecting && ks.ModeID() == ModeNormal {
		ks.recording = nil
	}
	ks.ClearCounts()
}

// adjustVisualChar ensures the selection includes the grapheme at the cursor
// position, since visual charwise mode is inclusive of the cursor character.
func adjustVisualChar(b *Buffer, c *Cursor) {
	if !c.HasSel {
		return
	}
	if c.Pos >= c.Orig[0] {
		// Forward: extend Sel[1] past the grapheme at cursor
		_, _, sz := b.DecodeGraphemeAt(c.Pos)
		if sz > 0 {
			c.Sel[1] = c.Pos + sz
		}
	} else {
		// Backward: extend Sel[1] past the grapheme at anchor
		_, _, sz := b.DecodeGraphemeAt(c.Orig[0])
		if sz > 0 {
			c.Sel[1] = c.Orig[0] + sz
		}
	}
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
	if m.Flags&Linewise != 0 {
		ks.forceLinewise = true
	}
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		start := c.Pos
		end := m.Fn(b, c, count)
		if end < 0 {
			continue // failed motion: the operation does nothing
		}
		if start > end {
			start, end = end, start
		}
		if m.Flags&Inclusive != 0 && end < b.Len() {
			_, _, sz := b.DecodeGraphemeAt(end)
			end += sz
		}
		if m.Flags&Linewise != 0 {
			start, end = extendToLines(b, start, end)
			if op.Key == "d" {
				start = linewiseEOFAdjust(b, start, end)
			}
		}
		if start == end {
			// Empty range (e.g. dh at column 0): vim aborts the operation
			// without touching the register.
			continue
		}
		op.Fn(ks, b, start, end)
	}
	ks.forceLinewise = false
	ks.ResetAction()
}

// linewiseEOFAdjust extends a linewise delete range to include the newline
// preceding it when the range reaches EOF without a trailing newline, so
// deleting the last line(s) doesn't leave a dangling empty line.
func linewiseEOFAdjust(b *Buffer, start, end int) int {
	if end == b.Len() && start > 0 && (end == start || b.ByteAt(end-1) != '\n') {
		return start - 1
	}
	return start
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

// registerMotion binds a motion in normal, visual, visual-line, visual-block,
// and operator-pending modes.
func registerMotion(ks *KeyState, keys []string, m MotionDef) {
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		applyMotion(ks, m, false)
	}, keys...)
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine, ModeVisualBlock} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			applyMotion(ks, m, true)
		}, keys...)
	}
	ks.modes[ModeOperatorPending].Bindings.Bind(func(ks *KeyState) {
		execMotionOp(ks, m)
	}, keys...)
}

// registerCharMotion binds a char-argument motion (f, t, F, T) in all motion
// modes. reverseFn is the opposite direction for ; and , repeat.
func registerCharMotion(ks *KeyState, key string, fn func(b *Buffer, c Cursor, count int, ch rune) int, reverseFn func(b *Buffer, c Cursor, count int, ch rune) int, flags MotionFlags) {
	handler := func(ks *KeyState) {
		ks.WaitForChar(func(ks *KeyState, ch string) {
			rs := []rune(ch)
			if len(rs) != 1 {
				// Special key (<Esc>, <CR>, ...): cancel.
				ks.ResetAction()
				return
			}
			// Save for ; and , repeat.
			ks.lastCharSearch = charSearch{
				fn: fn, reverse: reverseFn, ch: rs[0], flags: flags,
			}
			m := MotionDef{
				Fn: func(b *Buffer, c Cursor, count int) int {
					return fn(b, c, count, rs[0])
				},
				Flags: flags,
			}
			if ks.Pending() != nil {
				execMotionOp(ks, m)
			} else if ks.Mode().IsVisual {
				applyMotion(ks, m, true)
			} else {
				applyMotion(ks, m, false)
			}
		})
	}
	for _, mode := range []ModeID{ModeNormal, ModeVisual, ModeVisualLine, ModeVisualBlock, ModeOperatorPending} {
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
	line, _ := b.LineColAt(c.Pos)
	target := line + count
	if target > b.NumLines() {
		target = b.NumLines()
	}
	if target == line {
		return -1 // already at the last line (vim: dj there does nothing)
	}
	return b.VisualLoc(target, c.Vx)
}

func motionUp(b *Buffer, c Cursor, count int) int {
	if count == 0 {
		count = 1
	}
	line, _ := b.LineColAt(c.Pos)
	target := line - count
	if target < 0 {
		target = 0
	}
	if target == line {
		return -1 // already at the first line
	}
	return b.VisualLoc(target, c.Vx)
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
				return -1 // not found
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
				return -1 // not found
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
	if pos >= 0 {
		_, _, sz := b.DecodeGraphemeBefore(pos)
		pos -= sz
	}
	return pos
}

func motionTillCharBack(b *Buffer, c Cursor, count int, ch rune) int {
	pos := motionFindCharBack(b, c, count, ch)
	if pos >= 0 {
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

func motionMatchBracket(b *Buffer, c Cursor, _ int) int {
	open := map[rune]rune{'(': ')', '{': '}', '[': ']'}
	close := map[rune]rune{')': '(', '}': '{', ']': '['}

	r, _ := b.DecodeRuneAt(c.Pos)
	if match, ok := open[r]; ok {
		depth := 0
		pos := c.Pos
		for pos < b.Len() {
			ch, sz := b.DecodeRuneAt(pos)
			if ch == r {
				depth++
			} else if ch == match {
				depth--
				if depth == 0 {
					return pos
				}
			}
			pos += sz
		}
	} else if opener, ok := close[r]; ok {
		depth := 0
		pos := c.Pos
		for pos >= 0 {
			ch, sz := b.DecodeRuneAt(pos)
			if sz == 0 {
				break
			}
			if ch == r {
				depth++
			} else if ch == opener {
				depth--
				if depth == 0 {
					return pos
				}
			}
			_, bsz := b.DecodeRuneBefore(pos)
			if bsz == 0 {
				break
			}
			pos -= bsz
		}
	}
	return -1 // not on a bracket, or no match: the motion fails
}

// --- Registration ---

// RegisterMotions registers all motion bindings.
func RegisterMotions(ks *KeyState) {
	registerMotion(ks, []string{"h"}, MotionDef{Fn: motionLeft})
	registerMotion(ks, []string{KeyLeft}, MotionDef{Fn: motionLeft})
	registerMotion(ks, []string{"l"}, MotionDef{Fn: motionRight})
	registerMotion(ks, []string{KeyRight}, MotionDef{Fn: motionRight})
	registerMotion(ks, []string{"j"}, MotionDef{Fn: motionDown, Vertical: true, Flags: Linewise})
	registerMotion(ks, []string{KeyDown}, MotionDef{Fn: motionDown, Vertical: true, Flags: Linewise})
	registerMotion(ks, []string{"k"}, MotionDef{Fn: motionUp, Vertical: true, Flags: Linewise})
	registerMotion(ks, []string{KeyUp}, MotionDef{Fn: motionUp, Vertical: true, Flags: Linewise})

	// Deliberately swapped from vim: 0 goes to the first non-blank and
	// ^ to column 0. (0 while a count is pending is still a count digit;
	// see tryCount.)
	registerMotion(ks, []string{"0"}, MotionDef{Fn: motionFirstNonBlank})
	registerMotion(ks, []string{"^"}, MotionDef{Fn: motionBOL})
	registerMotion(ks, []string{KeyHome}, MotionDef{Fn: motionBOL})
	registerMotion(ks, []string{"$"}, MotionDef{Fn: motionEOL, Name: "$"})
	registerMotion(ks, []string{KeyEnd}, MotionDef{Fn: motionEOL, Name: "$"})

	registerMotion(ks, []string{"g", "g"}, MotionDef{Fn: motionFileTop, Flags: Linewise})
	registerMotion(ks, []string{"G"}, MotionDef{Fn: motionFileBottom, Flags: Linewise})

	registerMotion(ks, []string{"w"}, MotionDef{Fn: motionWordRight, Name: "w"})
	registerMotion(ks, []string{"b"}, MotionDef{Fn: motionWordLeft, Name: "b"})
	registerMotion(ks, []string{"e"}, MotionDef{Fn: motionWordEnd, Flags: Inclusive, Name: "e"})
	registerMotion(ks, []string{"W"}, MotionDef{Fn: motionWORDRight, Name: "W"})
	registerMotion(ks, []string{"B"}, MotionDef{Fn: motionWORDLeft, Name: "B"})
	registerMotion(ks, []string{"E"}, MotionDef{Fn: motionWORDEnd, Flags: Inclusive, Name: "E"})

	registerMotion(ks, []string{"{"}, MotionDef{Fn: motionParaUp})
	registerMotion(ks, []string{"}"}, MotionDef{Fn: motionParaDown})

	// Ctrl-D / Ctrl-U: half-page scroll. Scrolls the view and moves the
	// cursor by the same amount so the cursor stays at the same screen row.
	// At file boundaries the cursor moves to stay visible.
	scrollHalfPage := func(ks *KeyState, dir int) {
		v := ks.activeView()
		if v == nil {
			return
		}
		b := ks.Buf()
		halfPage := v.height / 2
		count := ks.RawCount()
		if count == 0 {
			count = 1
		}
		scroll := halfPage * count * dir

		// Scroll the view.
		newTop := v.topline + scroll
		if newTop < 0 {
			newTop = 0
		}
		maxTop := b.NumLines() - v.height + 1
		if maxTop < 0 {
			maxTop = 0
		}
		if newTop > maxTop {
			newTop = maxTop
		}
		v.topline = newTop

		// Move cursor by the same amount.
		c := b.Cursor()
		line, _ := b.LineColAt(c.Pos)
		newLine := line + scroll
		if newLine < 0 {
			newLine = 0
		}
		if newLine > b.NumLines() {
			newLine = b.NumLines()
		}
		*c = c.MoveTo(b.VisualLoc(newLine, c.Vx))
		ks.vertical = true
		ks.ClearCounts()
	}
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		scrollHalfPage(ks, 1)
	}, "<C-d>")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		scrollHalfPage(ks, -1)
	}, "<C-u>")

	registerCharMotion(ks, "f", motionFindChar, motionFindCharBack, Inclusive)
	registerCharMotion(ks, "F", motionFindCharBack, motionFindChar, Charwise)
	registerCharMotion(ks, "t", motionTillChar, motionTillCharBack, Inclusive)
	registerCharMotion(ks, "T", motionTillCharBack, motionTillChar, Charwise)

	// %: matching bracket (inclusive: d% deletes both brackets)
	registerMotion(ks, []string{"%"}, MotionDef{Fn: motionMatchBracket, Flags: Inclusive})

	// ;: repeat last f/t/F/T
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if ks.lastCharSearch.fn != nil {
			applyMotion(ks, MotionDef{
				Fn: func(b *Buffer, c Cursor, count int) int {
					return ks.lastCharSearch.fn(b, c, count, ks.lastCharSearch.ch)
				},
				Flags: ks.lastCharSearch.flags,
			}, false)
		}
	}, ";")

	// ,: reverse last f/t/F/T
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if ks.lastCharSearch.reverse != nil {
			applyMotion(ks, MotionDef{
				Fn: func(b *Buffer, c Cursor, count int) int {
					return ks.lastCharSearch.reverse(b, c, count, ks.lastCharSearch.ch)
				},
			}, false)
		}
	}, ",")
}
