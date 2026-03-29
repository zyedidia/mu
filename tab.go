package main

import (
	"fmt"
)

// Tab represents one editor tab containing a split tree of view panes.
type Tab struct {
	root  *SplitNode
	panes map[uint]*View // leaf ID → view
	cur   uint           // active pane ID
	w, h  int
}

// NewTab creates a new tab with a single view pane.
func NewTab(v *View, w, h int) *Tab {
	root := NewSplitRoot(w, h)
	t := &Tab{
		root:  root,
		panes: map[uint]*View{root.ID(): v},
		cur:   root.ID(),
		w:     w,
		h:     h,
	}
	v.Resize(w, h)
	return t
}

// ActiveView returns the currently focused view.
func (t *Tab) ActiveView() *View {
	return t.panes[t.cur]
}

// Resize resizes the tab and propagates to the split tree and all panes.
func (t *Tab) Resize(w, h int) {
	t.w = w
	t.h = h
	t.root.Resize(w, h)
	t.root.EachLeaf(func(leaf *SplitNode) {
		if v, ok := t.panes[leaf.id]; ok {
			v.Resize(leaf.W, leaf.H)
		}
	})
}

// VSplit splits the active pane vertically and inserts the new view on the
// right.
func (t *Tab) VSplit(v *View) {
	node := t.root.GetNode(t.cur)
	if node == nil {
		return
	}
	newID := node.VSplit()
	if newID == 0 {
		return
	}
	t.panes[newID] = v
	t.cur = newID
	t.Resize(t.w, t.h)
}

// HSplit splits the active pane horizontally and inserts the new view on
// the bottom.
func (t *Tab) HSplit(v *View) {
	node := t.root.GetNode(t.cur)
	if node == nil {
		return
	}
	newID := node.HSplit()
	if newID == 0 {
		return
	}
	t.panes[newID] = v
	t.cur = newID
	t.Resize(t.w, t.h)
}

// Unsplit removes the active pane. Returns false if it's the last pane.
func (t *Tab) Unsplit() bool {
	if len(t.panes) <= 1 {
		return false
	}
	node := t.root.GetNode(t.cur)
	if node == nil {
		return false
	}
	delete(t.panes, t.cur)
	newID := node.Unsplit()
	t.cur = newID
	t.Resize(t.w, t.h)
	return true
}

// NextPane cycles to the next pane, saving/restoring cursors.
func (t *Tab) NextPane() {
	if v := t.panes[t.cur]; v != nil {
		v.Deactivate()
	}
	t.cur = t.root.NextLeaf(t.cur)
	if v := t.panes[t.cur]; v != nil {
		v.Activate()
	}
}

// PrevPane cycles to the previous pane, saving/restoring cursors.
func (t *Tab) PrevPane() {
	if v := t.panes[t.cur]; v != nil {
		v.Deactivate()
	}
	t.cur = t.root.PrevLeaf(t.cur)
	if v := t.panes[t.cur]; v != nil {
		v.Activate()
	}
}

// SwitchTo switches focus to the pane with the given ID.
func (t *Tab) SwitchTo(id uint) {
	if id == t.cur {
		return
	}
	if _, ok := t.panes[id]; !ok {
		return
	}
	if v := t.panes[t.cur]; v != nil {
		v.Deactivate()
	}
	t.cur = id
	if v := t.panes[t.cur]; v != nil {
		v.Activate()
	}
}

// FocusLeft switches to the nearest pane to the left.
func (t *Tab) FocusLeft()  { t.SwitchTo(t.root.NeighborLeft(t.cur)) }
func (t *Tab) FocusRight() { t.SwitchTo(t.root.NeighborRight(t.cur)) }
func (t *Tab) FocusUp()    { t.SwitchTo(t.root.NeighborUp(t.cur)) }
func (t *Tab) FocusDown()  { t.SwitchTo(t.root.NeighborDown(t.cur)) }

// NumPanes returns the number of panes.
func (t *Tab) NumPanes() int {
	return len(t.panes)
}

