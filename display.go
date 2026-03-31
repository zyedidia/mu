package main

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// A Visualizer specifies how runes should be displayed. It handles tab
// expansion, whitespace visualization, and non-printable character rendering.
type Visualizer struct {
	TabSize int
	CharMap map[rune]string // mapped strings must be single runes
}

// String returns the display string and style for a rune at visual column vx.
func (v *Visualizer) String(r rune, vx int, th *Theme) (string, Style) {
	if r == '\n' {
		// Newline visualization (e.g. "$") if configured, otherwise invisible.
		if char, ok := v.CharMap[r]; ok {
			return char, th.Style("hidden-char")
		}
		return "", th.Default()
	}
	if r == '\t' {
		tsz := v.TabSize
		if char, ok := v.CharMap[r]; ok {
			return char + strings.Repeat(" ", tsz-(vx%tsz)-1), th.Style("hidden-char")
		}
		return strings.Repeat(" ", tsz-(vx%tsz)), th.Default()
	}
	if s, ok := v.CharMap[r]; ok {
		return s, th.Style("hidden-char")
	}
	if unicode.IsPrint(r) {
		return string(r), th.Default()
	}
	return fmt.Sprintf("<%02x>", r), th.Default()
}

// Size returns the display width of a rune at visual column vx. The width
// parameter is the grapheme width from the unicode segmenter.
func (v *Visualizer) Size(r rune, vx int, width int) int {
	if r == '\t' {
		return v.TabSize - (vx % v.TabSize)
	}
	if s, ok := v.CharMap[r]; ok {
		c, _ := utf8.DecodeRuneInString(s)
		return runewidth.RuneWidth(c)
	}
	if unicode.IsPrint(r) {
		return width
	}
	return 4 // <xx>
}

// Special returns true if the rune needs special rendering (charmap or
// non-printable).
func (v *Visualizer) Special(r rune) bool {
	if _, ok := v.CharMap[r]; ok {
		return true
	}
	return !unicode.IsPrint(r)
}

// RenderTracker provides callbacks for the rendering loop. Draw is called for
// each character to be displayed. Track is called for each byte offset to map
// between buffer and visual positions; returning true aborts rendering.
type RenderTracker struct {
	Draw  func(bx, by, vx, vy int, mainc rune, combc []rune, style Style)
	Track func(off, bx, by, vx, vy int) bool
}

// RenderForward renders the buffer starting from byte offset 'off' into a box
// of the given width and height. The vis parameter controls tab/special char
// rendering, and th provides syntax highlighting styles.
func (b *Buffer) RenderForward(tracker RenderTracker, vis *Visualizer, width, height, off int, softwrap, wordwrap bool, th *Theme) {
	var vx, x, y int

	// Compute syntax matches for the visible range.
	if tracker.Draw != nil && th != nil {
		endLine, _ := b.LineColAt(off)
		end := b.OffsetAt(endLine+height+1, 0)
		if end > b.Len() {
			end = b.Len()
		}
		b.HighlightRange(off, end)
	}

	newline := func() bool {
		x = 0
		y++
		return false
	}

	drawRune := func(off int, c rune, combc []rune, rwidth, bx, by int, style Style) (done bool) {
		if tracker.Draw != nil {
			// Syntax highlighting: override default style with syntax group.
			if th != nil && style == th.Default() {
				if group := b.SyntaxGroup(off); group != "" {
					style = th.Style(group)
				}
			}
			// Selection overlay.
			for _, cur := range b.cursors {
				if cur.HasSelection() && off >= cur.Sel[0] && off < cur.Sel[1] {
					if th.HasStyle("selection") {
						style.Bg = th.Style("selection").Bg
					} else {
						style = th.Default().Add(AttrReverse)
					}
				}
			}
			tracker.Draw(bx, by, x, y, c, combc, style)
		}
		x += rwidth
		vx += rwidth

		if x >= width && softwrap {
			return newline()
		}
		return false
	}

	for {
		blen := b.Len()
		if off >= blen {
			by, bx := 0, 0
			if blen > 0 {
				by, bx = b.LineColAt(blen - 1)
				bx++
			}
			if tracker.Track != nil && tracker.Track(blen, bx, by, x, y) {
				return
			}
			break
		}

		r, combc, size, gwidth := b.DecodeGraphemeWidthAt(off)
		by, bx := b.LineColAt(off)
		if tracker.Track != nil && tracker.Track(off, bx, by, x, y) {
			return
		}

		if r == '\n' {
			str, style := vis.String(r, x, th)
			for _, c := range str {
				drawRune(off, c, nil, runewidth.RuneWidth(c), bx, by, style)
			}
			// If the newline is selected, highlight one cell past end of line.
			if tracker.Draw != nil {
				hasSel := false
				for _, cur := range b.cursors {
					if cur.HasSelection() && off >= cur.Sel[0] && off < cur.Sel[1] {
						hasSel = true
						break
					}
				}
				if x < width && hasSel {
					selStyle := th.Default()
					if th.HasStyle("selection") {
						selStyle.Bg = th.Style("selection").Bg
					} else {
						selStyle = selStyle.Add(AttrReverse)
					}
					tracker.Draw(bx, by, x, y, ' ', nil, selStyle)
					x++
					vx++
				}
			}
			// Fill to end of line (for cursorline highlighting).
			for x < width && tracker.Draw != nil {
				tracker.Draw(bx, by, x, y, ' ', nil, th.Default())
				x++
				vx++
			}
			end := newline()
			vx = 0
			if end {
				return
			}
		} else if vis.Special(r) {
			dr, style := vis.String(r, x, th)
			for _, c := range dr {
				if drawRune(off, c, nil, runewidth.RuneWidth(c), bx, by, style) {
					return
				}
			}
		} else {
			if drawRune(off, r, combc, gwidth, bx, by, th.Default()) {
				return
			}
		}

		off += size
	}
}
