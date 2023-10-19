package ned

import (
	"log"

	"github.com/zyedidia/ned/buffer"
	"github.com/zyedidia/ned/pane/buf/info"
	"github.com/zyedidia/ned/pkg/grapheme"
	"github.com/zyedidia/ned/pkg/theme"
)

type message struct {
	data string
	err  bool
}

type InfoBar struct {
	w, h int

	msg    message
	cmd    *info.InfoPane
	active bool
	ed     *Editor
}

func NewInfoBar(b *buffer.Buffer, ed *Editor) *InfoBar {
	return &InfoBar{
		cmd: info.NewInfoPane(b, nil, ed.termclip, ed.config, ed),
		ed:  ed,
	}
}

func (i *InfoBar) Message(msg string) {
	i.msg = message{msg, false}
	log.Println("info:", msg)
}

func (i *InfoBar) Error(msg string) {
	i.msg = message{msg, true}
	log.Println("error:", msg)
}

func (i *InfoBar) Clear() {
	i.msg = message{"", false}
}

// func (i *InfoBar) Prompt(msg string, update func(user string)) (string, bool) {
// 	return "", false
// }

func (i *InfoBar) Resize(w, h int) {
	i.w, i.h = w, h
	i.cmd.Resize(w, h)
}

func (i *InfoBar) Display(draw func(x, y int, mainc rune, combc []rune, style theme.Style), cursor func(x, y int)) {
	msg := i.msg.data
	x, y := 0, 0
	for len(msg) > 0 {
		r, combc, size := grapheme.DecodeInString(msg)
		st := i.ed.theme.Default()
		if i.msg.err {
			st = i.ed.theme.Style("error")
		}
		draw(x, y, r, combc, st)
		msg = msg[size:]
		x++
	}

	if i.active {
		i.cmd.Display(func(bx, by int, mainc rune, combc []rune, style theme.Style) {
			draw(x+bx, y+by, mainc, combc, style)
		}, func(bx, by int) {
			cursor(x+bx, y+by)
		}, i.ed.theme)
		return
	}
}

func (i *InfoBar) Prompt(msg string) (resp string, canceled bool) {
	i.Message(msg)
	m := i.ed.GetMode()
	i.ed.SetMode("cmd")
	i.active = true
	i.ed.SendRedraw()
	r := <-i.cmd.Done
	i.ed.displayLock.Lock()
	i.ed.SetMode(m)
	i.Clear()
	i.active = false
	return r.Resp, r.Canceled
	// i.cmd.Activate(i.ed.interp, func(resp string, canceled bool) error {
	// 	i.cmd.Unregister(i.ed.interp)
	// 	i.active = false
	// 	i.ed.panes[i.ed.cur].Register(i.ed.interp)
	// 	i.ed.SetMode(m)
	// 	i.Clear()
	// 	return done(resp, canceled)
	// })
}

func (i *InfoBar) CharPrompt(msg string, done func(resp string, canceled bool) error) {
	// i.Message(msg)
	// m := i.ed.GetMode()
	// i.ed.SetMode("charcmd")
	// i.active = true
	// if i.ed.valid() {
	// 	i.ed.panes[i.ed.cur].Unregister(i.ed.interp)
	// }
	// i.cmd.Activate(i.ed.interp, func(resp string, canceled bool) error {
	// 	i.cmd.Unregister(i.ed.interp)
	// 	i.active = false
	// 	i.ed.panes[i.ed.cur].Register(i.ed.interp)
	// 	i.ed.SetMode(m)
	// 	i.Clear()
	// 	return done(resp, canceled)
	// })
}
