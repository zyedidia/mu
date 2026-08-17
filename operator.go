package main

import (
	"bytes"
	"strings"
	"unicode"
)

// --- Operator functions ---

// opDelete removes the range and copies to register.
func opDelete(ks *KeyState, b *Buffer, start, end int) {
	content := make([]byte, end-start)
	copy(content, b.Slice(start, end))
	content, linewise := normalizeRegContent(ks, content)
	ks.regs.SetDefault(content, linewise, false)
	if r := ks.Register(); r != RegDefault {
		ks.regs.Set(r, content, linewise)
	}
	b.Remove(start, end)
}

// normalizeRegContent determines the linewise flag for register content.
// When a linewise operation is forced (dd/yy/V-mode) but the range has no
// trailing newline (last line of the file), the register copy is fixed up
// to end with a newline so a later paste inserts it as a full line.
func normalizeRegContent(ks *KeyState, content []byte) ([]byte, bool) {
	linewise := len(content) > 0 && content[len(content)-1] == '\n'
	if ks.forceLinewise && !linewise {
		if len(content) > 0 && content[0] == '\n' {
			// The range included the preceding newline (EOF delete):
			// move it to the end for the register copy.
			content = append(content[1:], '\n')
		} else {
			content = append(content, '\n')
		}
		linewise = true
	}
	return content, linewise
}

// opChange removes the range and switches to insert mode. For linewise
// ranges (ending with '\n'), it opens a new line at the deletion point
// rather than leaving the cursor on the next line's content.
func opChange(ks *KeyState, b *Buffer, start, end int) {
	linewise := end > start && b.Slice(end-1, end)[0] == '\n'
	opDelete(ks, b, start, end)
	if linewise {
		pos := start
		if pos > 0 {
			// Insert newline after the previous line's end.
			b.Insert(pos-1, []byte("\n"))
			// pos now points to the new empty line.
		} else {
			// Deleted from the top of the file; insert newline before.
			b.Insert(0, []byte("\n"))
			pos = 0
		}
		*b.Cursor() = b.Cursor().MoveTo(pos)
		// Apply autoindent from the surrounding line.
		v := ks.ActiveView()
		autoindent := false
		if v != nil && v.Opts != nil {
			autoindent, _ = GetOptBool(v.Opts, "autoindent")
		}
		if autoindent {
			line, _ := b.LineColAt(pos)
			var refLine int
			if line > 0 {
				refLine = line - 1
			} else if line < b.NumLines() {
				refLine = line + 1
			} else {
				refLine = -1
			}
			if refLine >= 0 {
				ws := leadingWS(b.GetLine(refLine))
				if len(ws) > 0 {
					b.Insert(pos, ws)
				}
			}
		}
	}
	ks.SetMode(ModeInsert)
}

// opYank copies the range to register without modifying the buffer.
func opYank(ks *KeyState, b *Buffer, start, end int) {
	content := make([]byte, end-start)
	copy(content, b.Slice(start, end))
	content, linewise := normalizeRegContent(ks, content)
	ks.regs.SetDefault(content, linewise, true)
	if r := ks.Register(); r != RegDefault {
		ks.regs.Set(r, content, linewise)
	}
}

// indentBytes returns the indent string for one level based on view options.
func indentBytes(ks *KeyState) []byte {
	if v := ks.ActiveView(); v != nil && v.Opts != nil {
		if tts, ok := GetOptBool(v.Opts, "tabstospaces"); ok && tts {
			ts := 4
			if n, ok := GetOptInt(v.Opts, "tabsize"); ok && n > 0 {
				ts = n
			}
			return bytes.Repeat([]byte{' '}, ts)
		}
	}
	return []byte("\t")
}

// tabSize returns the configured tab size from view options.
func tabSize(ks *KeyState) int {
	if v := ks.ActiveView(); v != nil && v.Opts != nil {
		if n, ok := GetOptInt(v.Opts, "tabsize"); ok && n > 0 {
			return n
		}
	}
	return 4
}

