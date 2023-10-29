package buffer

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zyedidia/go-runewidth"
	"github.com/zyedidia/gpeg/vm"
	"github.com/zyedidia/mu/pkg/theme"
)

// A RenderTracker is a pair of functions that are used to act on results from
// rendering. Draw is called when a character is drawn at a certain location
// and indicates the styling, and Track is used to keep track of conversions
// between buffer locations and visual locations.
type RenderTracker struct {
	Draw  func(vx, vy int, mainc rune, combc []rune, style theme.Style)
	Track func(off, bx, by, vx, vy int) bool
}

// A RuneVisualizer specifies how runes should be visualized. For example a
// runevisualizer may convert a tab ('\t') to a certain number of spaces, or
// render non-graphic characters using hex codes.
type RuneVisualizer interface {
	// String should not contain any combining characters.
	String(r rune, vx int, th *theme.Theme) (string, theme.Style)
	Size(r rune, vx int, width int) int
	Special(r rune) bool
}

// Visualizer provides a default implementation of a RuneVisualizer. It can be
// configured for tab size and can map arbitrary runes to single character
// strings. This is used for whitespace visualization. Non-graphic characters
// are rendered using hex codes.
type Visualizer struct {
	TabSize int
	CharMap map[rune]string // strings must be single runes
}

func (v *Visualizer) String(r rune, vx int, th *theme.Theme) (string, theme.Style) {
	if r == '\t' {
		tsz := v.TabSize
		if char, ok := v.CharMap[r]; ok {
			return char + strings.Repeat(" ", tsz-(vx%tsz)-1), th.Style("hidden-char")
		}
		return strings.Repeat(" ", tsz-(vx%tsz)), th.Default()
	} else if s, ok := v.CharMap[r]; ok {
		return s, th.Style("hidden-char")
	} else if unicode.IsPrint(r) {
		return string(r), th.Default()
	}

	return fmt.Sprintf("<%02x>", r), th.Default()
}

func (v *Visualizer) Size(r rune, vx int, width int) int {
	if r == '\t' {
		tsz := v.TabSize
		return tsz - (vx % tsz)
	} else if s, ok := v.CharMap[r]; ok {
		r, _ := utf8.DecodeRuneInString(s)
		return runewidth.RuneWidth(r)
	} else if unicode.IsPrint(r) {
		return width
	} else if r == '\t' {
		tsz := v.TabSize
		return tsz - (vx % tsz)
	}

	// <xx> is 4 bytes
	return 4
}

func (v *Visualizer) Special(r rune) bool {
	if _, ok := v.CharMap[r]; ok {
		return true
	}
	return !unicode.IsPrint(r)
}

// RenderForward draws this buffer in a box of 'width' and 'height', starting
// from byte offset 'off'. The 'softwrap' and 'wordwrap' inputs control
// wrapping, 'th' controls the theme used for highlighting.
func (b *Buffer) RenderForward(tracker RenderTracker, width, height, off int, displayer RuneVisualizer, softwrap, wordwrap bool, th *theme.Theme) {
	// vx is the visual x within the line without taking into account softwrap
	// (so therefore not the actual x, y coordinate the character is drawn at).
	// This is needed for keeping track of the correct tabstop width.
	var vx, x, y int

	if tracker.Draw != nil {
		if b.hisem.TryAcquire(1) {
			if b.highlighter != nil {
				l, _ := b.LineColAt(off)
				end := b.OffsetAt(l+height, 0)
				// highlight if the range is not in the matches
				if b.matches == nil || b.minvalid || !b.matches.InRange(off) || !b.matches.InRange(end-1) {
					b.matches = b.highlighter.HighlightMatches(b.Buffer.Reader, b.syntbl, &vm.Interval{off, end})
					b.minvalid = false
				}
			}
			b.hisem.Release(1)
		}
	}

	newline := func() bool {
		x = 0
		y++
		// TODO: for now return false and let the caller terminate the
		// function. In the future we may want to optimize so that we can avoid
		// rendering unviewable text
		return false
		// return y >= height
	}

	drawRune := func(off int, c rune, combc []rune, rwidth, bx, by int, style theme.Style) (done, loop bool) {
		if tracker.Draw != nil {
			if b.matches != nil && style == th.Default() {
				style = th.Style(b.matches.Group(off))
			}
			tracker.Draw(x, y, c, combc, style)
		}
		x += rwidth
		vx += rwidth

		if x >= width {
			if softwrap {
				if newline() {
					return true, false
				}
				return false, false
			} else {
				// TODO: maybe we can optimize long lines with no
				// softwrap the issue is that if we cut off drawing
				// early, the horizontal scrolling that the caller may
				// attempt to apply will be incorrect.  if we want to
				// support this optimization, we have to pass
				// horizontal scrolling information to this function
				// instead.

				// off = b.OffsetAt(by+1, 0)
				// if newline() {
				// 	return
				// }
				// continue loop
			}
		}
		return false, false
	}

loop:
	for {
		blen := b.Len()
		if off >= blen {
			by, bx := b.LineColAt(blen - 1)
			if tracker.Track(blen, bx+1, by, x, y) {
				return
			}
			break
		}

		if softwrap && wordwrap {
			wordsz := b.wordSizeAt(vx, off, identchar, displayer)
			if x+wordsz-1 > width {
				if newline() {
					return
				}
			}
		}

		// get rune at off
		r, combc, size, width := b.DecodeGraphemeWidthAt(off)
		by, bx := b.LineColAt(off)
		if tracker.Track(off, bx, by, x, y) {
			return
		}

		if r == '\n' {
			str, style := displayer.String(r, x, th)
			if str != "\n" {
				for _, c := range str {
					drawRune(off, c, nil, width, bx, by, style)
				}
			}
			end := newline()
			vx = 0
			if end {
				return
			}
		} else if displayer.Special(r) {
			dr, style := displayer.String(r, x, th)
			for _, c := range dr {
				done, loop := drawRune(off, c, nil, runewidth.RuneWidth(c), bx, by, style)
				if done {
					return
				} else if loop {
					continue loop
				}
			}
		} else {
			done, loop := drawRune(off, r, combc, width, bx, by, th.Default())
			if done {
				return
			} else if loop {
				continue loop
			}
		}

		off += size
	}
}

func (b *Buffer) wordSizeAt(vx, off int, wordchar func(r rune) bool, displayer RuneVisualizer) int {
	vn := 0
	r, _, sz, width := b.DecodeGraphemeWidthAt(off)
	off += sz
	vn += displayer.Size(r, vx+vn, width)
	for wordchar(r) {
		r, _, sz, width = b.DecodeGraphemeWidthAt(off)
		off += sz
		vn += displayer.Size(r, vx+vn, width)
	}
	return vn
}

func identchar(r rune) bool {
	return (r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r == '_')
}
