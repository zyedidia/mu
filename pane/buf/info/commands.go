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

var commands = []tclutil.Command{
	{
		"execute",
		(*InfoPane).Execute,
		"execute: execute the current command",
	},
	{
		"cancel",
		(*InfoPane).Cancel,
		"cancel: cancel the current command",
	},
}