// autoClosePairs maps opening characters to their closing counterparts.
var autoClosePairs = map[byte]byte{
	'{':  '}',
	'(':  ')',
	'[':  ']',
	'"':  '"',
	'\'': '\'',
	'`':  '`',
}

// autoCloseClosers is the set of closing characters for skip-over detection.
var autoCloseClosers = map[byte]bool{
	'}': true, ')': true, ']': true,
	'"': true, '\'': true, '`': true,
}

// closerFor returns the matching closing rune for an opener.
func closerFor(opener byte) rune {
	switch opener {
	case '{':
		return '}'
	case '(':
		return ')'
	case '[':
		return ']'
	}
	return 0
}

// applyAutoindent inserts indentation at cursor i based on the previous
// line (refLine). It copies leading whitespace and adds an extra indent
// level if the reference line ends with an opening bracket or colon.
// If splitPair is true and the cursor is on a matching closer, it also
// splits the bracket pair across lines.
func applyAutoindent(ks *KeyState, b *Buffer, i int, refLine int, splitPair bool) {
	ref := b.GetLine(refLine)
	ws := leadingWS(ref)
	if len(ws) > 0 {
		b.Insert(b.cursors[i].Pos, ws)
	}
	trimmed := bytes.TrimRight(ref, " \t")
	if len(trimmed) > 0 {
		last := trimmed[len(trimmed)-1]
		if last == '{' || last == '(' || last == '[' || last == ':' {
			b.Insert(b.cursors[i].Pos, indentBytes(ks))
			if splitPair {
				r, _, sz := b.DecodeGraphemeAt(b.cursors[i].Pos)
				closer := closerFor(last)
				if sz > 0 && r == closer {
					pos := b.cursors[i].Pos
					nl := append([]byte("\n"), ws...)
					b.Insert(pos, nl)
					b.cursors[i] = b.cursors[i].MoveTo(pos)
				}
			}
		}
	}
}

// insertNewline inserts a newline at cursor i and applies autoindent if
// enabled. For whitespace-only lines, the whitespace is removed instead
// of copied (matching vis).
func insertNewline(ks *KeyState, b *Buffer, i int) {
	v := ks.ActiveView()
	autoindent := false
	if v != nil && v.Opts != nil {
		autoindent, _ = GetOptBool(v.Opts, "autoindent")
	}

	if !autoindent {
		b.Insert(b.cursors[i].Pos, []byte("\n"))
		return
	}

	line, _ := b.LineColAt(b.cursors[i].Pos)
	curLine := b.GetLine(line)
	ws := leadingWS(curLine)
	trimmed := bytes.TrimRight(curLine, " \t")

	// Whitespace-only line: remove the whitespace and just insert newline.
	if len(trimmed) == 0 && len(ws) > 0 {
		lineStart := b.OffsetAt(line, 0)
		b.Remove(lineStart, lineStart+len(ws))
		b.Insert(b.cursors[i].Pos, []byte("\n"))
		return
	}

	b.Insert(b.cursors[i].Pos, []byte("\n"))

	newLine, _ := b.LineColAt(b.cursors[i].Pos)
	if newLine == 0 {
		return
	}
	applyAutoindent(ks, b, i, newLine-1, true)
}

// cleanAutoindent removes trailing autoindent whitespace from the cursor
// line if the line contains only whitespace. Called when leaving insert mode.
func cleanAutoindent(b *Buffer) {
	for i := range b.cursors {
		c := b.cursors[i]
		line, _ := b.LineColAt(c.Pos)
		lineContent := b.GetLine(line)
		ws := leadingWS(lineContent)
		if len(ws) == len(lineContent) && len(ws) > 0 {
			start := b.OffsetAt(line, 0)
			b.Remove(start, start+len(ws))
		}
	}
}

// leadingWS returns the leading whitespace bytes of a line.
func leadingWS(line []byte) []byte {
	for i, b := range line {
		if b != ' ' && b != '\t' {
			return line[:i]
		}
	}
	return line
}

