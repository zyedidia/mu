package ned

import (
	"log"

	"github.com/micro-editor/tcell/v2"
	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/ned/buffer"
	"github.com/zyedidia/ned/pane/buf/info"
	"github.com/zyedidia/ned/pkg/grapheme"
	"github.com/zyedidia/ned/pkg/theme"
)

type Screen interface {
	PollEvent() tcell.Event
	Clear()
	Draw(x, y int, mainc rune, combc []rune, style theme.Style)
	ShowCursor(x, y int)
	Show()
}

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
	// screen Screen
}

func NewInfoBar(interp *tcl.Interp, b *buffer.Buffer, ed *Editor) *InfoBar {
	return &InfoBar{
		cmd: info.NewInfoPane(interp, b, nil, ed.termclip, ed.config),
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
		}, cursor, i.ed.theme)
		return
	}
}

func (i *InfoBar) Prompt(msg string) {
	i.Message(msg)
	log.Println(i.ed.SetMode("cmd"))
	i.active = true
	if i.ed.valid() {
		i.ed.panes[i.ed.cur].Unregister(i.ed.interp)
	}
	i.cmd.Register(i.ed.interp)
}
