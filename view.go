package main

import (
	"sort"
	"strconv"
)

// DrawFunc is called for each cell to render.
type DrawFunc func(x, y int, mainc rune, combc []rune, style Style)

// CursorFunc is called for each cursor position on screen.
type CursorFunc func(x, y int, main bool)

// overlayCell records a drawn cell's content so it can be redrawn restyled.
type overlayCell struct {
	r     rune
	combc []rune
	style Style
	ok    bool
}

// fakeCursorCell is a screen cell where a fake cursor must appear. clipped
// marks a cursor pinned to the last column because its true position is one
// past the right edge; wrapped marks one moved to the start of the row below
// (see the cursor placement in Display). Both draw over a cell the walk
// already painted, so the glyph under them is captured and redrawn.
type fakeCursorCell struct {
	x, y, off int
	clipped   bool
	wrapped   bool
}

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

	// Cached softwrap row geometry (see rowStarts): for each buffer line,
	// the byte column where each of its visual rows starts. Valid for one
	// buffer edit generation, length, and view width.
	geomGen   int
	geomLen   int
	geomWidth int
	geomRows  map[int][]int

	// Per-frame scratch reused by Display.
	scratchLines       []int
	scratchCursorlines map[int]bool

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
	return len(strconv.Itoa(v.buf.LastLine()+1)) + 1
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

// rowStarts returns the byte columns where each visual row of the line
// starts ([0] alone for an unwrapped line), cached until the buffer or the
// view width changes. The cache is what keeps the geometry helpers cheap on
// enormous lines: the line is walked once per edit, not several times per
// frame (each helper used to re-walk it from the start, which made a
// keystroke on a multi-megabyte line cost hundreds of milliseconds).
func (v *View) rowStarts(line int) []int {
	b := v.buf
	w := v.bufferWidth()
	if v.geomRows == nil || v.geomGen != b.EditGen() || v.geomLen != b.Len() || v.geomWidth != w {
		v.geomRows = make(map[int][]int)
		v.geomGen = b.EditGen()
		v.geomLen = b.Len()
		v.geomWidth = w
	}
	if s, ok := v.geomRows[line]; ok {
		return s
	}
	start := b.OffsetAt(line, 0)
	nl := start + b.LineLen(line)
	starts := []int{0}
	b.RenderForward(RenderTracker{
		Track: func(off, bx, by, tx, ty int) bool {
			if by > line || off >= nl {
				return true
			}
			if ty == len(starts) {
				starts = append(starts, bx)
			}
			return false
		},
	}, &v.vis, w, v.height, start, v.SoftWrap, v.WordWrap, DefaultTheme)
	v.geomRows[line] = starts
	return starts
}

// displayLoc returns the visual row of pos within its buffer line and the
// row-local visual column.
func (v *View) displayLoc(pos int) (row, col int) {
	b := v.buf
	line, bcol := b.LineColAt(pos)
	starts := v.rowStarts(line)
	row = sort.SearchInts(starts, bcol+1) - 1
	if row < 0 {
		row = 0
	}
	// Walk just this row for the row-local visual column: the renderer
	// resets the column at every wrap, so rendering from a row boundary
	// reproduces the display exactly.
	start := b.OffsetAt(line, starts[row])
	b.RenderForward(RenderTracker{
		Track: func(off, bx, by, tx, ty int) bool {
			col = tx
			return off >= pos
		},
	}, &v.vis, v.bufferWidth(), v.height, start, v.SoftWrap, v.WordWrap, DefaultTheme)
	return row, col
}

// displayRows returns how many visual rows the line's content occupies. A
// trailing row holding only the line's newline (a line exactly filling its
// last row) is not counted.
func (v *View) displayRows(line int) int {
	if !v.SoftWrap {
		return 1
	}
	return len(v.rowStarts(line))
}

// displayRowOf returns the buffer line of pos and its visual row within that
// line, clamped to the line's content rows.
func (v *View) displayRowOf(pos int) (int, int) {
	line, _ := v.buf.LineColAt(pos)
	if !v.SoftWrap {
		return line, 0
	}
	row, _ := v.displayLoc(pos)
	if r := v.displayRows(line); row >= r {
		row = r - 1
	}
	return line, row
}

// rowStartCol returns the byte column where the given visual row of a line
// starts (0 for the first row).
func (v *View) rowStartCol(line, row int) int {
	if !v.SoftWrap || row <= 0 {
		return 0
	}
	if s := v.rowStarts(line); row < len(s) {
		return s[row]
	}
	return 0
}

