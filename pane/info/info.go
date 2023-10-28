package info

import (
	"encoding/gob"

	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/mu/buffer"
	"github.com/zyedidia/mu/config"
	"github.com/zyedidia/mu/pane/buf"
	"github.com/zyedidia/mu/pkg/tclutil"
	"github.com/zyedidia/mu/pkg/theme"
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

	Callback func(cur string)
	Done     chan InfoResp
}

func NewInfoPane(b *buffer.Buffer, msger buf.Messager, clip buf.Clipboard, cfg buf.Config, editor buf.Editor) *InfoPane {
	ip := &InfoPane{
		BufPane: buf.NewBufPaneOpts(b, msger, clip, cfg, editor, false, 0),
		Done:    make(chan InfoResp),
		history: loadHistory(cfg.CacheFS()),
	}
	return ip
}

func (ip *InfoPane) Register(interp *tcl.Interp) {
	ip.BufPane.Register(interp)
	for _, c := range commands {
		tclutil.Register(interp, c.Name, c.Fn, ip, c.Pre, nil)
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

const histfile = "history.dat"

func (ip *InfoPane) SerializeHistory(cfg buffer.Config) error {
	delete(ip.history, "password")
	f, err := cfg.CacheFS().Create(histfile)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := gob.NewEncoder(f)
	return enc.Encode(ip.history)
}

func loadHistory(fs config.WriteFS) (hist map[string][]string) {
	f, err := fs.Open(histfile)
	if err != nil {
		return map[string][]string{}
	}
	defer f.Close()
	dec := gob.NewDecoder(f)
	err = dec.Decode(&hist)
	if err != nil {
		return map[string][]string{}
	}
	return hist
}

func (ip *InfoPane) Display(draw func(vx, vy int, mainc rune, combc []rune, style theme.Style), showCursor func(x, y int), th *theme.Theme) {
	if ip.typ == "password" {
		showCursor(0, 0)
		return
	}
	ip.BufPane.Display(draw, showCursor, th)
}
