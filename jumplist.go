package main

import (
	"fmt"
	"strings"
)

// Jump list (vim <C-o>/<C-i>): positions recorded before "jump" commands
// (G, gg, {, }, %, H/M/L, searches, mark jumps, :<line>, gd, buffer
// switches), navigable backward and forward. Entries reference buffers, so
// jumping back can cross files.

// jumpPos is one jump list entry. Line/column are stored (not byte
// offsets) and clamped on use, so entries survive edits, vim-style.
type jumpPos struct {
	buf       *Buffer
	line, col int
}

// JumpList holds the entries and the navigation index; idx equal to
// len(entries) means "at the live position" (past the newest entry).
type JumpList struct {
	entries []jumpPos
	idx     int
}

const maxJumps = 100

// removeLine drops entries for the given buffer line (vim keeps at most
// one entry per line).
func (j *JumpList) removeLine(b *Buffer, line int) {
	for i := 0; i < len(j.entries); i++ {
		if j.entries[i].buf == b && j.entries[i].line == line {
			j.entries = append(j.entries[:i], j.entries[i+1:]...)
			if i < j.idx {
				j.idx--
			}
			i--
		}
	}
}

// push records a position (taken before a jump lands) and resets the index
// to the live end.
func (j *JumpList) push(p jumpPos) {
	j.removeLine(p.buf, p.line)
	j.entries = append(j.entries, p)
	if len(j.entries) > maxJumps {
		j.entries = j.entries[1:]
	}
	j.idx = len(j.entries)
}

// back moves one entry backward, storing the current position first so
// forward can return to it.
func (j *JumpList) back(cur jumpPos) (jumpPos, bool) {
	if j.idx == 0 {
		return jumpPos{}, false
	}
	if j.idx >= len(j.entries) {
		// Remember the live position as the newest entry.
		j.removeLine(cur.buf, cur.line)
		j.entries = append(j.entries, cur)
		j.idx = len(j.entries) - 1
		if j.idx == 0 {
			return jumpPos{}, false
		}
	}
	j.idx--
	return j.entries[j.idx], true
}

// forward moves one entry forward.
func (j *JumpList) forward() (jumpPos, bool) {
	if j.idx >= len(j.entries)-1 {
		return jumpPos{}, false
	}
	j.idx++
	return j.entries[j.idx], true
}

// prune drops entries whose buffer is no longer alive.
func (j *JumpList) prune(alive func(*Buffer) bool) {
	idx := j.idx
	var out []jumpPos
	for i, p := range j.entries {
		if alive(p.buf) {
			out = append(out, p)
		} else if i < j.idx {
			idx--
		}
	}
	j.entries = out
	if idx > len(out) {
		idx = len(out)
	}
	j.idx = idx
}

// --- Editor integration ---

// curJumpPos returns the current position as a jump list entry.
func (e *Editor) curJumpPos() (jumpPos, bool) {
	v := e.ActiveView()
	if v == nil {
		return jumpPos{}, false
	}
	line, col := v.buf.LineColAt(v.buf.Cursor().Pos)
	return jumpPos{buf: v.buf, line: line, col: col}, true
}

// pushJump records the current position; call before a jump moves away.
func (e *Editor) pushJump() {
	if p, ok := e.curJumpPos(); ok {
		e.jumps.push(p)
	}
}

// pushJumpAt records an explicit position in a buffer (used when the
// cursor has already moved, e.g. accepting an incremental search).
func (e *Editor) pushJumpAt(b *Buffer, pos int) {
	line, col := b.LineColAt(pos)
	e.jumps.push(jumpPos{buf: b, line: line, col: col})
}

// bufferListed reports whether b is in the buffer list.
func (e *Editor) bufferListed(b *Buffer) bool {
	for _, ob := range e.buffers {
		if ob == b {
			return true
		}
	}
	return false
}

// gotoJump moves to a jump entry, switching buffers if needed and clamping
// the stored line/column to the buffer's current contents.
func (e *Editor) gotoJump(p jumpPos) {
	if v := e.ActiveView(); v == nil || v.buf != p.buf {
		e.showBuffer(p.buf)
	}
	b := p.buf
	line := p.line
	if line > b.LastLine() {
		line = b.LastLine()
	}
	col := p.col
	if ll := b.LineLen(line); col > ll {
		col = ll
	}
	*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(line, col)).VimClamp(b)
}

// jumpBack moves to the previous jump list entry (<C-o>).
func (e *Editor) jumpBack() {
	e.jumps.prune(e.bufferListed)
	cur, ok := e.curJumpPos()
	if !ok {
		return
	}
	if p, ok := e.jumps.back(cur); ok {
		e.gotoJump(p)
	}
}

// jumpForward moves to the next jump list entry (<C-i>/<Tab>).
func (e *Editor) jumpForward() {
	e.jumps.prune(e.bufferListed)
	if p, ok := e.jumps.forward(); ok {
		e.gotoJump(p)
	}
}

// cmdJumps lists the jump list (vim :jumps), oldest first, with ">"
// marking the current index.
func cmdJumps(e *Editor, args []string) error {
	if len(e.jumps.entries) == 0 {
		e.infobar.Message("no jumps")
		return nil
	}
	var parts []string
	for i, p := range e.jumps.entries {
		mark := ""
		if i == e.jumps.idx {
			mark = ">"
		}
		parts = append(parts, fmt.Sprintf("%s%d:%d %s", mark, p.line+1, p.col, bufDisplayName(p.buf)))
	}
	if e.jumps.idx >= len(e.jumps.entries) {
		parts = append(parts, ">")
	}
	e.infobar.Message(strings.Join(parts, "  "))
	return nil
}