// displayPos returns the byte offset on the given visual row of a line whose
// column is closest to wantX without exceeding it, clamped to the row's last
// position (which may be the line's newline; callers VimClamp as needed).
func (v *View) displayPos(line, row, wantX int) int {
	b := v.buf
	if line > b.LastLine() {
		line = b.LastLine()
	}
	pos := b.OffsetAt(line, 0)
	starts := v.rowStarts(line)
	if row < 0 || row >= len(starts) {
		return pos
	}
	// Walk just the requested row.
	b.RenderForward(RenderTracker{
		Track: func(off, bx, by, tx, ty int) bool {
			if by > line || ty > 0 {
				return true
			}
			if tx > wantX {
				return true
			}
			pos = off
			return false
		},
	}, &v.vis, v.bufferWidth(), v.height, b.OffsetAt(line, starts[row]), v.SoftWrap, v.WordWrap, DefaultTheme)
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

// Viewport is a snapshot of a view's scroll position, for exact
// save/restore (e.g. returning from a cancelled search).
type Viewport struct {
	TopLine, TopCol, StCol int
}

// Viewport returns the current scroll position.
func (v *View) Viewport() Viewport {
	return Viewport{TopLine: v.topline, TopCol: v.topcol, StCol: v.stcol}
}

// SetViewport restores a scroll position snapshot.
func (v *View) SetViewport(vp Viewport) {
	v.topline = vp.TopLine
	v.topcol = vp.TopCol
	v.stcol = vp.StCol
}

// --- Visual-row viewport ---
//
// The viewport starts at a visual row: topline is a buffer line and topcol
// the byte column of one of its wrap-row starts (0 unless a softwrapped
// line is partially scrolled off the top). Because the renderer resets the
// column at every wrap, rendering from a row boundary produces exactly the
// same rows as rendering the whole line, so a tall wrapped line can be
// entered gradually.

// topRow returns the viewport start as a (line, visual row) pair, snapping
// stale state (edits, resizes) to a sane position.
func (v *View) topRow() (int, int) {
	if v.topline < 0 {
		return 0, 0
	}
	if v.topline > v.buf.LastLine() {
		return v.buf.LastLine(), 0
	}
	if !v.SoftWrap || v.topcol <= 0 || v.topcol >= v.buf.LineLen(v.topline) {
		return v.topline, 0
	}
	row, _ := v.displayLoc(v.buf.OffsetAt(v.topline, v.topcol))
	return v.topline, row
}

// setTopRow sets the viewport start to the given (line, visual row).
func (v *View) setTopRow(line, row int) {
	if line < 0 {
		line, row = 0, 0
	}
	v.topline = line
	v.topcol = v.rowStartCol(line, row)
}

// stepRows advances a (line, visual row) position by n rows (n may be
// negative), clamping at the buffer's first and last visual rows.
func (v *View) stepRows(line, row, n int) (int, int) {
	for n > 0 {
		rows := v.displayRows(line)
		if row+n < rows {
			return line, row + n
		}
		if line >= v.buf.LastLine() {
			return line, rows - 1
		}
		n -= rows - row
		line++
		row = 0
	}
	for n < 0 {
		if row+n >= 0 {
			return line, row + n
		}
		if line <= 0 {
			return line, 0
		}
		n += row + 1
		line--
		row = v.displayRows(line) - 1
	}
	return line, row
}

// rowsBetween returns the number of visual rows from one display position
// down to another (negative if the target is above), with the magnitude
// capped at limit.
func (v *View) rowsBetween(fromLine, fromRow, toLine, toRow, limit int) int {
	if fromLine == toLine {
		return clamp(toRow-fromRow, -limit, limit)
	}
	if toLine > fromLine {
		n := v.displayRows(fromLine) - fromRow
		for l := fromLine + 1; l < toLine && n < limit; l++ {
			n += v.displayRows(l)
		}
		n += toRow
		if n > limit {
			n = limit
		}
		return n
	}
	n := fromRow
	for l := fromLine - 1; l > toLine && n < limit; l-- {
		n += v.displayRows(l)
	}
	n += v.displayRows(toLine) - toRow
	if n > limit {
		n = limit
	}
	return -n
}

// maxTopRow returns the lowest viewport start that keeps the window full:
// the buffer's last visual row sits on the bottom screen row.
func (v *View) maxTopRow() (int, int) {
	last := v.buf.LastLine()
	return v.stepRows(last, v.displayRows(last)-1, -(v.height - 1))
}

// Relocate adjusts the viewport so that the primary cursor is visible,
// scrolling by visual rows so softwrapped lines are handled correctly.
func (v *View) Relocate() {
	c := v.buf.Cursor()
	bl := bLoc{}
	bl.line, bl.col = v.buf.LineColAt(c.Pos)

	// Vertical scrolling.
	margin := v.effScrollMargin()
	curLine, curRow := v.displayRowOf(c.Pos)
	topLine, topRow := v.topRow()

	// A stale viewport start can sit below maxTopRow — a session restored
	// from a smaller pane, a window that grew, a file that shrank — which
	// would leave the window under-full, showing blank rows past the end
	// of the buffer. Vim keeps the window full whenever the buffer allows,
	// so clamp before the margin logic (which otherwise keeps any viewport
	// that already shows the cursor). Every line fills at least one row,
	// so the window can only be under-full when fewer than height lines
	// remain below the top; the cheap line-count check skips the precise
	// (row-walking) maxTopRow computation whenever the window is
	// obviously full.
	if topLine > v.buf.LastLine()-v.height {
		if ml, mr := v.maxTopRow(); topLine > ml || (topLine == ml && topRow > mr) {
			topLine, topRow = ml, mr
			v.setTopRow(ml, mr)
		}
	}

	dist := v.rowsBetween(topLine, topRow, curLine, curRow, v.height+1)
	if dist < margin {
		v.setTopRow(v.stepRows(curLine, curRow, -margin))
	} else if dist >= v.height-margin {
		l, r := v.stepRows(curLine, curRow, -(v.height - 1 - margin))
		if ml, mr := v.maxTopRow(); l > ml || (l == ml && r > mr) {
			l, r = ml, mr
		}
		v.setTopRow(l, r)
	} else if v.topcol != 0 || v.topline != topLine {
		// No scroll needed, but re-anchor topcol on a valid row boundary
		// (it can drift after edits or resizes).
		v.setTopRow(topLine, topRow)
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

// inlayHintGutterStyle returns the theme style for the inlay-hint gutter
// marker: gutter-hint when defined, falling back to the comment style.
func inlayHintGutterStyle(th *Theme) Style {
	if th.HasStyle("gutter-hint") {
		return th.Style("gutter-hint")
	}
	return th.Style("comment")
}

// --- Rendering ---

// Display renders the view by calling draw for each cell and showCursor for
// each cursor position. If active is false, cursorline highlighting is skipped.
func (v *View) Display(draw DrawFunc, showCursor CursorFunc, th *Theme, active ...bool) {
	isActive := len(active) == 0 || active[0]
	gutter := v.gutterTotalWidth()

	// Per-frame scratch, reused across frames to avoid churn.
	if cap(v.scratchLines) < v.height {
		v.scratchLines = make([]int, v.height)
	}
	lines := v.scratchLines[:v.height] // maps visual row -> buffer line+1
	for i := range lines {
		lines[i] = 0
	}
	if v.scratchCursorlines == nil {
		v.scratchCursorlines = make(map[int]bool)
	}
	clear(v.scratchCursorlines)

	// Track which lines have cursors (for cursorline).
	// Disable cursorline when any cursor has an active selection.
	cursorlines := v.scratchCursorlines
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

	// Fake cursors: the terminal hardware cursor can only mark one
	// position, so when several cursors exist every one of them (primary
	// included) is drawn as a block cursor by inverting its cell, which
	// stays visible on any background (selection, search, cursorline). A
	// theme may override the look with a "cursor" style.
	fake := isActive && v.buf.NumCursors() > 1
	cursorStyle := func(base Style) Style {
		if th.HasStyle("cursor") {
			return th.Style("cursor")
		}
		base.Attr ^= AttrReverse
		return base
	}
	var cursorAt, cursorDrawn map[int]bool
	var edgeCells, firstCells []overlayCell
	var fakeCursors []fakeCursorCell
	if fake {
		cursorAt = make(map[int]bool)
		cursorDrawn = make(map[int]bool)
		for _, c := range v.buf.Cursors() {
			cursorAt[c.Pos] = true
		}
		edgeCells = make([]overlayCell, v.height)
		firstCells = make([]overlayCell, v.height)
	}

	var curOff int
	// Walk end position (in view coordinates), for blanking the region
	// past the end of the buffer.
	endX, endY := 0, 0

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
			if fake {
				// Only the first cell drawn for the cursor's offset is
				// the cursor cell (a tab's later cells and end-of-line
				// fills share the offset).
				if cursorAt[curOff] && !cursorDrawn[curOff] {
					cursorDrawn[curOff] = true
					style = cursorStyle(style)
				} else {
					if sx == v.width-1 {
						edgeCells[vy] = overlayCell{mainc, combc, style, true}
					}
					if sx == gutter && !firstCells[vy].ok {
						firstCells[vy] = overlayCell{mainc, combc, style, true}
					}
				}
			}
			draw(sx, vy, mainc, combc, style)
		},
		Track: func(off, bx, by, vx, vy int) bool {
			curOff = off
			endX, endY = vx, vy
			if vy >= v.height {
				return true
			}
			lines[vy] = by + 1
			sx := gutter + vx - v.stcol
			for i, c := range v.buf.Cursors() {
				if c.Pos == off && sx >= gutter {
					s, sy := sx, vy
					clipped, wrapped := false, false
					if s >= v.width {
						// The cursor sits one past the last cell of an
						// exactly-full row (insert mode at the end of
						// such a line). Softwrapped, that is where the
						// next character typed goes, so the cursor
						// belongs at the start of the row below rather
						// than on top of the line's last character.
						// Without softwrap there is no row below to
						// wrap onto, and on the last row there is none
						// on screen: pin to the last cell rather than
						// leave the cursor stale off-screen.
						if v.SoftWrap && vy+1 < v.height {
							s, sy, wrapped = gutter, vy+1, true
						} else {
							s, clipped = v.width-1, true
						}
					}
					showCursor(s, sy, i == 0)
					if fake {
						fakeCursors = append(fakeCursors, fakeCursorCell{s, sy, off, clipped, wrapped})
					}
				}
			}
			return false
		},
	}, &v.vis, v.bufferWidth(), v.height, stpos, v.SoftWrap, v.WordWrap, th)

	// The walk paints nothing past the end of the buffer; blank the
	// remainder of the viewport (the phantom-line row's tail and any rows
	// below it) so the editor needs no per-frame whole-screen clear. For
	// a full window the walk ends past the last row and this is free.
	if endY < v.height {
		blank := th.Default()
		sx := gutter + endX - v.stcol
		if sx < gutter {
			sx = gutter
		}
		for x := sx; x < v.width; x++ {
			draw(x, endY, ' ', nil, blank)
		}
		for yy := endY + 1; yy < v.height; yy++ {
			for x := 0; x < v.width; x++ {
				draw(x, yy, ' ', nil, blank)
			}
		}
	}

	// Fake cursors whose cell produced no drawn glyph — a cursor at the
	// end of the buffer, or one moved off an exactly-full row (wrapped to
	// the row below, or pinned to the last column) — get an overlay cell
	// drawn directly, reusing the captured content of the column they land
	// on so the glyph under the cursor is preserved.
	for _, fc := range fakeCursors {
		if cursorDrawn[fc.off] {
			continue
		}
		cell := overlayCell{r: ' ', style: th.Default()}
		if fc.clipped && edgeCells[fc.y].ok {
			cell = edgeCells[fc.y]
		} else if fc.wrapped && firstCells[fc.y].ok {
			cell = firstCells[fc.y]
		}
		draw(fc.x, fc.y, cell.r, cell.combc, cursorStyle(cell.style))
	}

	// Draw line numbers (without fmt: this runs per visible line per
	// frame).
	if v.LineNums {
		lnumWid := v.lineNumWidth()
		var numBuf [20]byte
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
			style := lnumStyleFn(l - 1)
			x := v.GutterWidth
			if i != 0 && l == lines[i-1] {
				// Continuation row of a wrapped line: blank.
				for j := 0; j < lnumWid; j++ {
					draw(x+j, i, ' ', nil, style)
				}
				continue
			}
			num := strconv.AppendInt(numBuf[:0], int64(l), 10)
			pad := lnumWid - 1 - len(num)
			for j := 0; j < pad; j++ {
				draw(x+j, i, ' ', nil, style)
			}
			for j, c := range num {
				draw(x+pad+j, i, rune(c), nil, style)
			}
			draw(x+lnumWid-1, i, ' ', nil, style)
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
			} else if _, ok := v.buf.GetInlayHintAt(l - 1); ok {
				ch = 'i'
				style = inlayHintGutterStyle(th)
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
