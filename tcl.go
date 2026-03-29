package main

import (
	"github.com/zyedidia/gotcl"
)

// initTCL sets up the TCL interpreter and registers all editor commands.
func (e *Editor) initTCL() {
	e.interp = gotcl.NewInterp()

	for _, cmd := range editorCommands {
		cmd := cmd // capture
		e.interp.SetCmd(cmd.Name, func(interp *gotcl.Interp, args []*gotcl.TclObj) gotcl.TclStatus {
			strs := make([]string, len(args))
			for i, a := range args {
				strs[i] = a.AsString()
			}
			err := cmd.Fn(e, strs)
			if err != nil {
				return interp.FailStr(err.Error())
			}
			return interp.Return(nil)
		})
	}
}

// EvalTCL evaluates a TCL string in the editor's interpreter.
func (e *Editor) EvalTCL(s string) error {
	_, err := e.interp.EvalString(s)
	return err
}
