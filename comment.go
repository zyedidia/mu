package main

import (
	"bytes"
)

// Comment toggling (tcomment-style): gc is an operator that toggles line
// comments over a motion or visual selection, and gcc (or gcgc) toggles the
// current line. The comment prefix per filetype comes from comments.toml.

// opToggleComment is the gc operator function. It always operates on whole
// lines, whichever motion or selection produced the range.
func opToggleComment(ks *KeyState, b *Buffer, start, end int) {
	prefix := ""
	if ks.commentPrefix != nil {
		prefix = ks.commentPrefix(b)
	}
	if prefix == "" {
		return
	}
	toggleComment(b, prefix, start, end, tabSize(ks))
}

// indentWidth returns the visual width of leading whitespace (tab-aware).
func indentWidth(ws []byte, ts int) int {
	w := 0
	for _, c := range ws {
		if c == '\t' {
			w += ts - w%ts
		} else {
			w++
		}
	}
	return w
}

// indentCol returns the byte offset within ws where the visual width
// reaches target, stopping before a tab that would overshoot it.
func indentCol(ws []byte, target, ts int) int {
	w := 0
	for i, c := range ws {
		if w >= target {
			return i
		}
		if c == '\t' {
			next := w + ts - w%ts
			if next > target {
				return i
			}
			w = next
		} else {
			w++
		}
	}
	return len(ws)
}

// toggleComment comments the lines covered by [start, end) with prefix, or
// uncomments them if every non-blank line is already commented. Blank lines
// are left alone. Comments are aligned at the block's lowest indentation
// (tcomment-style): deeper lines keep their extra indent after the prefix.
func toggleComment(b *Buffer, prefix string, start, end, ts int) {
	pre := []byte(prefix)
	sl, _ := b.LineColAt(start)
	el, _ := b.LineColAt(end)
	if el > sl && b.OffsetAt(el, 0) == end {
		el-- // range ends exactly at a line start: don't include that line
	}

	// Uncomment only if every non-blank line in the range is commented.
	uncomment := false
	for l := sl; l <= el; l++ {
		t := bytes.TrimLeft(b.GetLine(l), " \t")
		if len(t) == 0 {
			continue
		}
		uncomment = true
		if !bytes.HasPrefix(t, pre) {
			uncomment = false
			break
		}
	}

	// Apply bottom-up so line offsets stay valid.
	if uncomment {
		for l := el; l >= sl; l-- {
			line := b.GetLine(l)
			ws := leadingWS(line)
			rest := line[len(ws):]
			if !bytes.HasPrefix(rest, pre) {
				continue
			}
			n := len(pre)
			// Also remove the single space the commenter added.
			if len(rest) > n && rest[n] == ' ' {
				n++
			}
			off := b.OffsetAt(l, len(ws))
			b.Remove(off, off+n)
		}
		return
	}

	// Align all comments at the lowest indentation in the block.
	minW := -1
	for l := sl; l <= el; l++ {
		line := b.GetLine(l)
		if len(bytes.TrimLeft(line, " \t")) == 0 {
			continue
		}
		if w := indentWidth(leadingWS(line), ts); minW < 0 || w < minW {
			minW = w
		}
	}

	ins := append(append([]byte{}, pre...), ' ')
	for l := el; l >= sl; l-- {
		line := b.GetLine(l)
		if len(bytes.TrimLeft(line, " \t")) == 0 {
			continue // don't comment blank lines
		}
		col := indentCol(leadingWS(line), minW, ts)
		b.Insert(b.OffsetAt(l, col), ins)
	}
}

// RegisterComments registers the gc comment-toggle operator: gc<motion>,
// gcc / gcgc for lines, and gc on visual selections. The doubled forms come
// from the generic doubled-operator rule in operator.go.
func RegisterComments(ks *KeyState) {
	op := &PendingOp{Name: "gc", Key: "gc", Fn: opToggleComment}

	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.SetPending(op)
	}, "g", "c")

	// gcgc: the full doubled form ("c" alone is handled by the generic
	// doubled-operator binding registered for the c operator).
	ks.modes[ModeOperatorPending].Bindings.Bind(func(ks *KeyState) {
		execDoubledOp(ks, "gc")
	}, "g", "c")

	for _, mode := range []ModeID{ModeVisual, ModeVisualLine, ModeVisualBlock} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			execVisualOp(ks, "gc", opToggleComment)
		}, "g", "c")
	}
}
