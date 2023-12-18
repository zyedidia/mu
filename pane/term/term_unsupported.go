//go:build !linux && !darwin && !freebsd && !dragonfly && !openbsd_amd64

package term

import (
	"errors"
	"io"

	"github.com/zyedidia/gotcl"
	"github.com/zyedidia/mu/pkg/theme"
)

type TermPane struct{}

var ErrUnsupported = errors.New("unsupported system")

func (tp *TermPane) Resize(w, h int) {}

func (tp *TermPane) Status() (string, string) {
	return "", ""
}

func (tp *TermPane) SetOpt(name string, val interface{}) error {
	return ErrUnsupported
}
func (tp *TermPane) GetOpt(name string) (interface{}, bool) {
	return nil, false
}
func (tp *TermPane) Close() error {
	return nil
}
func (tp *TermPane) Closed() bool {
	return true
}

func (tp *TermPane) Register(interp *gotcl.Interp) string {
	return ""
}
func (tp *TermPane) Unregister(interp *gotcl.Interp) {}
func (tp *TermPane) SetMode(mode string)             {}
func (tp *TermPane) Help(w io.Writer)                {}
func (tp *TermPane) Name() string {
	return "term unsupported"
}

func (tp *TermPane) Display(draw func(x, y int, mainc rune, combc []rune, style theme.Style), cursor func(x, y int, main bool), theme *theme.Theme) {
	return
}
