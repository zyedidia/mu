package buf

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zyedidia/mu/buffer"
	"github.com/zyedidia/mu/pkg/theme"
)

func (bp *BufPane) Resize(w, h int) {
	bp.width, bp.height = w, h
}

// A vLine is a visual representation of a line, which is needed because
// softwrap may cause a single buffer line to be displayed as multiple lines on
// screen. The 'line' is the buffer line, and the 'row' is the visual row
// within the buffer line.
type vLine struct {
	line int
	row  int
}

// A visual location, which stores the visual column offset from the vLine.
type vLoc struct {
	vLine
	col int
}

func (v vLoc) Compare(other vLoc) int {
	if v.line < other.line {
		return -1
	} else if v.line > other.line {
		return 1
	}
	if v.row < other.row {
		return -1
	} else if v.row > other.row {
		return 1
	}
	if v.col < other.col {
		return -1
	} else if v.col > other.row {
		return 1
	}
	return 0
}

// A buffer location, where the col is a byte offset from the beginning of the
// buffer line.
type bLoc struct {
	line, col int
}

func (b *BufPane) vLoc2bLoc(vl vLoc) (bl bLoc) {
	off := b.OffsetAt(vl.line, 0)
	b.Buffer.RenderForward(buffer.RenderTracker{
		Draw: nil,
		Track: func(off, bx, by, vx, vy int) bool {
			if vx == vl.col && vy == vl.row {
				bl.line = by
				bl.col = bx
				return true
			} else if vy > vl.row || (vx >= vl.col && vy == vl.row) {
				return true
			}
			bl.line = by
			bl.col = bx
			return false
		},
	}, b.bufferWidth(), b.height, off, b.vis, b.softwrap, b.wordwrap, nil)
	return bl
}

func (b *BufPane) MouseLoc(x, y int) (int, int) {
	x -= b.width - b.bufferWidth()
	var bl bLoc
	b.Buffer.RenderForward(buffer.RenderTracker{
		Draw: nil,
		Track: func(off, bx, by, vx, vy int) bool {
			vx -= b.stcol
			if vx == x && vy == y {
				bl.line = by
				bl.col = bx
				return true
			} else if vy > y || (vx >= x && vy == y) {
				return true
			}
			bl.line = by
			bl.col = bx
			return false
		},
	}, b.bufferWidth(), b.height, b.stpos, b.vis, b.softwrap, b.wordwrap, nil)
	return bl.line, bl.col
}

func (b *BufPane) bLoc2vLoc(bl bLoc) (vl vLoc) {
	off := b.OffsetAt(bl.line, 0)
	b.Buffer.RenderForward(buffer.RenderTracker{
		Draw: nil,
		Track: func(off, bx, by, vx, vy int) bool {
			vl.row = vy
			vl.col = vx
			if bx >= bl.col && by == bl.line || by > bl.line {
				return true
			}
			return false
		},
	}, b.bufferWidth(), b.height, off, b.vis, b.softwrap, b.wordwrap, nil)
	vl.line = bl.line
	return vl
}

func (b *BufPane) rowcount(line int) int {
	vl := b.bLoc2vLoc(bLoc{line: line, col: b.Buffer.LineLen(line)})
	return vl.row + 1
}

func (b *BufPane) vLinesUp(vl vLoc, lines int) vLoc {
	if lines < vl.row {
		vl.row -= lines
		vl.col = 0
		return vl
	}
	n := vl.row
	i := vl.line - 1
	rows := b.rowcount(i)
	for n+rows < lines {
		i--
		n += rows
		rows = b.rowcount(i)
	}
	vl.line = i
	vl.row = rows - (lines - n)
	vl.col = 0
	return vl
}

func (b *BufPane) vDistance(v1, v2 vLoc) int {
	// rows at the end of v1
	rows := b.rowcount(v1.line) - v1.row
	// rows between v1 and v2
	for i := v1.line + 1; i <= v2.line-1; i++ {
		rows += b.rowcount(i)
	}
	// rows in v2
	rows += v2.row
	return rows
}

