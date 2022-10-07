package info

import "github.com/zyedidia/ned/pkg/tclutil"

func (ip *InfoPane) Execute() error {
	text := string(ip.Bytes())
	ip.BufPane.Remove(0, ip.BufPane.Len())
	return ip.done(text, false)
}

var commands = []tclutil.Command{
	{
		"execute",
		(*InfoPane).Execute,
		"execute: execute the current command",
	},
}
