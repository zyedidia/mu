package main

import (
	"bytes"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

// Visual-block mode (vim's Ctrl-V): the selection is a rectangle of visual
// columns between the anchor (Cursor.Orig[0]) and the cursor position. The
// rectangle is recomputed on demand from the two corner byte offsets, so
// edits that shift offsets keep it consistent.

// blockRect is the rectangle described by a block-mode selection: an
// inclusive range of buffer lines and an inclusive range of visual columns.
type blockRect struct {
	top, bot    int  // buffer lines, top <= bot
	left, right int  // visual columns, left <= right
	toEOL       bool // $-block: right edge extends to each line's end
}

// blockRectFor computes the rectangle between a cursor's anchor and position.
// The corner characters are included, so a corner on a tab or a wide
// character extends the rectangle over all cells that character occupies.
func blockRectFor(b *Buffer, c Cursor) blockRect {
	aLine, _ := b.LineColAt(c.Orig[0])
	cLine, _ := b.LineColAt(c.Pos)
	as, ae := vcolSpan(b, c.Orig[0])
	cs, ce := vcolSpan(b, c.Pos)
	return blockRect{
		top:   min(aLine, cLine),
		bot:   max(aLine, cLine),
		left:  min(as, cs),
		right: max(ae, ce),
		toEOL: c.BlockEOL,
	}
}

// lineByteSpace reports whether visual-column math on this line falls back
// to byte columns (mirroring VisualCol/VisualLoc's fallback).
func lineByteSpace(b *Buffer, line int) bool {
	return b.vis == nil || b.LineLen(line) > maxVisualWalk
}

// cellWidth returns the number of display cells the grapheme at a position
// spans when it starts at visual column vx (byte size in byte-column
// fallback mode).
func cellWidth(b *Buffer, r rune, sz, gwidth, vx int, byteSpace bool) int {
	if byteSpace {
		return sz
	}
	w := b.vis.Size(r, vx, gwidth)
	if w < 1 {
		w = 1
	}
	return w
}

// vcolSpan returns the inclusive visual-column span [start, end] of the
// grapheme at off. For '\n' or EOF the span is a single column.
func vcolSpan(b *Buffer, off int) (int, int) {
	start := b.VisualCol(off)
	r, _, sz, gwidth := b.DecodeGraphemeWidthAt(off)
	if sz == 0 || r == '\n' {
		return start, start
	}
	line, _ := b.LineColAt(off)
	w := cellWidth(b, r, sz, gwidth, start, lineByteSpace(b, line))
	return start, start + w - 1
}

// blockSpan describes how a block rectangle intersects a single line.
type blockSpan struct {
	start, end int // byte range of covered graphemes [start, end)
	// padLeft/padRight are the uncovered cells of partially covered edge
	// characters (a tab or wide char straddling the block edge). Deleting
	// the range reinserts them as spaces to preserve the layout.
	padLeft, padRight int
	text              []byte // covered text; partially covered chars become spaces
	width             int    // covered width in cells
}

// blockLineSpan computes the intersection of a block rectangle with one line.
// An empty intersection yields start == end (at the end of the line).
func blockLineSpan(b *Buffer, line int, r blockRect) blockSpan {
	off := b.OffsetAt(line, 0)
	byteSpace := lineByteSpace(b, line)
	sp := blockSpan{start: -1}
	vx := 0
	for {
		ch, _, sz, gwidth := b.DecodeGraphemeWidthAt(off)
		if sz == 0 || ch == '\n' {
			break
		}
		w := cellWidth(b, ch, sz, gwidth, vx, byteSpace)
		s, e := vx, vx+w-1
		if !r.toEOL && s > r.right {
			break
		}
		if e >= r.left {
			cs, ce := max(s, r.left), e
			if !r.toEOL {
				ce = min(e, r.right)
			}
			if sp.start < 0 {
				sp.start = off
				sp.padLeft = max(0, r.left-s)
			}
			sp.end = off + sz
			sp.padRight = 0
			if !r.toEOL && e > r.right {
				sp.padRight = e - r.right
			}
			if cs == s && ce == e {
				sp.text = append(sp.text, b.Slice(off, off+sz)...)
			} else {
				sp.text = append(sp.text, spaces(ce-cs+1)...)
			}
			sp.width += ce - cs + 1
		}
		off += sz
		vx += w
	}
	if sp.start < 0 {
		sp.start, sp.end = off, off
	}
	return sp
}

// adjustVisualBlockSel recomputes Sel as the byte span between the two block
// corners (inclusive of the later corner's grapheme).
func adjustVisualBlockSel(b *Buffer, c *Cursor) {
	if !c.HasSel {
		return
	}
	lo := min(c.Orig[0], c.Pos)
	hi := max(c.Orig[0], c.Pos)
	_, _, sz := b.DecodeGraphemeAt(hi)
	c.Sel[0] = lo
	c.Sel[1] = hi + sz
}

func spaces(n int) []byte {
	if n <= 0 {
		return nil
	}
	return bytes.Repeat([]byte{' '}, n)
}

// setBlockRegister writes blockwise rows to the default (and selected)
// register.
func setBlockRegister(ks *KeyState, texts [][]byte, width int, isYank bool) {
	content := bytes.Join(texts, []byte("\n"))
	ks.regs.SetDefaultBlock(content, width, isYank)
	if r := ks.Register(); r != RegDefault {
		ks.regs.SetBlock(r, content, width)
	}
}

// blockRegWidth returns the register width for a rectangle: its column span,
// or the widest covered row for a ragged $-block.
func blockRegWidth(r blockRect, spans []blockSpan) int {
	if !r.toEOL {
		return r.right - r.left + 1
	}
	w := 0
	for _, sp := range spans {
		if sp.width > w {
			w = sp.width
		}
	}
	return w
}

// blockDeleteRect deletes the rectangle's text line by line (top-down,
// recomputing each line's span after earlier edits), optionally writing the
// deleted rows to the register. Partially covered edge characters are
// replaced by spaces for their uncovered cells. It returns the byte position
// of the block's top-left corner after the edit and the per-line positions
// where text was removed.
func blockDeleteRect(ks *KeyState, b *Buffer, r blockRect, writeReg bool) (int, []int) {
	texts := make([][]byte, 0, r.bot-r.top+1)
	spans := make([]blockSpan, 0, r.bot-r.top+1)
	var edited []int
	pos := -1
	for line := r.top; line <= r.bot; line++ {
		sp := blockLineSpan(b, line, r)
		if pos < 0 {
			pos = sp.start + sp.padLeft
		}
		texts = append(texts, sp.text)
		spans = append(spans, sp)
		if sp.start == sp.end {
			continue
		}
		b.Remove(sp.start, sp.end)
		if pad := sp.padLeft + sp.padRight; pad > 0 {
			b.Insert(sp.start, spaces(pad))
		}
		edited = append(edited, sp.start+sp.padLeft)
	}
	if writeReg {
		setBlockRegister(ks, texts, blockRegWidth(r, spans), false)
	}
	return pos, edited
}

// execBlockDelete implements d/x (and D with toEOL) in visual-block mode.
func execBlockDelete(ks *KeyState, forceEOL bool) {
	b := ks.Buf()
	b.UndoBarrier()
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if !c.HasSelection() {
			continue
		}
		r := blockRectFor(b, c)
		if forceEOL {
			r.toEOL = true
		}
		pos, _ := blockDeleteRect(ks, b, r, true)
		b.cursors[i] = b.cursors[i].MoveTo(pos).VimClamp(b)
	}
	ks.SetMode(ModeNormal)
	ks.ResetAction()
}

