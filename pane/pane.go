package pane

import (
	"io"

	"github.com/zyedidia/gotcl"
	"github.com/zyedidia/mu/pkg/theme"
)

type Pane interface {
	Register(interp *gotcl.Interp) string
	Unregister(interp *gotcl.Interp)
	SetMode(mode string)
	Help(w io.Writer)
	Name() string

	Display(draw func(x, y int, mainc rune, combc []rune, style theme.Style), cursor func(x, y int, main bool), theme *theme.Theme)
	DisplayStatus(draw func(x, y int, mainc rune, combc []rune, style theme.Style), w int, theme *theme.Theme) bool
	Resize(w, h int)

	Status() (string, string)

	SetOpt(name string, val interface{}) error
	GetOpt(name string) (interface{}, bool)

	Close() error
	Closed() bool
}
