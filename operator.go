package main

import (
	"bytes"
	"unicode"
)

// --- Operator functions ---

// opDelete removes the range and copies to register.
func opDelete(ks *KeyState, b *Buffer, start, end int) {
	content := make([]byte, end-start)
	copy(content, b.Slice(start, end))
	linewise := end > start && content[len(content)-1] == '\n'
	ks.regs.SetDefault(content, linewise, false)
	if r := ks.Register(); r != RegDefault {
		ks.regs.Set(r, content, linewise)
	}
	b.Remove(start, end)
}

// opChange removes the range and switches to insert mode.
func opChange(ks *KeyState, b *Buffer, start, end int) {
	opDelete(ks, b, start, end)
	ks.SetMode(ModeInsert)
}

// opYank copies the range to register without modifying the buffer.
func opYank(ks *KeyState, b *Buffer, start, end int) {
	content := make([]byte, end-start)
	copy(content, b.Slice(start, end))
	linewise := end > start && content[len(content)-1] == '\n'
	ks.regs.SetDefault(content, linewise, true)
	if r := ks.Register(); r != RegDefault {
		ks.regs.Set(r, content, linewise)
	}
}

// opIndent adds one level of indentation to each line in the range.
func opIndent(ks *KeyState, b *Buffer, start, end int) {
	sl, _ := b.LineColAt(start)
	el, _ := b.LineColAt(end)
	if el > sl && b.OffsetAt(el, 0) == end {
		el-- // don't indent next line if range ends at BOL
	}
	indent := []byte("\t") // TODO: use tabstospaces option
	for line := el; line >= sl; line-- {
		off := b.OffsetAt(line, 0)
		if b.LineLen(line) > 0 {
			b.Insert(off, indent)
		}
	}
}

// opDedent removes one level of indentation from each line in the range.
func opDedent(ks *KeyState, b *Buffer, start, end int) {
	sl, _ := b.LineColAt(start)
	el, _ := b.LineColAt(end)
	if el > sl && b.OffsetAt(el, 0) == end {
		el--
	}
	for line := el; line >= sl; line-- {
		off := b.OffsetAt(line, 0)
		if off >= b.Len() {
			continue
		}
		r, _ := b.DecodeRuneAt(off)
		if r == '\t' {
			b.Remove(off, off+1)
		} else if r == ' ' {
			// Remove up to tabsize spaces.
			n := 0
			for n < 4 { // TODO: use tabsize option
				if off+n >= b.Len() {
					break
				}
				r, _ := b.DecodeRuneAt(off + n)
				if r != ' ' {
					break
				}
				n++
			}
			if n > 0 {
				b.Remove(off, off+n)
			}
		}
	}
}

// --- Linewise operator execution (for dd, yy, cc, >>, <<) ---

func execLineOp(ks *KeyState, opFn func(*KeyState, *Buffer, int, int)) {
	b := ks.Buf()
	b.UndoBarrier()
	count := ks.RawCount()
	if count == 0 {
		count = 1
	}
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		line, _ := b.LineColAt(c.Pos)
		start, end := lineRange(b, line, count)
		opFn(ks, b, start, end)
	}
	ks.ResetAction()
}

// execLineChange handles cc: deletes line contents without removing the
// line itself, then enters insert mode. For counts > 1 (e.g. 3cc), the
// extra lines are fully removed but one empty line is preserved.
func execLineChange(ks *KeyState) {
	b := ks.Buf()
	b.UndoBarrier()
	count := ks.RawCount()
	if count == 0 {
		count = 1
	}
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		line, _ := b.LineColAt(c.Pos)
		start := b.OffsetAt(line, 0)
		// Remove extra lines entirely (their content + newlines).
		if count > 1 {
			extraStart := b.OffsetAt(line+1, 0)
			extraEnd := b.OffsetAt(line+count, 0)
			if extraEnd > b.Len() {
				extraEnd = b.Len()
			}
			if extraEnd > extraStart {
				opDelete(ks, b, extraStart, extraEnd)
			}
		}
		// Delete the first line's content (preserve the newline).
		end := start + b.LineLen(line)
		if end > start {
			opDelete(ks, b, start, end)
		}
	}
	ks.SetMode(ModeInsert)
	ks.ResetAction()
}

// --- Visual mode operator execution ---

func execVisualOp(ks *KeyState, opFn func(*KeyState, *Buffer, int, int)) {
	b := ks.Buf()
	b.UndoBarrier()
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if !c.HasSelection() {
			continue
		}
		start, end := c.Sel[0], c.Sel[1]
		if ks.ModeID() == ModeVisualLine {
			start, end = extendToLines(b, start, end)
		}
		opFn(ks, b, start, end)
		b.cursors[i].HasSel = false
	}
	if ks.ModeID() != ModeInsert {
		ks.SetMode(ModeNormal)
	}
}

