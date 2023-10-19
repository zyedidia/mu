package info

import (
	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/mu/buffer"
	"github.com/zyedidia/mu/pane/buf"
	"github.com/zyedidia/mu/pkg/tclutil"
)

type InfoResp struct {
	Resp     string
	Canceled bool
}

type InfoPane struct {
	*buf.BufPane

	Done chan InfoResp
}

func NewInfoPane(b *buffer.Buffer, msger buf.Messager, clip buf.Clipboard, cfg buf.Config, eval buf.Evaluator) *InfoPane {
	ip := &InfoPane{
		BufPane: buf.NewBufPaneOpts(b, msger, clip, cfg, eval, false),
		Done:    make(chan InfoResp),
	}
	return ip
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
