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
	// Save the current pane's cursor before splitting.
	if cur := t.panes[t.cur]; cur != nil {
		cur.Deactivate()
	}
	newID := node.VSplit()
	if newID == 0 {
		return
	}
	t.panes[newID] = v
	t.cur = newID
	v.Activate()
	t.Resize(t.w, t.h)
}

// HSplit splits the active pane horizontally and inserts the new view on
// the bottom.
func (t *Tab) HSplit(v *View) {
	node := t.root.GetNode(t.cur)
	if node == nil {
		return
	}
	if cur := t.panes[t.cur]; cur != nil {
		cur.Deactivate()
	}
	newID := node.HSplit()
	if newID == 0 {
		return
	}
	t.panes[newID] = v
	t.cur = newID
	v.Activate()
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
	// Restore the surviving pane's own cursor: with a shared buffer it
	// would otherwise be left wherever the closed pane was.
	if v := t.panes[t.cur]; v != nil {
		v.Activate()
	}
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

	// Right: filetype | endings | line:col | pct
	line, col := b.LineColAt(b.Cursor().Pos)
	endings := ""
	if b.Text().Opts.Endings != nil {
		endings = b.Text().Opts.Endings.String() + " | "
	}
	ft := ""
	if b.Filetype != "" {
		ft = b.Filetype + " | "
	}
	numLines := b.NumLines() + 1
	pct := "Top"
	if numLines <= 1 {
		pct = "All"
	} else if line+1 >= numLines {
		pct = "Bot"
	} else if line == 0 {
		pct = "Top"
	} else {
		pct = fmt.Sprintf("%d%%", (line+1)*100/numLines)
	}
	right := fmt.Sprintf(" %s%s%d:%d %s ", ft, endings, line+1, col+1, pct)

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

// drawDividers renders vertical dividers between vertically-split panes.
// Horizontal dividers are not drawn — the top pane's status bar acts as
// the visual separator.
func (t *Tab) drawDividers(draw DrawFunc, th *Theme) {
	style := th.Style("statusline")
	drawVertDividers(t.root, draw, style)
}

// drawVertDividers recursively draws the vertical divider column for each
// SplitVert node.
func drawVertDividers(n *SplitNode, draw DrawFunc, style Style) {
	if n.IsLeaf() {
		return
	}
	if n.Kind == SplitVert {
		// The divider column is between children[0] and children[1].
		x := n.children[0].X + n.children[0].W
		for y := n.Y; y < n.Y+n.H; y++ {
			draw(x, y, '│', nil, style)
		}
	}
	drawVertDividers(n.children[0], draw, style)
	drawVertDividers(n.children[1], draw, style)
}