// --- Operator binding helpers ---

func registerOperator(ks *KeyState, key string, opFn func(*KeyState, *Buffer, int, int)) {
	op := &PendingOp{
		Name: key,
		Key:  key,
		Fn:   opFn,
	}

	// Normal mode: set pending operator.
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.SetPending(op)
	}, key)

	// Operator-pending: doubled operator = linewise.
	ks.modes[ModeOperatorPending].Bindings.Bind(func(ks *KeyState) {
		if p := ks.Pending(); p != nil && p.Key == key {
			if key == "c" {
				execLineChange(ks)
			} else {
				execLineOp(ks, opFn)
			}
		}
	}, key)

	// Visual modes: operate on selection.
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			execVisualOp(ks, opFn)
		}, key)
	}
}

// --- Registration ---

// RegisterOperators registers all operator and command bindings.
func RegisterOperators(ks *KeyState) {
	// Operators: d, c, y, >, <
	registerOperator(ks, "d", opDelete)
	registerOperator(ks, "c", opChange)
	registerOperator(ks, "y", opYank)
	registerOperator(ks, ">", opIndent)
	registerOperator(ks, "<", opDedent)

	// --- Normal mode commands ---

	// x: delete char under cursor
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		count := ks.Count()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			end := c.Pos
			for j := 0; j < count; j++ {
				r, _, sz := b.DecodeGraphemeAt(end)
				if r == '\n' || sz == 0 {
					break
				}
				end += sz
			}
			if end > c.Pos {
				opDelete(ks, b, c.Pos, end)
			}
		}
		ks.ResetAction()
	}, "x")

	// X: delete char before cursor
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			start := c.Pos
			_, _, sz := b.DecodeGraphemeBefore(start)
			if sz > 0 {
				r, _ := b.DecodeRuneAt(start - sz)
				if r != '\n' {
					opDelete(ks, b, start-sz, start)
				}
			}
		}
		ks.ResetAction()
	}, "X")

	// D: delete to end of line
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			end := c.LineEnd(b).Pos
			if end > c.Pos {
				opDelete(ks, b, c.Pos, end)
			}
		}
		ks.ResetAction()
	}, "D")

	// C: change to end of line
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			end := c.LineEnd(b).Pos
			if end > c.Pos {
				opDelete(ks, b, c.Pos, end)
			}
		}
		ks.SetMode(ModeInsert)
		ks.ResetAction()
	}, "C")

	// Y: yank line
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		count := ks.Count()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			line, _ := b.LineColAt(c.Pos)
			start, end := lineRange(b, line, count)
			opYank(ks, b, start, end)
		}
		ks.ResetAction()
	}, "Y")

	// p: paste after cursor
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		paste(ks, false)
	}, "p")

	// P: paste before cursor
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		paste(ks, true)
	}, "P")

	// J: join lines
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		count := ks.Count()
		for j := 0; j < count; j++ {
			c := b.Cursor()
			end := c.LineEnd(b).Pos
			if end < b.Len() {
				// Remove newline and leading whitespace on next line.
				nlEnd := end + 1 // past the \n
				for nlEnd < b.Len() {
					r, _, sz := b.DecodeGraphemeAt(nlEnd)
					if !unicode.IsSpace(r) || r == '\n' {
						break
					}
					nlEnd += sz
				}
				b.Remove(end, nlEnd)
				b.Insert(end, []byte(" "))
			}
		}
		ks.ResetAction()
	}, "J")

	// u: undo (not repeatable)
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.recording = nil
		ks.Buf().Undo()
		ks.ResetAction()
	}, "u")

	// Ctrl-R: redo (not repeatable)
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.recording = nil
		ks.Buf().Redo()
		ks.ResetAction()
	}, "<C-r>")

	// .: repeat last action
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.recording = nil // don't record the replay itself
		ks.Replay()
	}, ".")

	// --- Mode switching ---

	// i: insert before cursor
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.SetMode(ModeInsert)
	}, "i")

	// a: insert after cursor
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		c := b.Cursor()
		r, _, sz := b.DecodeGraphemeAt(c.Pos)
		if r != '\n' && sz > 0 {
			*c = c.MoveTo(c.Pos + sz)
		}
		ks.SetMode(ModeInsert)
	}, "a")

	// I: insert at first non-blank
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		c := b.Cursor()
		*c = c.MoveTo(motionFirstNonBlank(b, *c, 0))
		ks.SetMode(ModeInsert)
	}, "I")

	// A: insert at end of line
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		c := b.Cursor()
		*c = c.MoveTo(c.LineEnd(b).Pos)
		ks.SetMode(ModeInsert)
	}, "A")

	// o: open line below
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		c := b.Cursor()
		end := c.LineEnd(b).Pos
		b.Insert(end, []byte("\n"))
		// Move cursor to the new line (end+1 because \n was inserted at end).
		*b.Cursor() = b.Cursor().MoveTo(end + 1)
		ks.SetMode(ModeInsert)
	}, "o")

	// O: open line above
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		c := b.Cursor()
		start := c.LineStart(b).Pos
		b.Insert(start, []byte("\n"))
		*b.Cursor() = b.Cursor().MoveTo(start)
		ks.SetMode(ModeInsert)
	}, "O")

	// v: visual mode
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		c := b.Cursor()
		_, _, sz := b.DecodeGraphemeAt(c.Pos)
		c.HasSel = true
		c.Orig = [2]int{c.Pos, c.Pos + sz}
		c.Sel = [2]int{c.Pos, c.Pos + sz}
		ks.SetMode(ModeVisual)
	}, "v")

	// V: visual line mode
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		c := b.Cursor()
		*c = c.SelectTo(c.Pos)
		adjustVisualLine(b, c)
		ks.SetMode(ModeVisualLine)
	}, "V")

	// Escape in visual modes: back to normal
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			b := ks.Buf()
			for i := range b.cursors {
				b.cursors[i].HasSel = false
			}
			ks.SetMode(ModeNormal)
		}, KeyEscape)
		ks.modes[mode].Bindings.Bind(ks.modes[mode].Bindings.root.children[KeyEscape].action, "<C-c>")
	}

	// Escape in operator-pending: cancel
	ks.modes[ModeOperatorPending].Bindings.Bind(func(ks *KeyState) {
		ks.ResetAction()
	}, KeyEscape)
	ks.modes[ModeOperatorPending].Bindings.Bind(ks.modes[ModeOperatorPending].Bindings.root.children[KeyEscape].action, "<C-c>")

	// --- Insert mode ---

	// Escape: back to normal, move cursor left
	ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		c := b.Cursor()
		if c.Pos > 0 {
			_, _, sz := b.DecodeGraphemeBefore(c.Pos)
			r, _ := b.DecodeRuneAt(c.Pos - sz)
			if r != '\n' {
				*c = c.MoveTo(c.Pos - sz)
			}
		}
		ks.SetMode(ModeNormal)
	}, KeyEscape)
	ks.modes[ModeInsert].Bindings.Bind(ks.modes[ModeInsert].Bindings.root.children[KeyEscape].action, "<C-c>")

	// Backspace in insert mode
	ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			if c.Pos > 0 {
				_, _, sz := b.DecodeGraphemeBefore(c.Pos)
				b.Remove(c.Pos-sz, c.Pos)
			}
		}
	}, KeyBacksp)

	// Delete in insert mode
	ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			_, _, sz := b.DecodeGraphemeAt(c.Pos)
			if sz > 0 {
				b.Remove(c.Pos, c.Pos+sz)
			}
		}
	}, KeyDelete)

	// Enter in insert mode
	ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := 0; i < b.NumCursors(); i++ {
			b.Insert(b.cursors[i].Pos, []byte("\n"))
		}
	}, KeyEnter)

	// Tab in insert mode is handled by registerCompletionBindings
	// (triggers completion or inserts tab depending on context).

	// Default key handler for insert mode: insert the character.
	ks.modes[ModeInsert].OnKey = func(ks *KeyState, key string) {
		if len(key) == 0 || (len(key) > 1 && key[0] == '<') {
			return // ignore unbound special keys
		}
		b := ks.Buf()
		data := []byte(key)
		for i := 0; i < b.NumCursors(); i++ {
			b.Insert(b.cursors[i].Pos, data)
		}
	}

	// Replace mode Escape
	ks.modes[ModeReplace].Bindings.Bind(func(ks *KeyState) {
		ks.SetMode(ModeNormal)
	}, KeyEscape)

	// Replace mode: overwrite char and advance
	ks.modes[ModeReplace].OnKey = func(ks *KeyState, key string) {
		if len(key) == 0 || (len(key) > 1 && key[0] == '<') {
			return
		}
		b := ks.Buf()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			r, _, sz := b.DecodeGraphemeAt(c.Pos)
			if sz > 0 && r != '\n' {
				b.Remove(c.Pos, c.Pos+sz)
			}
			b.Insert(c.Pos, []byte(key))
		}
	}

	// R: enter replace mode
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.Buf().UndoBarrier()
		ks.SetMode(ModeReplace)
	}, "R")

	// r<char>: replace char under cursor
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.WaitForChar(func(ks *KeyState, ch string) {
			b := ks.Buf()
			b.UndoBarrier()
			count := ks.Count()
			for i := 0; i < b.NumCursors(); i++ {
				c := b.cursors[i]
				for j := 0; j < count; j++ {
					r, _, sz := b.DecodeGraphemeAt(c.Pos)
					if sz == 0 || r == '\n' {
						break
					}
					b.Remove(c.Pos, c.Pos+sz)
					b.Insert(c.Pos, []byte(ch))
				}
			}
			ks.ResetAction()
		})
	}, "r")

	// s: delete char + enter insert
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		count := ks.Count()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			end := c.Pos
			for j := 0; j < count; j++ {
				r, _, sz := b.DecodeGraphemeAt(end)
				if r == '\n' || sz == 0 {
					break
				}
				end += sz
			}
			if end > c.Pos {
				opDelete(ks, b, c.Pos, end)
			}
		}
		ks.SetMode(ModeInsert)
		ks.ResetAction()
	}, "s")

	// S: delete line contents + enter insert
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			line, _ := b.LineColAt(c.Pos)
			start := b.OffsetAt(line, 0)
			end := start + b.LineLen(line)
			if end > start {
				opDelete(ks, b, start, end)
			}
		}
		ks.SetMode(ModeInsert)
		ks.ResetAction()
	}, "S")

	// ~: toggle case and advance
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		count := ks.Count()
		for i := 0; i < b.NumCursors(); i++ {
			for j := 0; j < count; j++ {
				c := b.cursors[i]
				r, _, sz := b.DecodeGraphemeAt(c.Pos)
				if sz == 0 || r == '\n' {
					break
				}
				var repl rune
				if unicode.IsUpper(r) {
					repl = unicode.ToLower(r)
				} else {
					repl = unicode.ToUpper(r)
				}
				b.Remove(c.Pos, c.Pos+sz)
				b.Insert(c.Pos, []byte(string(repl)))
			}
		}
		ks.ResetAction()
	}, "~")

	// .: repeat last edit
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.Replay()
	}, ".")

	// H/M/L: screen top/middle/bottom
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			*v.buf.Cursor() = v.buf.Cursor().MoveTo(v.buf.OffsetAt(v.topline, 0))
		}
		ks.ResetAction()
	}, "H")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			mid := v.topline + v.height/2
			*v.buf.Cursor() = v.buf.Cursor().MoveTo(v.buf.OffsetAt(mid, 0))
		}
		ks.ResetAction()
	}, "M")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			bot := v.topline + v.height - 1
			if bot > v.buf.NumLines() {
				bot = v.buf.NumLines()
			}
			*v.buf.Cursor() = v.buf.Cursor().MoveTo(v.buf.OffsetAt(bot, 0))
		}
		ks.ResetAction()
	}, "L")

	// Ctrl-F/Ctrl-B: full page down/up
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			applyMotion(ks, MotionDef{Fn: func(b *Buffer, c Cursor, _ int) int {
				return motionDown(b, c, v.height*ks.Count())
			}}, false)
		}
	}, "<C-f>")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			applyMotion(ks, MotionDef{Fn: func(b *Buffer, c Cursor, _ int) int {
				return motionUp(b, c, v.height*ks.Count())
			}}, false)
		}
	}, "<C-b>")

	// Ctrl-E/Ctrl-Y: scroll view without moving cursor
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			v.topline += ks.Count()
			if v.topline > v.buf.NumLines() {
				v.topline = v.buf.NumLines()
			}
		}
		ks.count = 0
	}, "<C-e>")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			v.topline -= ks.Count()
			if v.topline < 0 {
				v.topline = 0
			}
		}
		ks.count = 0
	}, "<C-y>")

	// m<char>: set mark
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.WaitForChar(func(ks *KeyState, ch string) {
			if len(ch) == 1 {
				if ks.marks == nil {
					ks.marks = make(map[byte]int)
				}
				ks.marks[ch[0]] = ks.Buf().Cursor().Pos
			}
		})
	}, "m")

	// '<char>: jump to mark (line start)
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.WaitForChar(func(ks *KeyState, ch string) {
			if len(ch) == 1 {
				if pos, ok := ks.marks[ch[0]]; ok {
					b := ks.Buf()
					if pos > b.Len() {
						pos = b.Len()
					}
					line, _ := b.LineColAt(pos)
					*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(line, 0))
				}
			}
		})
	}, "'")

	// `<char>: jump to mark (exact position)
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.WaitForChar(func(ks *KeyState, ch string) {
			if len(ch) == 1 {
				if pos, ok := ks.marks[ch[0]]; ok {
					b := ks.Buf()
					if pos > b.Len() {
						pos = b.Len()
					}
					*b.Cursor() = b.Cursor().MoveTo(pos)
				}
			}
		})
	}, "`")

	// gu: lowercase operator
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.SetPending(&PendingOp{
			Name: "gu", Key: "gu",
			Fn: func(ks *KeyState, b *Buffer, start, end int) {
				opCase(b, start, end, unicode.ToLower)
			},
		})
	}, "g", "u")
	// gU: uppercase operator
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.SetPending(&PendingOp{
			Name: "gU", Key: "gU",
			Fn: func(ks *KeyState, b *Buffer, start, end int) {
				opCase(b, start, end, unicode.ToUpper)
			},
		})
	}, "g", "U")

	// zz/zt/zb: scroll view to center/top/bottom cursor
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			line, _ := v.buf.LineColAt(v.buf.Cursor().Pos)
			v.topline = line - v.height/2
			if v.topline < 0 {
				v.topline = 0
			}
		}
		ks.ResetAction()
	}, "z", "z")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			line, _ := v.buf.LineColAt(v.buf.Cursor().Pos)
			v.topline = line
		}
		ks.ResetAction()
	}, "z", "t")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			line, _ := v.buf.LineColAt(v.buf.Cursor().Pos)
			v.topline = line - v.height + 1
			if v.topline < 0 {
				v.topline = 0
			}
		}
		ks.ResetAction()
	}, "z", "b")

	// Ctrl-W in insert mode: delete word before cursor
	ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			start := c.WordLeft(b, IsWordChar).Pos
			if start < c.Pos {
				b.Remove(start, c.Pos)
			}
		}
	}, "<C-w>")

	// Ctrl-U in insert mode: delete to start of line
	ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			start := c.LineStart(b).Pos
			if start < c.Pos {
				b.Remove(start, c.Pos)
			}
		}
	}, "<C-u>")

	// o in visual mode: swap cursor to other end of selection
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			b := ks.Buf()
			for i := range b.cursors {
				c := &b.cursors[i]
				if c.HasSel {
					if c.Pos == c.Sel[0] {
						c.Pos = c.Sel[1]
					} else {
						c.Pos = c.Sel[0]
					}
					c.Orig[0], c.Orig[1] = c.Orig[1], c.Orig[0]
				}
			}
		}, "o")
	}

	// u/U in visual mode: lowercase/uppercase selection
	ks.modes[ModeVisual].Bindings.Bind(func(ks *KeyState) {
		execVisualOp(ks, func(ks *KeyState, b *Buffer, start, end int) {
			opCase(b, start, end, unicode.ToLower)
		})
	}, "u")
	ks.modes[ModeVisual].Bindings.Bind(func(ks *KeyState) {
		execVisualOp(ks, func(ks *KeyState, b *Buffer, start, end int) {
			opCase(b, start, end, unicode.ToUpper)
		})
	}, "U")
}