// execBlockYank implements y in visual-block mode.
func execBlockYank(ks *KeyState) {
	b := ks.Buf()
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if !c.HasSelection() {
			continue
		}
		r := blockRectFor(b, c)
		texts := make([][]byte, 0, r.bot-r.top+1)
		spans := make([]blockSpan, 0, r.bot-r.top+1)
		for line := r.top; line <= r.bot; line++ {
			sp := blockLineSpan(b, line, r)
			texts = append(texts, sp.text)
			spans = append(spans, sp)
		}
		setBlockRegister(ks, texts, blockRegWidth(r, spans), true)
		b.cursors[i] = b.cursors[i].MoveTo(b.VisualLoc(r.top, r.left)).VimClamp(b)
	}
	ks.SetMode(ModeNormal)
	ks.ResetAction()
}

// startBlockInsert rebuilds the cursor list with one cursor per position and
// enters insert mode; typed text appears at every position and the cursors
// collapse back to the first when insert mode ends.
func startBlockInsert(ks *KeyState, inserts []int) {
	b := ks.Buf()
	if len(inserts) == 0 {
		ks.SetMode(ModeNormal)
		ks.ResetAction()
		return
	}
	b.RemoveCursors()
	*b.Cursor() = b.Cursor().MoveTo(inserts[0])
	for _, p := range inserts[1:] {
		b.SpawnCursor(p)
	}
	b.SwitchCursor(0)
	ks.blockInsert = true
	ks.SetMode(ModeInsert)
	ks.ResetAction()
}

