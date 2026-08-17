package main

import (
	"bytes"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Text formatting (vim's gq): reflow lines to the textwidth option, joining
// and splitting them so no line exceeds the wrap column. Blocks separated by
// blank lines are formatted independently, indentation follows each block's
// first line, and comment blocks keep their comment leader on every wrapped
// line (using the same comments.toml prefixes as gc).

// textWidth returns the wrap column from the textwidth option (vim-style
// fallback of 79 when unset or non-positive).
func textWidth(ks *KeyState) int {
	if v := ks.ActiveView(); v != nil && v.Opts != nil {
		if n, ok := GetOptInt(v.Opts, "textwidth"); ok && n > 0 {
			return n
		}
	}
	return 79
}

// opFormat is the gq operator function: reflow the lines covered by the
// range. The cursor is left on the first non-blank of the last formatted
// line, as in vim.
func opFormat(ks *KeyState, b *Buffer, start, end int) {
	width := textWidth(ks)
	ts := tabSize(ks)
	prefix := ""
	if ks.commentPrefix != nil {
		prefix = ks.commentPrefix(b)
	}

	sl, _ := b.LineColAt(start)
	el, _ := b.LineColAt(end)
	if el > sl && b.OffsetAt(el, 0) == end {
		el-- // range ends exactly at a line start: don't include that line
	}

	last := formatLines(b, sl, el, width, ts, prefix)
	pos := motionFirstNonBlank(b, Cursor{Pos: b.OffsetAt(last, 0)}, 0)
	*b.Cursor() = b.Cursor().MoveTo(pos).VimClamp(b)
}

// formatBlock is a run of adjacent non-blank lines that reflows as one
// paragraph.
type formatBlock struct {
	first, last int
	comment     bool
}

// formatLines reflows the blocks within [sl, el] and returns the line
// number of the last formatted line after the edits.
func formatLines(b *Buffer, sl, el, width, ts int, prefix string) int {
	pre := []byte(prefix)
	isBlank := func(l int) bool {
		return len(bytes.TrimLeft(b.GetLine(l), " \t")) == 0
	}
	isComment := func(l int) bool {
		if len(pre) == 0 {
			return false
		}
		return bytes.HasPrefix(bytes.TrimLeft(b.GetLine(l), " \t"), pre)
	}

	// Group the range into blocks: blank lines separate them, and comment
	// lines never merge with code lines.
	var blocks []formatBlock
	for l := sl; l <= el; l++ {
		if isBlank(l) {
			continue
		}
		c := isComment(l)
		if n := len(blocks); n > 0 && blocks[n-1].last == l-1 && blocks[n-1].comment == c {
			blocks[n-1].last = l
		} else {
			blocks = append(blocks, formatBlock{first: l, last: l, comment: c})
		}
	}
	if len(blocks) == 0 {
		return el
	}

	// Reflow bottom-up so earlier line numbers stay valid; the returned
	// last-line number is then adjusted by the size changes of the blocks
	// above it.
	last := -1
	for i := len(blocks) - 1; i >= 0; i-- {
		blk := blocks[i]
		n := reflowBlock(b, blk, width, ts, pre)
		if last < 0 {
			last = blk.first + n - 1
		} else {
			last += n - (blk.last - blk.first + 1)
		}
	}
	return last
}

// reflowBlock rewrites one block as greedily filled lines and returns the
// new number of lines. All output lines take the leader (indentation plus
// comment prefix, if any) of the block's first line.
func reflowBlock(b *Buffer, blk formatBlock, width, ts int, pre []byte) int {
	// Leader from the first line.
	first := b.GetLine(blk.first)
	leader := append([]byte{}, leadingWS(first)...)
	if blk.comment {
		leader = append(leader, pre...)
		leader = append(leader, ' ')
	}
	leaderW := indentWidth(leadingWS(first), ts)
	if blk.comment {
		leaderW += runewidth.StringWidth(string(pre)) + 1
	}

	// Collect the words, stripping each line's own leader.
	var words []string
	for l := blk.first; l <= blk.last; l++ {
		line := bytes.TrimLeft(b.GetLine(l), " \t")
		if blk.comment {
			line = bytes.TrimPrefix(line, pre)
			if len(line) > 0 && line[0] == ' ' {
				line = line[1:]
			}
		}
		words = append(words, strings.Fields(string(line))...)
	}

	// Greedy fill: words never split, so a single overlong word may still
	// exceed the width (as in vim).
	var out [][]byte
	if len(words) == 0 {
		out = append(out, bytes.TrimRight(leader, " "))
	} else {
		cur := words[0]
		curW := leaderW + runewidth.StringWidth(words[0])
		for _, w := range words[1:] {
			ww := runewidth.StringWidth(w)
			if curW+1+ww <= width {
				cur += " " + w
				curW += 1 + ww
			} else {
				out = append(out, append(append([]byte{}, leader...), cur...))
				cur = w
				curW = leaderW + ww
			}
		}
		out = append(out, append(append([]byte{}, leader...), cur...))
	}

	// Replace the block's lines. A final line without a newline (EOF)
	// stays that way.
	start := b.OffsetAt(blk.first, 0)
	end := b.OffsetAt(blk.last+1, 0)
	if end > b.Len() {
		end = b.Len()
	}
	trailingNL := end > start && b.ByteAt(end-1) == '\n'
	text := bytes.Join(out, []byte("\n"))
	if trailingNL {
		text = append(text, '\n')
	}
	b.Remove(start, end)
	b.Insert(start, text)
	return len(out)
}

// RegisterFormat registers the gq formatting operator: gq<motion>, gqq /
// gqgq for lines, and gq on visual selections.
func RegisterFormat(ks *KeyState) {
	op := &PendingOp{Name: "gq", Key: "gq", Fn: opFormat}

	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.SetPending(op)
	}, "g", "q")

	// gqq and gqgq (via the generic doubled-operator rule).
	ks.modes[ModeOperatorPending].Bindings.Bind(func(ks *KeyState) {
		execDoubledOp(ks, "q")
	}, "q")
	ks.modes[ModeOperatorPending].Bindings.Bind(func(ks *KeyState) {
		execDoubledOp(ks, "gq")
	}, "g", "q")

	for _, mode := range []ModeID{ModeVisual, ModeVisualLine, ModeVisualBlock} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			execVisualOp(ks, "gq", opFormat)
		}, "g", "q")
	}
}
