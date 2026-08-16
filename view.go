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
// dimensions, and rendering. Each View has its own saved cursor so that
// multiple views of the same buffer scroll independently.
type View struct {
	buf         *Buffer
	vis         Visualizer
	savedCursor Cursor // this view's cursor (synced on activate/deactivate)

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

	// Highlight is a byte range [start, end) to display with a search
	// highlight style. Set to [0,0] for no highlight.
	Highlight [2]int

	// Opts holds per-buffer resolved options (autoindent, tabsize, etc.).
	Opts map[string]any
}

// NewView creates a new View for the given buffer.
func NewView(buf *Buffer, tabsize int) *View {
	v := &View{
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
	buf.vis = &v.vis
	return v
}

// Activate restores this view's saved cursor to the buffer. Call when
// switching focus to this view.
func (v *View) Activate() {
	*v.buf.Cursor() = v.savedCursor
}

// Deactivate saves the buffer's current cursor to this view. Call before
// switching focus away.
func (v *View) Deactivate() {
	v.savedCursor = *v.buf.Cursor()
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

// --- Display (wrapped) line geometry ---
//
// With softwrap on, a buffer line occupies one or more visual rows. These
// helpers map between byte offsets and (row, row-local column) positions
// using the same render walk as the display, so they agree exactly with
// what is on screen (including per-row tab expansion).

// displayLoc returns the visual row of pos within its buffer line and the
// row-local visual column.
func (v *View) displayLoc(pos int) (row, col int) {
	b := v.buf
	line, _ := b.LineColAt(pos)
	start := b.OffsetAt(line, 0)
	b.RenderForward(RenderTracker{
		Track: func(off, bx, by, tx, ty int) bool {
			row, col = ty, tx
			return off >= pos
		},
	}, &v.vis, v.bufferWidth(), v.height, start, v.SoftWrap, v.WordWrap, DefaultTheme)
	return row, col
}

// displayRows returns how many visual rows the line's content occupies. A
// trailing row holding only the line's newline (a line exactly filling its
// last row) is not counted.
func (v *View) displayRows(line int) int {
	b := v.buf
	start := b.OffsetAt(line, 0)
	nl := start + b.LineLen(line)
	rows := 1
	b.RenderForward(RenderTracker{
		Track: func(off, bx, by, tx, ty int) bool {
			if by > line || off >= nl {
				return true
			}
			rows = ty + 1
			return false
		},
	}, &v.vis, v.bufferWidth(), v.height, start, v.SoftWrap, v.WordWrap, DefaultTheme)
	return rows
}

// displayPos returns the byte offset on the given visual row of a line whose
// column is closest to wantX without exceeding it, clamped to the row's last
// position (which may be the line's newline; callers VimClamp as needed).
func (v *View) displayPos(line, row, wantX int) int {
	b := v.buf
	if line > b.NumLines() {
		line = b.NumLines()
	}
	pos := b.OffsetAt(line, 0)
	b.RenderForward(RenderTracker{
		Track: func(off, bx, by, tx, ty int) bool {
			if by > line || ty > row {
				return true
			}
			if ty == row {
				if tx > wantX {
					return true
				}
				pos = off
			}
			return false
		},
	}, &v.vis, v.bufferWidth(), v.height, b.OffsetAt(line, 0), v.SoftWrap, v.WordWrap, DefaultTheme)
	return pos
}

// --- Scrolling ---

// effScrollMargin returns the vertical scroll margin clamped so that a
// stable viewport position always exists. With a margin larger than
// (height-1)/2 the top and bottom conditions overlap and the viewport
// would oscillate on every relocate.
func (v *View) effScrollMargin() int {
	m := v.ScrollMargin
	if max := (v.height - 1) / 2; m > max {
		m = max
	}
	if m < 0 {
		m = 0
	}
	return m
}

// effHScrollMargin returns the horizontal scroll margin clamped to the
// buffer width for the same reason.
func (v *View) effHScrollMargin() int {
	m := v.HScrollMargin
	if max := (v.bufferWidth() - 1) / 2; m > max {
		m = max
	}
	if m < 0 {
		m = 0
	}
	return m
}

// Relocate adjusts topline/stcol so that the primary cursor is visible.
func (v *View) Relocate() {
	c := v.buf.Cursor()
	bl := bLoc{}
	bl.line, bl.col = v.buf.LineColAt(c.Pos)

	// Vertical scrolling.
	margin := v.effScrollMargin()
	if bl.line < v.topline+margin {
		v.topline = bl.line - margin
		v.topcol = 0
	} else if bl.line >= v.topline+v.height-margin {
		top := bl.line - v.height + 1 + margin
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
		hmargin := v.effHScrollMargin()
		vl := v.bLoc2vLoc(bl)
		if vl.col < v.stcol+hmargin {
			v.stcol = vl.col - hmargin
			if v.stcol < 0 {
				v.stcol = 0
			}
		} else if vl.col >= v.stcol+v.bufferWidth()-hmargin {
			v.stcol = vl.col - v.bufferWidth() + 1 + hmargin
		}
	}
}

// diagGutterStyle returns the theme style for a diagnostic gutter marker:
// gutter-error / gutter-warning when defined (all shipped themes define
// them), falling back to the bare error / warning groups.
func diagGutterStyle(th *Theme, t DiagnosticType) Style {
	if name := "gutter-" + t.String(); th.HasStyle(name) {
		return th.Style(name)
	}
	return th.Style(t.String())
}

// --- Rendering ---

// Display renders the view by calling draw for each cell and showCursor for
// each cursor position. If active is false, cursorline highlighting is skipped.
func (v *View) Display(draw DrawFunc, showCursor CursorFunc, th *Theme, active ...bool) {
	isActive := len(active) == 0 || active[0]
	gutter := v.gutterTotalWidth()
	lines := make([]int, v.height) // maps visual row -> buffer line+1

	// Track which lines have cursors (for cursorline).
	// Disable cursorline when any cursor has an active selection.
	cursorlines := make(map[int]bool)
	if v.CursorLine && isActive {
		hasSelection := false
		for _, c := range v.buf.Cursors() {
			if c.HasSelection() {
				hasSelection = true
				break
			}
		}
		if !hasSelection {
			for _, c := range v.buf.Cursors() {
				line, _ := v.buf.LineColAt(c.Pos)
				cursorlines[line] = true
			}
		}
	}
	cursorlineStyle := th.Style("cursorline")

	hasHL := v.Highlight[0] != v.Highlight[1]
	hlStyle := th.Default().Add(AttrReverse)
	if th.HasStyle("search") {
		hlStyle = th.Style("search")
	}

	var curOff int

	// When horizontally scrolled, end-of-line fills must reach the right
	// edge of the visible window (stcol+width), not just column `width`.
	fillTo := 0
	if !v.SoftWrap {
		fillTo = v.stcol + v.bufferWidth()
	}

	stpos := v.buf.OffsetAt(v.topline, v.topcol)
	v.buf.RenderForward(RenderTracker{
		FillTo: fillTo,
		Draw: func(bx, by, vx, vy int, mainc rune, combc []rune, style Style) {
			sx := gutter + vx - v.stcol
			if sx < gutter || sx >= v.width || vy >= v.height {
				return
			}
			if hasHL && curOff >= v.Highlight[0] && curOff < v.Highlight[1] {
				style = hlStyle
			}
			if cursorlines[by] && !hasHL {
				style.Bg = cursorlineStyle.Bg
			}
			draw(sx, vy, mainc, combc, style)
		},
		Track: func(off, bx, by, vx, vy int) bool {
			curOff = off
			if vy >= v.height {
				return true
			}
			lines[vy] = by + 1
			sx := gutter + vx - v.stcol
			for i, c := range v.buf.Cursors() {
				if c.Pos == off && sx >= gutter && sx < v.width {
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
		lastRow := 0
		for i, l := range lines {
			if l == 0 {
				break
			}
			lastRow = i
			ch := ' '
			style := th.Style("line-number")
			if d, ok := v.buf.GetDiagnosticAt(l - 1); ok {
				ch = '>'
				style = diagGutterStyle(th, d.Type).Add(AttrReverse)
			}
			for x := 0; x < v.GutterWidth; x++ {
				draw(x, i, ch, nil, style)
			}
		}

		// Off-screen diagnostic indicators: show ^ on the first row if
		// there are diagnostics above the viewport, v on the last row if
		// there are diagnostics below. Use the color of the nearest
		// off-screen diagnostic.
		botLine := -1
		if lines[lastRow] > 0 {
			botLine = lines[lastRow] - 1
		}
		var nearestAbove, nearestBelow *Diagnostic
		for i := range v.buf.GetDiagnostics() {
			d := &v.buf.GetDiagnostics()[i]
			if d.Line < v.topline {
				if nearestAbove == nil || d.Line > nearestAbove.Line {
					nearestAbove = d
				}
			}
			if botLine >= 0 && d.Line > botLine {
				if nearestBelow == nil || d.Line < nearestBelow.Line {
					nearestBelow = d
				}
			}
		}
		if nearestAbove != nil && lines[0] > 0 {
			if _, ok := v.buf.GetDiagnosticAt(lines[0] - 1); !ok {
				s := diagGutterStyle(th, nearestAbove.Type).Add(AttrReverse)
				for x := 0; x < v.GutterWidth; x++ {
					draw(x, 0, '^', nil, s)
				}
			}
		}
		if nearestBelow != nil && botLine >= 0 {
			if _, ok := v.buf.GetDiagnosticAt(botLine); !ok {
				s := diagGutterStyle(th, nearestBelow.Type).Add(AttrReverse)
				for x := 0; x < v.GutterWidth; x++ {
					draw(x, lastRow, 'v', nil, s)
				}
			}
		}
	}
}
