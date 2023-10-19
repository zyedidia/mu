package buf

import (
	"github.com/zyedidia/mu/buffer"
)

// TODO: need virtual cursors to handle visual x
func (bp *BufPane) CursorUp(c buffer.Cursor) buffer.Cursor {
	c = c.Deselect(0)
	line, _ := bp.LineColAt(c.Pos)
	c.Pos = bp.VisualLoc(line-1, c.Vx, bp.vis)
	bp.vertical = true
	return c
}

func (bp *BufPane) CursorDown(c buffer.Cursor) buffer.Cursor {
	c.Deselect(1)
	line, _ := bp.LineColAt(c.Pos)
	c.Pos = bp.VisualLoc(line+1, c.Vx, bp.vis)
	bp.vertical = true
	return c
}

func (bp *BufPane) RecalcVX(c *buffer.Cursor) {
	line, col := bp.Buffer.LineColAt(c.Pos)
	vl := bp.bLoc2vLoc(bLoc{line: line, col: col})
	c.Vx = vl.col
}
