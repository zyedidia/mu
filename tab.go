package mu

import (
	"github.com/zyedidia/mu/pane"
	"github.com/zyedidia/mu/pkg/theme"
	"github.com/zyedidia/mu/split"
)

type splitpane struct {
	id   uint
	pane pane.Pane
	bar  *StatusBar
}

func SplitPane(e *Editor, id uint, p pane.Pane) splitpane {
	return splitpane{
		id:   id,
		pane: p,
		bar:  NewStatusBar(e, p),
	}
}

type Tab struct {
	w, h  int
	root  *split.Node
	panes []splitpane
	cur   uint
}

func (e *Editor) newTab(p pane.Pane) *Tab {
	root := split.NewRoot(0, 0, e.w, e.h-1) // -1 for infobar
	t := &Tab{
		w:     e.w,
		h:     e.h - 1, // -1 for infobar
		root:  root,
		panes: []splitpane{SplitPane(e, root.ID(), p)},
		cur:   root.ID(),
	}
	t.Resize(t.w, t.h)
	return t
}

func (t *Tab) Resize(w, h int) {
	t.root.Resize(w, h)
	for _, p := range t.panes {
		node := t.root.GetNode(p.id)
		// make space for the divider if we are not fully wide
		nw := node.W - 1
		if node.X+node.W == w {
			nw = node.W
		}
		p.pane.Resize(nw, node.H-1) // -1 for status bar
	}
	t.w, t.h = w, h
}

func (t *Tab) ActivePane() pane.Pane {
	for _, p := range t.panes {
		if t.cur == p.id {
			return p.pane
		}
	}
	return nil
}

func (t *Tab) Display(draw DrawFn, cursor CursorFn, th *theme.Theme) {
	for _, p := range t.panes {
		n := t.root.GetNode(p.id)
		p.pane.Display(func(x, y int, mainc rune, combc []rune, style theme.Style) {
			draw(n.X+x, n.Y+y, mainc, combc, style)
		}, func(x, y int) {
			if p.id == t.cur {
				cursor(n.X+x, n.Y+y)
			}
		}, th)
		nw := n.W - 1
		if n.X+n.W == t.w {
			nw = n.W
		}
		p.bar.Display(func(x, y int, mainc rune, combc []rune, style theme.Style) {
			draw(n.X+x, n.Y+n.H+y-1, mainc, combc, style)
		}, nw, th)
		if n.W+n.X != t.w {
			divstyle := th.Default().Add(theme.AttrReverse)
			for i := 0; i < n.H; i++ {
				draw(n.X+n.W-1, n.Y+i, '|', nil, divstyle)
			}
		}
	}
}

// --- Split operations ---

func (t *Tab) VSplit(e *Editor, p pane.Pane) {
	nid := t.root.GetNode(t.cur).VSplit(true)
	t.panes = append(t.panes, SplitPane(e, nid, p))
	t.cur = nid
	e.ActivatePane(p)
	t.Resize(t.w, t.h)
}

func (t *Tab) HSplit(e *Editor, p pane.Pane) {
	nid := t.root.GetNode(t.cur).HSplit(true)
	t.panes = append(t.panes, SplitPane(e, nid, p))
	t.cur = nid
	e.ActivatePane(p)
	t.Resize(t.w, t.h)
}

func (t *Tab) idx() int {
	for i, p := range t.panes {
		if p.id == t.cur {
			return i
		}
	}
	return -1
}

func (t *Tab) next() {
	idx := t.idx()
	if idx >= len(t.panes)-1 {
		t.cur = t.panes[0].id
	} else {
		t.cur = t.panes[idx+1].id
	}
}

func (t *Tab) prev() {
	idx := t.idx()
	if idx <= 0 {
		t.cur = t.panes[len(t.panes)-1].id
	} else {
		t.cur = t.panes[idx-1].id
	}
}

func (t *Tab) Unsplit(e *Editor) {
	ok := t.root.GetNode(t.cur).Unsplit()
	if !ok {
		panic("unsplitting a non-leaf node")
	}
	i := t.idx()
	copy(t.panes[i:], t.panes[i+1:])
	t.panes[len(t.panes)-1] = splitpane{}
	t.panes = t.panes[:len(t.panes)-1]
	t.prev()
	e.ActivatePane(t.ActivePane())
	t.Resize(t.w, t.h)
}

// --- Tab operations ---

// OpenTabPane opens a new tab with 'pane' as the only pane.
func (e *Editor) OpenTabPane(pane pane.Pane) {
	t := e.newTab(pane)
	e.tabs = append(e.tabs, t)
	e.curtab = len(e.tabs) - 1
	e.ActivatePane(t.ActivePane())
}

// CloseTabPane forcibly closes the current tab and all panes within. This does not
// call pane.Close, so if panes has unsaved work they will not have an opportunity
// to prevent being closed.
func (e *Editor) CloseTabPane() {
	i := e.curtab
	copy(e.tabs[i:], e.tabs[i+1:])
	e.tabs[len(e.tabs)-1] = nil
	e.tabs = e.tabs[:len(e.tabs)-1]
	if e.curtab > 0 {
		e.curtab--
	}
	if len(e.tabs) > 0 {
		e.ActivatePane(e.ActiveTab().ActivePane())
	} else {
		e.ActivatePane(nil)
	}
}

// Open opens 'p' in the currently active pane in this tab.
func (t *Tab) Open(e *Editor, p pane.Pane) error {
	if err := e.ActivePane().Close(); err != nil {
		return err
	}
	for i, sp := range t.panes {
		if sp.id == t.cur {
			t.panes[i] = SplitPane(e, sp.id, p)
			e.ActivatePane(p)
			break
		}
	}
	return nil
}