// paste inserts register content. If before is true, paste before cursor.
func paste(ks *KeyState, before bool) {
	b := ks.Buf()
	b.UndoBarrier()
	reg := ks.regs.Get(ks.Register())
	if len(reg.Content) == 0 {
		ks.ResetAction()
		return
	}
	count := ks.Count()
	content := bytes.Repeat(reg.Content, count)

	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if reg.Linewise {
			if before {
				pos := c.LineStart(b).Pos
				b.Insert(pos, content)
			} else {
				pos := c.LineEnd(b).Pos + 1
				if pos > b.Len() {
					b.Insert(b.Len(), append([]byte("\n"), content...))
				} else {
					b.Insert(pos, content)
				}
			}
		} else {
			pos := c.Pos
			if !before {
				_, _, sz := b.DecodeGraphemeAt(pos)
				if sz > 0 {
					pos += sz
				}
			}
			b.Insert(pos, content)
		}
	}
	ks.ResetAction()
}

// opCase changes the case of characters in [start, end) using transform.
// Processes in reverse to keep offsets stable.
func opCase(b *Buffer, start, end int, transform func(rune) rune) {
	pos := end
	for pos > start {
		r, sz := b.DecodeRuneBefore(pos)
		pos -= sz
		tr := transform(r)
		if tr != r {
			b.Remove(pos, pos+sz)
			b.Insert(pos, []byte(string(tr)))
		}
	}
}
