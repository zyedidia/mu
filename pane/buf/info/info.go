package info

import (
	"sync"

	"github.com/zyedidia/gotcl"
	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/ned/buffer"
	"github.com/zyedidia/ned/pane/buf"
	"github.com/zyedidia/ned/pkg/tclutil"
)

type InfoResp struct {
	Resp     string
	Canceled bool
}

type InfoPane struct {
	*buf.BufPane
	interp *tcl.Interp
	lock   sync.Mutex

	Done chan InfoResp
}

func NewInfoPane(b *buffer.Buffer, msger buf.Messager, clip buf.Clipboard, cfg buf.Config, eval buf.CmdRunner) *InfoPane {
	interp := gotcl.NewInterp()
	ip := &InfoPane{
		BufPane: buf.NewBufPaneOpts(b, msger, clip, cfg, eval, false),
		interp:  interp,
		Done:    make(chan InfoResp),
	}
	ip.Register(interp)
	return ip
}

func (ip *InfoPane) Eval(cmd string, vars []interface{}) error {
	ip.lock.Lock()
	defer ip.lock.Unlock()
	interp := gotcl.NewInterpFrom(ip.interp)
	err := tclutil.EvalWithVars(interp, cmd, vars)
	if err != nil {
		return ip.BufPane.Eval(cmd, vars)
	}
	return err
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
