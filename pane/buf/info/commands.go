package info

import (
	"log"

	"github.com/zyedidia/mu/pkg/tclutil"
)

func (ip *InfoPane) Execute() {
	text := string(ip.Bytes())
	ip.BufPane.Remove(0, ip.BufPane.Len())
	ip.history = append(ip.history, text)
	ip.histidx = len(ip.history)
	ip.Done <- InfoResp{text, false}
}

func (ip *InfoPane) Cancel() {
	text := string(ip.Bytes())
	ip.BufPane.Remove(0, ip.BufPane.Len())
	ip.history = append(ip.history, text)
	ip.histidx = len(ip.history)
	ip.Done <- InfoResp{text, true}
}

func (ip *InfoPane) EnterChar(char rune) {
	ip.Done <- InfoResp{string(char), false}
}

func (ip *InfoPane) HistoryPrev() {
	log.Println("history prev", ip.histidx, len(ip.history))
	if ip.histidx > 0 && ip.histidx <= len(ip.history) {
		ip.histidx--
		ip.BufPane.Remove(0, ip.BufPane.Len())
		ip.BufPane.Insert(0, []byte(ip.history[ip.histidx]))
	}
}

func (ip *InfoPane) HistoryNext() {
	if ip.histidx >= 0 && ip.histidx < len(ip.history)-1 {
		ip.histidx++
		ip.BufPane.Remove(0, ip.BufPane.Len())
		ip.BufPane.Insert(0, []byte(ip.history[ip.histidx]))
	} else {
		ip.histidx++
		ip.BufPane.Remove(0, ip.BufPane.Len())
	}
}

var commands = []tclutil.Command{
	{
		"history-prev",
		(*InfoPane).HistoryPrev,
		"history-prev: load previous response",
	},
	{
		"history-next",
		(*InfoPane).HistoryNext,
		"history-prev: load next response",
	},
	{
		"execute",
		(*InfoPane).Execute,
		"execute: return the current text as the prompt response",
	},
	{
		"cancel",
		(*InfoPane).Cancel,
		"cancel: cancel the current prompt",
	},
	{
		"enter-char",
		(*InfoPane).EnterChar,
		"enter-char: enter a single character as the prompt response",
	},
}
