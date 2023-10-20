package mu

import (
	"github.com/zyedidia/mu/pane"
	"github.com/zyedidia/mu/pkg/theme"
	"github.com/zyedidia/mu/split"
)

type Tab struct {
	splits *split.Node
	panes  map[uint]pane.Pane
	cur    uint
}

func (e *Editor) NewTab(w, h int) *Tab {
	root := split.NewRoot(0, 0, w, h)
	t := &Tab{
		splits: root,
		panes: map[uint]pane.Pane{
			root.ID(): e.NewEmptyBufPane(),
		},
		cur: root.ID(),
	}
	t.Resize(w, h)
	return t
}

func (t *Tab) Resize(w, h int) {
	t.splits.Resize(w, h)
	for id, p := range t.panes {
		node := t.splits.GetNode(id)
		p.Resize(node.W, node.H)
	}
}

func (t *Tab) Active() pane.Pane {
	return t.panes[t.cur]
}

func (t *Tab) Display(draw DrawFn, cursor CursorFn, theme *theme.Theme) {
	for _, p := range t.panes {
		p.Display(draw, cursor, theme)
	}
}

// --- Split operations ---

func (t *Tab) VSplit() {

}

func (t *Tab) HSplit() {

}

func (t *Tab) Unsplit() {

}

// --- Tab operations ---

func (e *Editor) OpenTab() {

}

func (e *Editor) CloseTab() {

}
