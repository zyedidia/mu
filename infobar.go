package mu

import (
	"log"

	"github.com/zyedidia/mu/buffer"
	"github.com/zyedidia/mu/pane/info"
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
	dmsg   string
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

func (i *InfoBar) DiagnosticMessage(msg string) {
	i.dmsg = msg
	log.Println("diagnostic:", msg)
}

func (i *InfoBar) ClearDiagnostic() {
	i.dmsg = ""
}

func (i *InfoBar) Error(msg string) {
	i.msg = message{msg, true}
	log.Println("error:", msg)
}

func (i *InfoBar) Clear() {
	i.msg = message{"", false}
	log.Println("clear info")
}

func (i *InfoBar) Resize(w, h int) {
	i.w, i.h = w, h
	i.cmd.Resize(w, h)
}

func (i *InfoBar) Display(draw DrawFn, cursor CursorFn) {
	msg := i.msg.data
	err := i.msg.err
	if i.dmsg != "" && !i.active {
		msg = i.dmsg
		err = false
	}

	x, y := 0, 0
	for len(msg) > 0 {
		r, combc, size := grapheme.DecodeInString(msg)
		st := i.ed.theme.Default()
		if err {
			st = i.ed.theme.Style("error")
		}
		draw(x, y, r, combc, st)
		msg = msg[size:]
		x++
	}

	if i.active {
		i.cmd.Display(func(bx, by int, mainc rune, combc []rune, style theme.Style) {
			draw(x+bx, y+by, mainc, combc, style)
		}, func(bx, by int, main bool) {
			cursor(x+bx, y+by, true)
		}, i.ed.theme)
		return
	}
}

func (i *InfoBar) Prompt(typ, msg string) (resp string, canceled bool) {
	return i.prompt(typ, msg, "", "cmd", nil)
}

func (i *InfoBar) Password(msg string) (resp string, canceled bool) {
	// password is a special type that causes history to not be saved and no
	// displaying to be disabled
	return i.prompt("password", msg, "", "cmd", nil)
}

func (i *InfoBar) IPrompt(typ, msg string, cb func(cur string)) (resp string, canceled bool) {
	return i.prompt(typ, msg, "", "cmd", cb)
}

func (i *InfoBar) CharPrompt(msg string) (resp string, canceled bool) {
	return i.prompt("charcmd", msg, "", "charcmd", nil)
}

func (i *InfoBar) prompt(typ, msg, partial, mode string, cb func(cur string)) (resp string, canceled bool) {
	i.Message(msg)
	i.cmd.BufPane.Insert(0, []byte(partial))
	i.cmd.SetType(typ)
	i.cmd.Callback = cb

	m := i.ed.GetMode()
	i.ed.MustSetMode(mode)
	i.active = true
	if i.ed.ActivePane() != nil {
		i.ed.ActivePane().Unregister(i.ed.interp)
	}
	i.cmd.Register(i.ed.interp)

	i.ed.displayLock.Unlock()
	i.ed.SendRedraw()
	r := <-i.cmd.Done
	i.ed.displayLock.Lock()

	i.ed.MustSetMode(m)
	i.cmd.Unregister(i.ed.interp)
	i.cmd.Callback = nil
	if i.ed.ActivePane() != nil {
		i.ed.ActivePane().Register(i.ed.interp)
	}
	i.Clear()
	i.active = false
	return r.Resp, r.Canceled
}