// opIndent adds one level of indentation to each line in the range.
func opIndent(ks *KeyState, b *Buffer, start, end int) {
	sl, _ := b.LineColAt(start)
	el, _ := b.LineColAt(end)
	if el > sl && b.OffsetAt(el, 0) == end {
		el-- // don't indent next line if range ends at BOL
	}
	indent := indentBytes(ks)
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
	ts := tabSize(ks)
	for line := el; line >= sl; line-- {
		off := b.OffsetAt(line, 0)
		if off >= b.Len() {
			continue
		}
		r, _ := b.DecodeRuneAt(off)
		if r == '\t' {
			b.Remove(off, off+1)
		} else if r == ' ' {
			n := 0
			for n < ts {
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

func execLineOp(ks *KeyState, key string, opFn func(*KeyState, *Buffer, int, int)) {
	b := ks.Buf()
	b.UndoBarrier()
	count := ks.RawCount()
	if count == 0 {
		count = 1
	}
	ks.forceLinewise = true
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		line, _ := b.LineColAt(c.Pos)
		start, end := lineRange(b, line, count)
		if key == "d" {
			start = linewiseEOFAdjust(b, start, end)
		}
		opFn(ks, b, start, end)
	}
	ks.forceLinewise = false
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
	ks.forceLinewise = true
	defer func() { ks.forceLinewise = false }()
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
		// Autoindent from neighboring line.
		v := ks.ActiveView()
		autoindent := false
		if v != nil && v.Opts != nil {
			autoindent, _ = GetOptBool(v.Opts, "autoindent")
		}
		if autoindent {
			refLine := -1
			if line > 0 {
				refLine = line - 1
			} else if line+1 <= b.NumLines() {
				refLine = line + 1
			}
			if refLine >= 0 {
				applyAutoindent(ks, b, i, refLine, false)
			}
		}
	}
	ks.SetMode(ModeInsert)
	ks.ResetAction()
}

// --- Visual mode operator execution ---

func execVisualOp(ks *KeyState, key string, opFn func(*KeyState, *Buffer, int, int)) {
	b := ks.Buf()
	b.UndoBarrier()
	lineMode := ks.ModeID() == ModeVisualLine
	if lineMode {
		ks.forceLinewise = true
	}
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if !c.HasSelection() {
			continue
		}
		start, end := c.Sel[0], c.Sel[1]
		if lineMode && key == "d" {
			start = linewiseEOFAdjust(b, start, end)
		}
		opFn(ks, b, start, end)
		b.cursors[i].HasSel = false
	}
	ks.forceLinewise = false
	if ks.ModeID() != ModeInsert {
		ks.SetMode(ModeNormal)
	}
	// Clear the register/count so a "x prefix doesn't leak into the next
	// operation.
	ks.ResetAction()
}

// joinLineAt joins line with the following line, replacing the newline and
// the next line's leading whitespace with a single space (vim J). Returns
// the position of the inserted space, or -1 when there is no next line.
func joinLineAt(b *Buffer, line int) int {
	end := b.OffsetAt(line, 0) + b.LineLen(line)
	if end >= b.Len() {
		return -1
	}
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
	return end
}

// execVisualJoin joins the lines covered by the selection (vim v_J). A
// selection within one line joins it with the next, like normal-mode J.
// The cursor is left on the last inserted space.
func execVisualJoin(ks *KeyState) {
	b := ks.Buf()
	b.UndoBarrier()
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if !c.HasSelection() {
			continue
		}
		sl, _ := b.LineColAt(c.Sel[0])
		el, _ := b.LineColAt(c.Sel[1])
		if el > sl && b.OffsetAt(el, 0) == c.Sel[1] {
			el--
		}
		joins := el - sl
		if joins < 1 {
			joins = 1
		}
		pos := -1
		for j := 0; j < joins; j++ {
			p := joinLineAt(b, sl)
			if p < 0 {
				break
			}
			pos = p
		}
		if pos >= 0 {
			b.cursors[i] = b.cursors[i].MoveTo(pos).VimClamp(b)
		} else {
			b.cursors[i].HasSel = false
		}
	}
	ks.SetMode(ModeNormal)
	ks.ResetAction()
}

