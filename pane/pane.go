package pane

import (
	"io"

	"github.com/zyedidia/gotcl"
	"github.com/zyedidia/ned/pkg/theme"
)

type Pane interface {
	Register(interp *gotcl.Interp)
	Unregister(interp *gotcl.Interp)
	Help(w io.Writer)
	Name() string

	Set(opt string, val interface{}) error
	Get(opt string) interface{}

	Display(draw func(x, y int, mainc rune, combc []rune, style theme.Style), cursor func(x, y int))
	Resize(w, h int)
}
