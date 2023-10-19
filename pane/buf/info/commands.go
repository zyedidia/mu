package info

import "github.com/zyedidia/ned/pkg/tclutil"

func (ip *InfoPane) Execute() error {
	text := string(ip.Bytes())
	ip.BufPane.Remove(0, ip.BufPane.Len())
	return ip.done(text, false)
}

func (ip *InfoPane) Cancel() error {
	text := string(ip.Bytes())
	ip.BufPane.Remove(0, ip.BufPane.Len())
	return ip.done(text, true)
}

func (ip *InfoPane) EnterChar(char rune) error {
	return ip.done(string(char), false)
}

var commands = []tclutil.Command{
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
