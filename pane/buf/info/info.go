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
}

func NewInfoPane(interp *tcl.Interp, b *buffer.Buffer, msger buf.Messager, clip buf.Clipboard, cfg buf.Config) *InfoPane {
	return &InfoPane{
		BufPane: buf.NewBufPane(b, msger, clip, cfg),
		interp:  interp,
	}
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