// execBlockChange implements c/s (and C with toEOL): delete the rectangle,
// then insert on every line that reached into it.
func execBlockChange(ks *KeyState, forceEOL bool) {
	b := ks.Buf()
	b.UndoBarrier()
	var inserts []int
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if !c.HasSelection() {
			continue
		}
		r := blockRectFor(b, c)
		if forceEOL {
			r.toEOL = true
		}
		pos, edited := blockDeleteRect(ks, b, r, true)
		if len(edited) == 0 {
			edited = []int{pos}
		}
		inserts = append(inserts, edited...)
	}
	startBlockInsert(ks, inserts)
}

// execBlockInsert implements I (insert at the block's left edge) and A
// (append after its right edge, or at each line's end for a $-block).
func execBlockInsert(ks *KeyState, appendSide bool) {
	b := ks.Buf()
	b.UndoBarrier()
	var inserts []int
	for ci := 0; ci < b.NumCursors(); ci++ {
		c := b.cursors[ci]
		if !c.HasSelection() {
			continue
		}
		r := blockRectFor(b, c)
		n := 0
		for line := r.top; line <= r.bot; line++ {
			lineEnd := b.OffsetAt(line, 0) + b.LineLen(line)
			lw := b.VisualCol(lineEnd)
			var pos, pad int
			if appendSide {
				if r.toEOL {
					pos = lineEnd
				} else if lw <= r.right {
					// Line ends inside the block: pad to the append column.
					pos, pad = lineEnd, r.right+1-lw
				} else {
					pos = b.VisualLoc(line, r.right+1)
				}
			} else {
				if lw < r.left {
					continue // vim skips lines that don't reach the block
				}
				pos = b.VisualLoc(line, r.left)
			}
			if pad > 0 {
				b.Insert(pos, spaces(pad))
			}
			inserts = append(inserts, pos+pad)
			n++
		}
		if n == 0 {
			inserts = append(inserts, b.VisualLoc(r.top, r.left))
		}
	}
	startBlockInsert(ks, inserts)
}

// execBlockReplace implements r<char>: fill the rectangle with the character,
// preserving the layout of partially covered tabs and wide characters.
func execBlockReplace(ks *KeyState, ch string) {
	b := ks.Buf()
	b.UndoBarrier()
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if !c.HasSelection() {
			continue
		}
		r := blockRectFor(b, c)
		pos := -1
		for line := r.top; line <= r.bot; line++ {
			sp := blockLineSpan(b, line, r)
			if pos < 0 {
				pos = sp.start + sp.padLeft
			}
			if sp.start == sp.end {
				continue
			}
			b.Remove(sp.start, sp.end)
			repl := make([]byte, 0, sp.padLeft+sp.padRight+sp.width*len(ch))
			repl = append(repl, spaces(sp.padLeft)...)
			repl = append(repl, strings.Repeat(ch, sp.width)...)
			repl = append(repl, spaces(sp.padRight)...)
			b.Insert(sp.start, repl)
		}
		b.cursors[i] = b.cursors[i].MoveTo(pos).VimClamp(b)
	}
	ks.SetMode(ModeNormal)
	ks.ResetAction()
}

// execBlockCase applies a case transform to the rectangle's text.
func execBlockCase(ks *KeyState, transform func(rune) rune) {
	b := ks.Buf()
	b.UndoBarrier()
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if !c.HasSelection() {
			continue
		}
		r := blockRectFor(b, c)
		pos := -1
		for line := r.top; line <= r.bot; line++ {
			sp := blockLineSpan(b, line, r)
			if pos < 0 {
				pos = sp.start
			}
			if sp.start == sp.end {
				continue
			}
			opCase(b, sp.start, sp.end, transform)
		}
		b.cursors[i] = b.cursors[i].MoveTo(pos).VimClamp(b)
	}
	ks.SetMode(ModeNormal)
	ks.ResetAction()
}

