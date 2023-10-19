package mu

import (
	"log"

	"github.com/zyedidia/mu/buffer"
	"github.com/zyedidia/mu/pane/buf/info"
	"github.com/zyedidia/mu/pkg/grapheme"
	"github.com/zyedidia/mu/pkg/theme"
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
	return i.prompt(msg, "cmd")
}

func (i *InfoBar) CharPrompt(msg string) (resp string, canceled bool) {
	return i.prompt(msg, "charcmd")
}

func (i *InfoBar) prompt(msg, mode string) (resp string, canceled bool) {
	i.Message(msg)
	m := i.ed.GetMode()
	i.ed.MustSetMode(mode)
	i.active = true
	if i.ed.valid() {
		i.ed.Active().Unregister(i.ed.interp)
	}
	i.cmd.Register(i.ed.interp)
	i.ed.SendRedraw()
	i.ed.displayLock.Unlock()
	r := <-i.cmd.Done
	i.ed.displayLock.Lock()
	i.ed.MustSetMode(m)
	i.cmd.Unregister(i.ed.interp)
	i.ed.Active().Register(i.ed.interp)
	i.Clear()
	i.active = false
	return r.Resp, r.Canceled
}
