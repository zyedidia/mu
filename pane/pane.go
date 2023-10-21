package pane

import (
	"io"

	"github.com/zyedidia/gotcl"
	"github.com/zyedidia/mu/pkg/theme"
)

type Pane interface {
	Register(interp *gotcl.Interp)
	Unregister(interp *gotcl.Interp)
	Help(w io.Writer)
	Name() string

	Display(draw func(x, y int, mainc rune, combc []rune, style theme.Style), cursor func(x, y int), theme *theme.Theme)
	Resize(w, h int)

	EvalRet(cmd string, vars []interface{}) (string, error)

	SetOpt(name string, val interface{}) error
	GetOpt(name string) (interface{}, bool)

	Close() error
}