// Relocate updates the window so that the given buffer location is in the
// view.
func (b *BufPane) Relocate(bl bLoc) {
	topl, topc := b.LineColAt(b.stpos)
	vl := b.bLoc2vLoc(bl)

	// vertical scrolling
	if !b.softwrap {
		if bl.line < topl+b.scrollmargin {
			b.stpos = b.OffsetAt(bl.line-b.scrollmargin, 0)
		} else if bl.line >= topl+b.height-b.scrollmargin {
			top := min(bl.line-b.height+1+b.scrollmargin, b.NumLines()-b.height+1)
			b.stpos = b.OffsetAt(top, 0)
		}
	} else {
		vtop := b.bLoc2vLoc(bLoc{line: topl, col: topc})
		topscroll := b.vLinesUp(vl, b.scrollmargin)
		bottomscroll := b.vLinesUp(vl, b.height-1-b.scrollmargin)
		if topscroll.Compare(vtop) < 0 {
			vtop = topscroll
			vtop.col = 0
		} else if bottomscroll.Compare(vtop) >= 0 {
			vtop = bottomscroll
			if b.NumLines()-vl.line < b.height {
				checktop := b.vLinesUp(b.bLoc2vLoc(bLoc{line: b.NumLines()}), b.height-1)
				if checktop.Compare(vtop) < 0 {
					vtop = checktop
				}
			}
		}
		btop := b.vLoc2bLoc(vtop)
		b.stpos = b.OffsetAt(btop.line, btop.col)
	}

	// horizontal scrolling
	if !b.softwrap {
		if vl.col < b.stcol+b.hscrollmargin {
			b.stcol = max(vl.col-b.hscrollmargin, 0)
		} else if vl.col >= b.stcol+b.bufferWidth()-b.hscrollmargin {
			b.stcol = vl.col - b.bufferWidth() + 1 + b.hscrollmargin
		}
	}
}

func (b *BufPane) Display(draw func(vx, vy int, mainc rune, combc []rune, style theme.Style), showCursor func(x, y int, main bool), th *theme.Theme) {
	lines := make([]int, b.height)

	linewid := 0
	if b.linenums {
		linewid = b.lnumWidth()
	}
	gutter := b.gutterWidth() + linewid

	b.Buffer.RenderForward(buffer.RenderTracker{
		Draw: func(vx, vy int, mainc rune, combc []rune, style theme.Style) {
			if gutter+vx-b.stcol >= b.width || vy >= b.height {
				return
			}
			draw(gutter+vx-b.stcol, vy, mainc, combc, style)
		},
		Track: func(off, bx, by, vx, vy int) bool {
			if vy >= b.height {
				return true
			}
			lines[vy] = by + 1
			for i, c := range b.Cursors() {
				if !c.HasSelection() && c.Pos == off && gutter+vx-b.stcol < b.width {
					showCursor(gutter+vx-b.stcol, vy, i == 0)
				}
			}
			return false
		},
	}, b.bufferWidth(), b.height, b.stpos, b.vis, b.softwrap, b.wordwrap, th)

	if b.linenums {
		strfmt := fmt.Sprintf("%%%dd ", linewid-1)
		for i, l := range lines {
			var ls string
			if l == 0 {
				break
			} else if i != 0 && l == lines[i-1] {
				ls = strings.Repeat(" ", linewid)
			} else {
				ls = fmt.Sprintf(strfmt, l)
			}

			x := b.gutterWidth() // start after gutter
			for _, c := range ls {
				draw(x, i, c, nil, th.Style("line-number"))
				x++
			}
		}
	}

	if b.gutter == 0 {
		return
	}

	c := b.Cursor()
	cline, _ := b.LineColAt(c.Pos)
	if b.messager != nil {
		b.messager.ClearDiagnostic()
	}
	for i, l := range lines {
		d, ok := b.GetDiagnosticAt(l - 1)
		ch := ' '
		style := th.Style("line-number")
		if ok {
			if d.Line == cline && b.messager != nil {
				b.messager.DiagnosticMessage(d.Text)
			}
			ch = '>'
			style = th.Style(d.Type.String()).Add(theme.AttrReverse)
		} else if i == 0 {
			if d, ok := b.GetDiagnosticBelow(l - 1); ok {
				ch = '^'
				style = th.Style(d.Type.String()).Add(theme.AttrReverse)
			}
		} else if i == len(lines)-1 {
			if d, ok := b.GetDiagnosticAbove(l - 1); ok {
				ch = 'v'
				style = th.Style(d.Type.String()).Add(theme.AttrReverse)
			}
		}
		for x := 0; x < b.gutterWidth(); x++ {
			draw(x, i, ch, nil, style)
		}
	}
}

func (b *BufPane) lnumWidth() int {
	return len(strconv.Itoa(b.NumLines())) + 1
}

func (b *BufPane) gutterWidth() int {
	return b.gutter
}

func (b *BufPane) bufferWidth() int {
	return b.width - b.lnumWidth() - b.gutterWidth()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