func toggleRuneCase(r rune) rune {
	if unicode.IsUpper(r) {
		return unicode.ToLower(r)
	}
	return unicode.ToUpper(r)
}

// repeatBlockRow repeats a block row count times horizontally, padding
// intermediate copies with spaces to the block width.
func repeatBlockRow(row []byte, width, count int) []byte {
	if count <= 1 || len(row) == 0 {
		return row
	}
	padded := append([]byte(nil), row...)
	if pad := width - runewidth.StringWidth(string(row)); pad > 0 {
		padded = append(padded, spaces(pad)...)
	}
	out := bytes.Repeat(padded, count-1)
	return append(out, row...)
}

// insertBlockAt inserts blockwise register content as a rectangle whose
// top-left corner is at (line, vcol). Rows land on successive lines, creating
// new lines past EOF and padding short lines with spaces. Returns the byte
// position of the start of the first inserted row.
func insertBlockAt(b *Buffer, line, vcol int, reg Register, count int) int {
	rows := bytes.Split(reg.Content, []byte("\n"))
	first := -1
	for j, row := range rows {
		text := repeatBlockRow(row, reg.BlockWidth, count)
		tl := line + j
		for tl > b.LastLine() {
			b.Insert(b.Len(), []byte("\n"))
		}
		off := b.VisualLoc(tl, vcol)
		if len(text) > 0 {
			// Empty rows leave the target line untouched (no padding).
			if pad := vcol - b.VisualCol(off); pad > 0 {
				b.Insert(off, spaces(pad))
				off += pad
			}
			b.Insert(off, text)
		}
		if first < 0 {
			first = off
		}
	}
	if first < 0 {
		first = b.VisualLoc(line, vcol)
	}
	return first
}

// blockVisualPaste implements p/P in visual-block mode: delete the rectangle
// (into the unnamed register), then paste the previous register content at
// the block's top-left corner.
func blockVisualPaste(ks *KeyState) {
	b := ks.Buf()
	b.UndoBarrier()
	reg := ks.regs.Get(ks.Register())
	if len(reg.Content) == 0 {
		ks.ResetAction()
		return
	}
	// Copy: the block delete below overwrites the unnamed register.
	reg.Content = append([]byte(nil), reg.Content...)
	for i := 0; i < b.NumCursors(); i++ {
		c := b.cursors[i]
		if !c.HasSelection() {
			continue
		}
		r := blockRectFor(b, c)
		pos, _ := blockDeleteRect(ks, b, r, true)
		switch {
		case reg.Block:
			line, _ := b.LineColAt(pos)
			pos = insertBlockAt(b, line, r.left, reg, 1)
		case reg.Linewise:
			// Paste the lines below the block's top line.
			line, _ := b.LineColAt(pos)
			at := b.OffsetAt(line+1, 0)
			if at > b.Len() {
				at = b.Len()
			}
			if at == b.Len() && b.Len() > 0 && b.ByteAt(b.Len()-1) != '\n' {
				data := append([]byte("\n"), reg.Content...)
				if data[len(data)-1] == '\n' {
					data = data[:len(data)-1]
				}
				b.Insert(at, data)
				pos = at + 1
			} else {
				b.Insert(at, reg.Content)
				pos = at
			}
			pos = motionFirstNonBlank(b, Cursor{Pos: pos}, 0)
		default:
			b.Insert(pos, reg.Content)
			// Cursor on the last pasted character, as in vim.
			_, _, gsz := b.DecodeGraphemeBefore(pos + len(reg.Content))
			pos = pos + len(reg.Content) - gsz
		}
		b.cursors[i] = b.cursors[i].MoveTo(pos).VimClamp(b)
	}
	ks.SetMode(ModeNormal)
	ks.ResetAction()
}

