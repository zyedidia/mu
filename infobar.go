package ned

import (
	"log"

	"github.com/micro-editor/tcell/v2"
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

	msg message
	// screen Screen
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

func (i *InfoBar) Prompt(msg string, update func(user string)) (string, bool) {
	return "", false
}

func (i *InfoBar) Resize(w, h int) {
	i.w, i.h = w, h
}

func (i *InfoBar) Display(draw func(x, y int, mainc rune, combc []rune, err bool), cursor func(x, y int)) {
	msg := i.msg.data
	x, y := 0, 0
	for len(msg) > 0 {
		r, combc, size := grapheme.DecodeInString(msg)
		draw(x, y, r, combc, i.msg.err)
		msg = msg[size:]
		x++
	}
}
