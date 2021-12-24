package pane

import (
	"github.com/zyedidia/gotcl"
)

type Pane interface {
	Register(interp *gotcl.Interp)
	Unregister(interp *gotcl.Interp)
	Name() string
	// Display(draw func(x, y int, mainc rune, combc []rune, style theme.Style), w, h int)
}
