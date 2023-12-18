package info

import (
	"github.com/zyedidia/mu/pkg/tclutil"
)

func (ip *InfoPane) PostEvent() {
	if ip.Callback != nil {
		ip.Callback(string(ip.Bytes()))
	}
}

func (ip *InfoPane) Execute() {
	text := string(ip.Bytes())
	ip.BufPane.Remove(0, ip.BufPane.Len())
	ip.history[ip.typ] = append(ip.history[ip.typ], text)
	ip.Callback = nil // make sure callback doesn't get called afterwards
	ip.Done <- InfoResp{text, false}
}

func (ip *InfoPane) Cancel() {
	text := string(ip.Bytes())
	ip.BufPane.Remove(0, ip.BufPane.Len())
	ip.Callback = nil // make sure callback doesn't get called afterwards
	ip.Done <- InfoResp{text, true}
}

func (ip *InfoPane) EnterChar(char rune) {
	ip.Done <- InfoResp{string(char), false}
}

func (ip *InfoPane) HistoryPrev() {
	if ip.histidx > 0 && ip.histidx <= len(ip.history[ip.typ]) {
		ip.histidx--
		ip.BufPane.Remove(0, ip.BufPane.Len())
		ip.BufPane.Insert(0, []byte(ip.history[ip.typ][ip.histidx]))
	}
}

func (ip *InfoPane) HistoryNext() {
	if ip.histidx >= 0 && ip.histidx < len(ip.history[ip.typ])-1 {
		ip.histidx++
		ip.BufPane.Remove(0, ip.BufPane.Len())
		ip.BufPane.Insert(0, []byte(ip.history[ip.typ][ip.histidx]))
	} else {
		ip.histidx++
		ip.BufPane.Remove(0, ip.BufPane.Len())
	}
}

var commands = []tclutil.Command{
	{
		Name: "history-prev",
		Fn:   (*InfoPane).HistoryPrev,
		Doc:  "history-prev: load previous response",
	},
	{
		Name: "history-next",
		Fn:   (*InfoPane).HistoryNext,
		Doc:  "history-prev: load next response",
	},
	{
		Name: "execute",
		Fn:   (*InfoPane).Execute,
		Doc:  "execute: return the current text as the prompt response",
	},
	{
		Name: "cancel",
		Fn:   (*InfoPane).Cancel,
		Doc:  "cancel: cancel the current prompt",
	},
	{
		Name: "enter-char",
		Fn:   (*InfoPane).EnterChar,
		Doc:  "enter-char: enter a single character as the prompt response",
	},
}
