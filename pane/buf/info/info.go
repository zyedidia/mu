package info

import (
	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/ned/buffer"
	"github.com/zyedidia/ned/pane/buf"
	"github.com/zyedidia/ned/pkg/tclutil"
)

type InfoPane struct {
	*buf.BufPane
	interp *tcl.Interp

	done func(resp string, canceled bool) error
}

func NewInfoPane(interp *tcl.Interp, b *buffer.Buffer, msger buf.Messager, clip buf.Clipboard, cfg buf.Config, eval buf.Evaluator) *InfoPane {
	return &InfoPane{
		BufPane: buf.NewBufPane(b, msger, clip, cfg, eval),
		interp:  interp,
	}
}

func (ip *InfoPane) Activate(interp *tcl.Interp, done func(resp string, canceled bool) error) {
	ip.done = done
	ip.Register(interp)
}

func (ip *InfoPane) Register(interp *tcl.Interp) {
	ip.BufPane.Register(interp)
	for _, c := range commands {
		tclutil.Register(interp, c.Name, c.Fn, ip)
	}
}

func (ip *InfoPane) Unregister(interp *tcl.Interp) {
	ip.BufPane.Unregister(interp)
	for _, c := range commands {
		tclutil.Unregister(interp, c.Name)
	}
}
