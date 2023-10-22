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

	typ string

	// history of responses per prompt type, 0 is oldest
	history map[string][]string
	histidx int

	Done chan InfoResp
}

func NewInfoPane(b *buffer.Buffer, msger buf.Messager, clip buf.Clipboard, cfg buf.Config, editor buf.Editor) *InfoPane {
	ip := &InfoPane{
		BufPane: buf.NewBufPaneOpts(b, msger, clip, cfg, editor, false),
		Done:    make(chan InfoResp),
		history: make(map[string][]string),
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

func (ip *InfoPane) SetType(s string) {
	ip.typ = s
	ip.histidx = len(ip.history[ip.typ])
}
