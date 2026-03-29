package main

import (
	"fmt"
	"strconv"
	"strings"
)

// DrawFunc is called for each cell to render.
type DrawFunc func(x, y int, mainc rune, combc []rune, style Style)

// CursorFunc is called for each cursor position on screen.
type CursorFunc func(x, y int, main bool)

// View manages the visible window into a Buffer: scrolling, viewport
// dimensions, and rendering.
type View struct {
	buf *Buffer
	vis Visualizer

	topline int // first visible buffer line
	topcol  int // first visible byte column (softwrap)
	stcol   int // horizontal scroll start visual column
	width   int // total viewport width
	height  int // total viewport height

	// Display options (set from config).
	ScrollMargin  int
	HScrollMargin int
	SoftWrap      bool
	WordWrap      bool
	LineNums      bool
	CursorLine    bool
	GutterWidth   int // width for diagnostic markers (0 or 1)
}

// NewView creates a new View for the given buffer.
func NewView(buf *Buffer, tabsize int) *View {
	return &View{
		buf: buf,
		vis: Visualizer{
			TabSize: tabsize,
			CharMap: make(map[rune]string),
		},
		ScrollMargin:  5,
		HScrollMargin: 5,
		LineNums:      true,
		CursorLine:    true,
		GutterWidth:   1,
	}
}

// Resize sets the viewport dimensions.
func (v *View) Resize(w, h int) {
	v.width = w
	v.height = h
}

// Buffer returns the view's buffer.
func (v *View) Buffer() *Buffer {
	return v.buf
}

// lineNumWidth returns the width needed for line numbers.
func (v *View) lineNumWidth() int {
	if !v.LineNums {
		return 0
	}
	return len(strconv.Itoa(v.buf.NumLines()+1)) + 1
}

// gutterTotalWidth returns the total gutter width (diagnostic + line numbers).
func (v *View) gutterTotalWidth() int {
	return v.GutterWidth + v.lineNumWidth()
}

// bufferWidth returns the width available for buffer text.
func (v *View) bufferWidth() int {
	w := v.width - v.gutterTotalWidth()
	if w < 1 {
		return 1
	}
	return w
}

// --- Location types ---

// vLoc is a visual location (line, wrapped row, visual column).
type vLoc struct {
	line int // buffer line
	row  int // visual row within the line (0 if no softwrap)
	col  int // visual column
}

// bLoc is a buffer location (line, byte column).
type bLoc struct {
	line, col int
}

// bLoc2vLoc converts a buffer location to a visual location.
func (v *View) bLoc2vLoc(bl bLoc) (vl vLoc) {
	off := v.buf.OffsetAt(bl.line, 0)
	v.buf.RenderForward(RenderTracker{
		Track: func(off, bx, by, vx, vy int) bool {
			vl.row = vy
			vl.col = vx
			if (bx >= bl.col && by == bl.line) || by > bl.line {
				return true
			}
			return false
		},
	}, &v.vis, v.bufferWidth(), v.height, off, v.SoftWrap, v.WordWrap, DefaultTheme)
	vl.line = bl.line
	return vl
}

// --- Scrolling ---

// Relocate adjusts topline/stcol so that the primary cursor is visible.
func (v *View) Relocate() {
	c := v.buf.Cursor()
	bl := bLoc{}
	bl.line, bl.col = v.buf.LineColAt(c.Pos)

	// Vertical scrolling.
	if bl.line < v.topline+v.ScrollMargin {
		v.topline = bl.line - v.ScrollMargin
		v.topcol = 0
	} else if bl.line >= v.topline+v.height-v.ScrollMargin {
		top := bl.line - v.height + 1 + v.ScrollMargin
		maxTop := v.buf.NumLines() - v.height + 1
		if top > maxTop {
			top = maxTop
		}
		v.topline = top
		v.topcol = 0
	}
	if v.topline < 0 {
		v.topline = 0
	}

	// Horizontal scrolling (only when softwrap is off).
	if !v.SoftWrap {
		vl := v.bLoc2vLoc(bl)
		if vl.col < v.stcol+v.HScrollMargin {
			v.stcol = vl.col - v.HScrollMargin
			if v.stcol < 0 {
				v.stcol = 0
			}
		} else if vl.col >= v.stcol+v.bufferWidth()-v.HScrollMargin {
			v.stcol = vl.col - v.bufferWidth() + 1 + v.HScrollMargin
		}
	}
}

// --- Rendering ---

// Display renders the view by calling draw for each cell and showCursor for
// each cursor position.
func (v *View) Display(draw DrawFunc, showCursor CursorFunc, th *Theme) {
	gutter := v.gutterTotalWidth()
	lines := make([]int, v.height) // maps visual row -> buffer line+1

	// Track which lines have cursors (for cursorline).
	cursorlines := make(map[int]bool)
	if v.CursorLine {
		for _, c := range v.buf.Cursors() {
			line, _ := v.buf.LineColAt(c.Pos)
			cursorlines[line] = true
		}
	}
	cursorlineStyle := th.Style("cursorline")

	stpos := v.buf.OffsetAt(v.topline, v.topcol)
	v.buf.RenderForward(RenderTracker{
		Draw: func(bx, by, vx, vy int, mainc rune, combc []rune, style Style) {
			sx := gutter + vx - v.stcol
			if sx < gutter || sx >= v.width || vy >= v.height {
				return
			}
			if cursorlines[by] {
				style.Bg = cursorlineStyle.Bg
			}
			draw(sx, vy, mainc, combc, style)
		},
		Track: func(off, bx, by, vx, vy int) bool {
			if vy >= v.height {
				return true
			}
			lines[vy] = by + 1
			sx := gutter + vx - v.stcol
			for i, c := range v.buf.Cursors() {
				if !c.HasSelection() && c.Pos == off && sx >= gutter && sx < v.width {
					showCursor(sx, vy, i == 0)
				}
			}
			return false
		},
	}, &v.vis, v.bufferWidth(), v.height, stpos, v.SoftWrap, v.WordWrap, th)

	// Draw line numbers.
	if v.LineNums {
		lnumWid := v.lineNumWidth()
		strfmt := fmt.Sprintf("%%%dd ", lnumWid-1)
		lnumStyleFn := func(l int) Style {
			style := th.Style("line-number")
			if th.HasStyle("current-line-number") && cursorlines[l] {
				style = th.Style("current-line-number")
			}
			return style
		}

		for i, l := range lines {
			if l == 0 {
				break
			}
			var ls string
			if i != 0 && l == lines[i-1] {
				ls = strings.Repeat(" ", lnumWid)
			} else {
				ls = fmt.Sprintf(strfmt, l)
			}
			x := v.GutterWidth
			for _, c := range ls {
				draw(x, i, c, nil, lnumStyleFn(l-1))
				x++
			}
		}
	}

	// Draw gutter (diagnostic markers).
	if v.GutterWidth > 0 {
		for i, l := range lines {
			if l == 0 {
				break
			}
			ch := ' '
			style := th.Style("line-number")
			if d, ok := v.buf.GetDiagnosticAt(l - 1); ok {
				ch = '>'
				style = th.Style(d.Type.String()).Add(AttrReverse)
			}
			for x := 0; x < v.GutterWidth; x++ {
				draw(x, i, ch, nil, style)
			}
		}
	}
}