// execVisualReplace replaces every character in the selection with ch,
// keeping newlines (vim v_r). The cursor is left at the selection start.
func execVisualReplace(ks *KeyState, ch string) {
	b := ks.Buf()
	b.UndoBarrier()
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if !c.HasSelection() {
			continue
		}
		start, end := c.Sel[0], c.Sel[1]
		var out []byte
		for pos := start; pos < end; {
			r, _, sz := b.DecodeGraphemeAt(pos)
			if sz == 0 {
				break
			}
			if r == '\n' {
				out = append(out, '\n')
			} else {
				out = append(out, ch...)
			}
			pos += sz
		}
		b.Remove(start, end)
		b.Insert(start, out)
		b.cursors[i] = b.cursors[i].MoveTo(start).VimClamp(b)
	}
	ks.SetMode(ModeNormal)
	ks.ResetAction()
}

// --- Operator binding helpers ---

// execDoubledOp runs the pending operator linewise if the pressed key
// doubles it: either the operator's full key (dd) or the last key of a
// multi-key operator (gcc, gcgc).
func execDoubledOp(ks *KeyState, key string) {
	p := ks.Pending()
	if p == nil || (p.Key != key && !strings.HasSuffix(p.Key, key)) {
		return
	}
	if p.Key == "c" {
		execLineChange(ks)
	} else {
		execLineOp(ks, p.Key, p.Fn)
	}
}

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

	// Operator-pending: doubled operator = linewise (dd, yy). Repeating
	// just the last key of a multi-key operator also counts (vim: gcc,
	// guu), so the handler dispatches on the pending operator, not the
	// bound key.
	ks.modes[ModeOperatorPending].Bindings.Bind(func(ks *KeyState) {
		execDoubledOp(ks, key)
	}, key)

	// Visual modes: operate on selection.
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			execVisualOp(ks, key, opFn)
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
		ks.forceLinewise = true
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			line, _ := b.LineColAt(c.Pos)
			start, end := lineRange(b, line, count)
			opYank(ks, b, start, end)
		}
		ks.forceLinewise = false
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

	// p/P in visual modes: replace selection with register content.
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine} {
		for _, key := range []string{"p", "P"} {
			ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
				visualPaste(ks)
			}, key)
		}
	}

	// J: join lines
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		// A count of N joins N lines (N-1 joins); no count joins 2 lines.
		count := ks.Count()
		if count > 1 {
			count--
		}
		for j := 0; j < count; j++ {
			line, _ := b.LineColAt(b.Cursor().Pos)
			if joinLineAt(b, line) < 0 {
				break
			}
		}
		ks.ResetAction()
	}, "J")

	// J in visual modes: join the selected lines (vim v_J).
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine, ModeVisualBlock} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			execVisualJoin(ks)
		}, "J")
	}

	// u: undo (not repeatable; takes a count as in vim)
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.recording = nil
		for i := 0; i < ks.Count(); i++ {
			ks.Buf().Undo()
		}
		ks.ResetAction()
	}, "u")

	// Ctrl-R: redo (not repeatable; takes a count as in vim)
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.recording = nil
		for i := 0; i < ks.Count(); i++ {
			ks.Buf().Redo()
		}
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
		ks.Buf().UndoBarrier()
		ks.SetMode(ModeInsert)
	}, "i")

	// a: insert after cursor
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
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
		b.UndoBarrier()
		c := b.Cursor()
		*c = c.MoveTo(motionFirstNonBlank(b, *c, 0))
		ks.SetMode(ModeInsert)
	}, "I")

	// A: insert at end of line
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		c := b.Cursor()
		*c = c.MoveTo(c.LineEnd(b).Pos)
		ks.SetMode(ModeInsert)
	}, "A")

	// o: open line below
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		c := b.Cursor()
		*c = c.MoveTo(c.LineEnd(b).Pos)
		insertNewline(ks, b, b.cur)
		ks.SetMode(ModeInsert)
	}, "o")

	// O: open line above
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		b.UndoBarrier()
		c := b.Cursor()
		line, _ := b.LineColAt(c.Pos)
		start := b.OffsetAt(line, 0)
		b.Insert(start, []byte("\n"))
		// Cursor was pushed to start+1; move back to the new empty line.
		*b.Cursor() = b.Cursor().MoveTo(start)
		v := ks.ActiveView()
		autoindent := false
		if v != nil && v.Opts != nil {
			autoindent, _ = GetOptBool(v.Opts, "autoindent")
		}
		if autoindent {
			// Original line is now line+1 after the inserted newline.
			ws := leadingWS(b.GetLine(line + 1))
			if len(ws) > 0 {
				b.Insert(b.Cursor().Pos, ws)
			}
		}
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
		// Anchor at cursor position (not line start) so that switching
		// to charwise visual mode preserves the original column.
		c.Orig = [2]int{c.Pos, c.Pos}
		c.Sel = [2]int{c.Pos, c.Pos}
		c.HasSel = true
		adjustVisualLine(b, c)
		ks.SetMode(ModeVisualLine)
	}, "V")

	// V in visual mode: switch to visual-line, extending selection to full lines.
	ks.modes[ModeVisual].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := range b.cursors {
			adjustVisualLine(b, &b.cursors[i])
		}
		ks.mode = ModeVisualLine
		if ks.onModeChange != nil {
			ks.onModeChange(ModeVisualLine)
		}
	}, "V")

	// v in visual-line mode: switch to visual (charwise).
	ks.modes[ModeVisualLine].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		// Recompute Sel as charwise from Orig and Pos.
		for i := range b.cursors {
			c := &b.cursors[i]
			if c.HasSel {
				c.Sel[0] = min(c.Orig[0], c.Pos)
				c.Sel[1] = max(c.Orig[1], c.Pos)
				adjustVisualChar(b, c)
			}
		}
		ks.mode = ModeVisual
		if ks.onModeChange != nil {
			ks.onModeChange(ModeVisual)
		}
	}, "v")

	// v in visual mode / V in visual-line mode: exit to normal.
	ks.modes[ModeVisual].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := range b.cursors {
			b.cursors[i].HasSel = false
		}
		ks.SetMode(ModeNormal)
		ks.ResetAction()
	}, "v")
	ks.modes[ModeVisualLine].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := range b.cursors {
			b.cursors[i].HasSel = false
		}
		ks.SetMode(ModeNormal)
		ks.ResetAction()
	}, "V")

	// Escape in visual modes: back to normal
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine, ModeVisualBlock} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			b := ks.Buf()
			for i := range b.cursors {
				b.cursors[i].HasSel = false
			}
			ks.SetMode(ModeNormal)
			ks.ResetAction()
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
		// Clean up autoindent whitespace first so it coalesces with the
		// insert-session undo event, then set the barrier for the next edit.
		cleanAutoindent(b)
		b.UndoBarrier()
		// A visual-block insert spawned one cursor per block line; collapse
		// back to the primary cursor (the block's top line).
		if ks.blockInsert {
			ks.blockInsert = false
			b.RemoveCursors()
		}
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
		v := ks.ActiveView()
		autoclose := false
		if v != nil && v.Opts != nil {
			autoclose, _ = GetOptBool(v.Opts, "autoclose")
		}
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			if c.Pos > 0 {
				_, _, sz := b.DecodeGraphemeBefore(c.Pos)
				// Auto-close: delete matching pair if cursor is between them.
				if autoclose && sz == 1 {
					r, _ := b.DecodeRuneAt(c.Pos - sz)
					if closer, ok := autoClosePairs[byte(r)]; ok {
						nr, _, nsz := b.DecodeGraphemeAt(c.Pos)
						if nsz > 0 && byte(nr) == closer {
							b.Remove(c.Pos, c.Pos+nsz)
						}
					}
				}
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

	// Enter in insert mode (with auto-indent)
	ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := 0; i < b.NumCursors(); i++ {
			insertNewline(ks, b, i)
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
		v := ks.ActiveView()
		autoclose := false
		if v != nil && v.Opts != nil {
			autoclose, _ = GetOptBool(v.Opts, "autoclose")
		}
		data := []byte(key)
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			if autoclose && len(data) == 1 {
				ch := data[0]
				// Skip over closing character if it matches what's under cursor.
				if autoCloseClosers[ch] {
					r, _, sz := b.DecodeGraphemeAt(c.Pos)
					if sz > 0 && byte(r) == ch {
						b.cursors[i] = c.MoveTo(c.Pos + sz)
						continue
					}
				}
				// Insert pair for opening character.
				if closer, ok := autoClosePairs[ch]; ok {
					// For quotes, don't pair if the character before cursor is a letter/digit
					// (likely mid-word, e.g. contractions like "don't").
					if ch == '\'' || ch == '"' || ch == '`' {
						if c.Pos > 0 {
							pr, _ := b.DecodeRuneBefore(c.Pos)
							if pr != 0 && (unicode.IsLetter(pr) || unicode.IsDigit(pr)) {
								b.Insert(c.Pos, data)
								continue
							}
						}
					}
					pair := []byte{ch, closer}
					b.Insert(c.Pos, pair)
					// Insert advances cursor past both chars; move back before the closer.
					b.cursors[i] = b.cursors[i].MoveTo(b.cursors[i].Pos - 1)
					continue
				}
			}
			b.Insert(c.Pos, data)
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
			switch ch {
			case KeyEnter:
				ch = "\n"
			case KeyTab:
				ch = "\t"
			default:
				if len([]rune(ch)) != 1 {
					// Special key (<Esc>, arrows, ...): cancel.
					ks.ResetAction()
					return
				}
			}
			b := ks.Buf()
			b.UndoBarrier()
			count := ks.Count()
			for i := 0; i < b.NumCursors(); i++ {
				c := b.cursors[i]
				// Measure count characters from the cursor; as in vim, the
				// command fails without changing anything if the line is
				// too short.
				end := c.Pos
				ok := true
				for j := 0; j < count; j++ {
					r, _, sz := b.DecodeGraphemeAt(end)
					if sz == 0 || r == '\n' {
						ok = false
						break
					}
					end += sz
				}
				if !ok {
					continue
				}
				if ch == "\n" {
					// {count}r<CR>: the replaced characters become a single
					// line break, cursor at the start of the new line.
					b.Remove(c.Pos, end)
					b.Insert(c.Pos, []byte("\n"))
					b.cursors[i] = b.cursors[i].MoveTo(c.Pos + 1).VimClamp(b)
				} else {
					repl := bytes.Repeat([]byte(ch), count)
					b.Remove(c.Pos, end)
					b.Insert(c.Pos, repl)
					// Cursor on the last replaced character, as in vim.
					b.cursors[i] = b.cursors[i].MoveTo(c.Pos + len(repl) - len(ch))
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

	// H/M/L: screen top/middle/bottom (by visual rows). H and L stay inside
	// the scroll margin so they don't force the viewport to move (vim
	// scrolloff).
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			tl, tr := v.topRow()
			n := 0
			if tl > 0 || tr > 0 {
				n = v.effScrollMargin()
			}
			l, r := v.stepRows(tl, tr, n)
			*v.buf.Cursor() = v.buf.Cursor().MoveTo(v.displayPos(l, r, 0)).VimClamp(v.buf)
		}
		ks.ResetAction()
	}, "H")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			tl, tr := v.topRow()
			l, r := v.stepRows(tl, tr, v.height/2)
			*v.buf.Cursor() = v.buf.Cursor().MoveTo(v.displayPos(l, r, 0)).VimClamp(v.buf)
		}
		ks.ResetAction()
	}, "M")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			b := v.buf
			tl, tr := v.topRow()
			ll, lr := b.NumLines(), v.displayRows(b.NumLines())-1
			var l, r int
			if v.rowsBetween(tl, tr, ll, lr, v.height) <= v.height-1 {
				// The buffer end is on screen: L goes to the last row.
				l, r = ll, lr
			} else {
				l, r = v.stepRows(tl, tr, v.height-1-v.effScrollMargin())
			}
			*b.Cursor() = b.Cursor().MoveTo(v.displayPos(l, r, 0)).VimClamp(b)
		}
		ks.ResetAction()
	}, "L")

	// Ctrl-F/Ctrl-B: full page down/up (by visual rows under softwrap)
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			applyDisplayMotion(ks, true, v.height*ks.Count())
		}
	}, "<C-f>")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			applyDisplayMotion(ks, false, v.height*ks.Count())
		}
	}, "<C-b>")

	// Ctrl-E/Ctrl-Y: scroll the view by visual rows. The cursor stays put
	// until it would leave the scroll margin, then it is pushed along (as
	// in vim), so the next relocate doesn't snap the viewport back.
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			ks.ensureLineVx()
			b := v.buf
			tl, tr := v.topRow()
			tl, tr = v.stepRows(tl, tr, ks.Count())
			v.setTopRow(tl, tr)
			ml, mr := v.stepRows(tl, tr, v.effScrollMargin())
			c := b.Cursor()
			if cl, cr := v.displayRowOf(c.Pos); cl < ml || (cl == ml && cr < mr) {
				*c = c.MoveTo(v.displayPos(ml, mr, c.Vx)).VimClamp(b)
				ks.vertical = true
			}
		}
		ks.ClearCounts()
	}, "<C-e>")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			ks.ensureLineVx()
			b := v.buf
			tl, tr := v.topRow()
			tl, tr = v.stepRows(tl, tr, -ks.Count())
			v.setTopRow(tl, tr)
			ml, mr := v.stepRows(tl, tr, v.height-1-v.effScrollMargin())
			c := b.Cursor()
			if cl, cr := v.displayRowOf(c.Pos); cl > ml || (cl == ml && cr > mr) {
				*c = c.MoveTo(v.displayPos(ml, mr, c.Vx)).VimClamp(b)
				ks.vertical = true
			}
		}
		ks.ClearCounts()
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

	// zz/zt/zb: scroll view to center/top/bottom the cursor's visual row
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			l, r := v.displayRowOf(v.buf.Cursor().Pos)
			v.setTopRow(v.stepRows(l, r, -(v.height / 2)))
		}
		ks.ResetAction()
	}, "z", "z")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			v.setTopRow(v.displayRowOf(v.buf.Cursor().Pos))
		}
		ks.ResetAction()
	}, "z", "t")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := ks.ActiveView(); v != nil {
			l, r := v.displayRowOf(v.buf.Cursor().Pos)
			v.setTopRow(v.stepRows(l, r, -(v.height - 1)))
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
	// (in visual-block mode this swaps to the diagonally opposite corner)
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine, ModeVisualBlock} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			b := ks.Buf()
			for i := range b.cursors {
				c := &b.cursors[i]
				if !c.HasSel {
					continue
				}
				// Swap cursor to the other end. Set Orig to anchor
				// at the current cursor's end so SelectTo on the next
				// motion preserves the full selection.
				if c.Pos <= c.Sel[0] {
					// Cursor at start → move to end.
					anchor := c.Sel[0]
					c.Orig = [2]int{anchor, anchor}
					end := c.Sel[1]
					_, _, sz := b.DecodeGraphemeBefore(end)
					if sz > 0 {
						end -= sz
					}
					c.Pos = end
				} else {
					// Cursor at end → move to start.
					// Anchor at a position on the last selected line.
					anchor := c.Sel[1]
					_, _, sz := b.DecodeGraphemeBefore(anchor)
					if sz > 0 {
						anchor -= sz
					}
					c.Orig = [2]int{anchor, anchor}
					c.Pos = c.Sel[0]
				}
			}
		}, "o")
	}

	// u/U in visual modes: lowercase/uppercase selection
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			execVisualOp(ks, "", func(ks *KeyState, b *Buffer, start, end int) {
				opCase(b, start, end, unicode.ToLower)
			})
		}, "u")
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			execVisualOp(ks, "", func(ks *KeyState, b *Buffer, start, end int) {
				opCase(b, start, end, unicode.ToUpper)
			})
		}, "U")
	}

	// x/s in visual modes: delete/change the selection (vim v_x, v_s).
	// ~ toggles case, and r<char> fills the selection with a character.
	// (Visual-block mode has its own rectangle versions.)
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			execVisualOp(ks, "d", opDelete)
		}, "x")
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			execVisualOp(ks, "c", opChange)
		}, "s")
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			execVisualOp(ks, "", func(ks *KeyState, b *Buffer, start, end int) {
				opCase(b, start, end, toggleRuneCase)
			})
		}, "~")
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			ks.WaitForChar(func(ks *KeyState, ch string) {
				if ch == KeyTab {
					ch = "\t"
				} else if len([]rune(ch)) != 1 {
					ks.ResetAction()
					return
				}
				execVisualReplace(ks, ch)
			})
		}, "r")
	}
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

	if reg.Block {
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			line, _ := b.LineColAt(c.Pos)
			vcol := b.VisualCol(c.Pos)
			if !before {
				// Paste after: insert one cell past the cursor character.
				if r, _, sz := b.DecodeGraphemeAt(c.Pos); sz > 0 && r != '\n' {
					_, e := vcolSpan(b, c.Pos)
					vcol = e + 1
				}
			}
			pos := insertBlockAt(b, line, vcol, reg, count)
			b.cursors[i] = b.cursors[i].MoveTo(pos).VimClamp(b)
		}
		ks.ResetAction()
		return
	}

	content := bytes.Repeat(reg.Content, count)

	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if reg.Linewise {
			// The cursor lands on the first non-blank of the first pasted
			// line, as in vim.
			var pasted int // start of the first pasted line
			if before {
				pasted = c.LineStart(b).Pos
				b.Insert(pasted, content)
			} else {
				pos := c.LineEnd(b).Pos + 1
				if pos > b.Len() {
					// Pasting below the last line of a file with no trailing
					// newline: prepend a newline and drop the content's
					// trailing one to preserve the missing final newline.
					data := append([]byte("\n"), content...)
					if data[len(data)-1] == '\n' {
						data = data[:len(data)-1]
					}
					pasted = b.Len() + 1
					b.Insert(b.Len(), data)
				} else {
					pasted = pos
					b.Insert(pos, content)
				}
			}
			fnb := motionFirstNonBlank(b, Cursor{Pos: pasted}, 0)
			b.cursors[i] = b.cursors[i].MoveTo(fnb).VimClamp(b)
		} else {
			pos := c.Pos
			if !before {
				// Paste after the cursor character, but never past the end
				// of the line (p on an empty line pastes into it, as in vim).
				if r, _, sz := b.DecodeGraphemeAt(pos); sz > 0 && r != '\n' {
					pos += sz
				}
			}
			b.Insert(pos, content)
			// The cursor lands on the last pasted character, as in vim.
			_, _, gsz := b.DecodeGraphemeBefore(pos + len(content))
			b.cursors[i] = b.cursors[i].MoveTo(pos + len(content) - gsz).VimClamp(b)
		}
	}
	ks.ResetAction()
}

// visualPaste replaces the visual selection with the register content.
func visualPaste(ks *KeyState) {
	b := ks.Buf()
	b.UndoBarrier()
	reg := ks.regs.Get(ks.Register())
	if len(reg.Content) == 0 {
		ks.ResetAction()
		return
	}
	content := reg.Content
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if !c.HasSelection() {
			continue
		}
		start, end := c.Sel[0], c.Sel[1]
		b.Remove(start, end)
		b.Insert(start, content)
		// The cursor lands on the last pasted character, as in vim.
		_, _, gsz := b.DecodeGraphemeBefore(start + len(content))
		b.cursors[i] = b.cursors[i].MoveTo(start + len(content) - gsz).VimClamp(b)
	}
	ks.SetMode(ModeNormal)
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