// Display renders all panes in the tab, including dividers and status bars.
// modeName is the current vim mode name for the status bar.
func (t *Tab) Display(draw DrawFunc, showCursor CursorFunc, th *Theme, modeName string) {
	t.root.EachLeaf(func(leaf *SplitNode) {
		v, ok := t.panes[leaf.id]
		if !ok {
			return
		}
		isCurrent := leaf.id == t.cur

		// Render the view offset by the leaf's position.
		// Reserve 1 row for status bar.
		viewH := leaf.H - 1
		if viewH < 1 {
			viewH = 1
		}
		v.Resize(leaf.W, viewH)

		// For inactive panes, temporarily swap in this view's saved
		// cursor so Relocate and Display use the right position.
		var savedCursor Cursor
		if !isCurrent {
			savedCursor = *v.buf.Cursor()
			*v.buf.Cursor() = v.savedCursor
		}

		v.Relocate()
		v.buf.SyntaxCheckWindow(v.buf.Cursor().Pos)

		v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
			draw(leaf.X+x, leaf.Y+y, mainc, combc, style)
		}, func(x, y int, main bool) {
			if isCurrent && main {
				showCursor(leaf.X+x, leaf.Y+y, true)
			}
		}, th, isCurrent)

		// Draw status bar at the bottom of this pane.
		drawPaneStatusBar(draw, v, leaf, th, modeName, isCurrent)

		// Restore the active cursor.
		if !isCurrent {
			*v.buf.Cursor() = savedCursor
		}
	})

	// Draw dividers.
	t.drawDividers(draw, th)
}

// drawPaneStatusBar renders a status bar at the bottom row of a leaf node.
func drawPaneStatusBar(draw DrawFunc, v *View, leaf *SplitNode, th *Theme, modeName string, isCurrent bool) {
	y := leaf.Y + leaf.H - 1
	style := th.Style("statusline")

	b := v.buf
	name := b.Path
	if name == "" {
		name = "[No Name]"
	}
	flags := ""
	if b.Modified() {
		flags += " [+]"
	}
	if b.readonly {
		flags += " [RO]"
	}

	// Left: mode | filename [flags]
	left := fmt.Sprintf(" %s%s ", name, flags)
	if isCurrent {
		left = fmt.Sprintf(" %s | %s%s ", modeName, name, flags)
	}

	// Right: endings | line:col
	line, col := b.LineColAt(b.Cursor().Pos)
	endings := ""
	if b.Text().Opts.Endings != nil {
		endings = b.Text().Opts.Endings.String() + " | "
	}
	right := fmt.Sprintf(" %s%d:%d ", endings, line+1, col+1)

	x := 0
	for _, r := range left {
		if x >= leaf.W {
			break
		}
		draw(leaf.X+x, y, r, nil, style)
		x++
	}
	rightStart := leaf.W - len([]rune(right))
	if rightStart < x {
		rightStart = x
	}
	for x < rightStart {
		draw(leaf.X+x, y, ' ', nil, style)
		x++
	}
	for _, r := range right {
		if x >= leaf.W {
			break
		}
		draw(leaf.X+x, y, r, nil, style)
		x++
	}
}

// drawDividers renders vertical and horizontal dividers between split panes.
func (t *Tab) drawDividers(draw DrawFunc, th *Theme) {
	style := th.Style("statusline")
	t.root.EachLeaf(func(leaf *SplitNode) {
		// Vertical divider to the left of right-side panes.
		if leaf.HasDividerLeft() {
			x := leaf.X - 1
			for y := leaf.Y; y < leaf.Y+leaf.H; y++ {
				draw(x, y, '│', nil, style)
			}
		}
		// Horizontal divider above bottom panes.
		if leaf.HasDividerAbove() {
			y := leaf.Y - 1
			for x := leaf.X; x < leaf.X+leaf.W; x++ {
				draw(x, y, '─', nil, style)
			}
		}
	})
}