// RegisterVisualBlock registers visual-block mode bindings. Motions and text
// objects are bound by RegisterMotions/RegisterTextObjects; escape, <C-c>,
// and the o corner swap are bound by RegisterOperators alongside the other
// visual modes.
func RegisterVisualBlock(ks *KeyState) {
	// <C-v> in normal mode: enter visual-block mode.
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		c := b.Cursor()
		_, _, sz := b.DecodeGraphemeAt(c.Pos)
		c.HasSel = true
		c.Orig = [2]int{c.Pos, c.Pos + sz}
		c.Sel = [2]int{c.Pos, c.Pos + sz}
		c.BlockSel = true
		c.BlockEOL = false
		ks.SetMode(ModeVisualBlock)
	}, "<C-v>")

	// <C-v> in visual/visual-line mode: switch to block, keeping the corners.
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			b := ks.Buf()
			for i := range b.cursors {
				c := &b.cursors[i]
				c.BlockSel = true
				c.BlockEOL = false
				adjustVisualBlockSel(b, c)
			}
			ks.mode = ModeVisualBlock
			if ks.onModeChange != nil {
				ks.onModeChange(ModeVisualBlock)
			}
		}, "<C-v>")
	}

	// <C-v> in block mode: back to normal.
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := range b.cursors {
			b.cursors[i].ClearSelection()
		}
		ks.SetMode(ModeNormal)
		ks.ResetAction()
	}, "<C-v>")

	// v in block mode: switch to charwise visual.
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := range b.cursors {
			c := &b.cursors[i]
			c.BlockSel = false
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

	// V in block mode: switch to linewise visual.
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := range b.cursors {
			c := &b.cursors[i]
			c.BlockSel = false
			adjustVisualLine(b, c)
		}
		ks.mode = ModeVisualLine
		if ks.onModeChange != nil {
			ks.onModeChange(ModeVisualLine)
		}
	}, "V")

	// O: swap the cursor to the other horizontal corner on the same line.
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		b := ks.Buf()
		for i := range b.cursors {
			c := &b.cursors[i]
			if !c.HasSel {
				continue
			}
			aLine, _ := b.LineColAt(c.Orig[0])
			cLine, _ := b.LineColAt(c.Pos)
			av := b.VisualCol(c.Orig[0])
			cv := b.VisualCol(c.Pos)
			newPos := (Cursor{Pos: b.VisualLoc(cLine, av)}).VimClamp(b).Pos
			newAnchor := (Cursor{Pos: b.VisualLoc(aLine, cv)}).VimClamp(b).Pos
			c.Pos = newPos
			_, _, asz := b.DecodeGraphemeAt(newAnchor)
			c.Orig = [2]int{newAnchor, newAnchor + asz}
			adjustVisualBlockSel(b, c)
		}
	}, "O")

	// Operators.
	for _, key := range []string{"d", "x"} {
		ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
			execBlockDelete(ks, false)
		}, key)
	}
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		execBlockDelete(ks, true)
	}, "D")

	ks.modes[ModeVisualBlock].Bindings.Bind(execBlockYank, "y")

	for _, key := range []string{"c", "s"} {
		ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
			execBlockChange(ks, false)
		}, key)
	}
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		execBlockChange(ks, true)
	}, "C")

	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		execBlockInsert(ks, false)
	}, "I")
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		execBlockInsert(ks, true)
	}, "A")

	// r<char>: fill the block with a character.
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		ks.WaitForChar(func(ks *KeyState, ch string) {
			if ch == KeyTab {
				ch = "\t"
			} else if len([]rune(ch)) != 1 {
				ks.ResetAction()
				return
			}
			execBlockReplace(ks, ch)
		})
	}, "r")

	// u/U/~: case operations on the block.
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		execBlockCase(ks, unicode.ToLower)
	}, "u")
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		execBlockCase(ks, unicode.ToUpper)
	}, "U")
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		execBlockCase(ks, toggleRuneCase)
	}, "~")

	// p/P: replace the block with register content.
	for _, key := range []string{"p", "P"} {
		ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
			blockVisualPaste(ks)
		}, key)
	}

	// >/<: indent the block's lines (the corner byte span covers exactly
	// the block's line range, so the shared visual operator applies).
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		execVisualOp(ks, ">", opIndent)
	}, ">")
	ks.modes[ModeVisualBlock].Bindings.Bind(func(ks *KeyState) {
		execVisualOp(ks, "<", opDedent)
	}, "<")
}
