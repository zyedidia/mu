package info

import "github.com/zyedidia/ned/pkg/tclutil"

func (ip *InfoPane) Execute() error {
	_, err := ip.interp.EvalString(string(ip.Bytes()))
	ip.interp.ClearError()
	return err
}

var commands = []tclutil.Command{
	{
		"execute",
		(*InfoPane).Execute,
		"execute: execute the current command",
	},
}
