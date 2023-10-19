package mu

import "github.com/zyedidia/mu/pane"

type Tab struct {
	panes []pane.Pane
	cur   int
}
