package mu

import (
	"github.com/zyedidia/gotcl"
	"github.com/zyedidia/mu/pkg/tclutil"
)

var tclcore = `
proc repeat-move {n fn} {
	if { [== "" $n] } {
		return [uplevel [concat $fn [cursor-pos]]]
	} else {
		setvar p [cursor-pos]
		for { setvar i 0 } { $i < $n } { incr i } {
			setvar p [uplevel [concat $fn $p]]
		}
		return $p
	}
}
proc repeat-fn {n fn} {
	if { [== "" $n] } {
		uplevel $fn
	} else {
		for { setvar i 0 } { $i < $n } { incr i } {
			uplevel $fn
		}
	}
}
proc all-cursors {fn} {
	for {setvar i 0} {$i < [num-cursors]} {incr i} {
		switch-cursor $i
		uplevel $fn
	}
}
proc zero {} {
	return 0
}
`

// RunCommand runs a micro TCL expression.
func (e *Editor) RunCommand(cmd string) (string, error) {
	return e.EvalRet(cmd, nil)
}

// Eval runs a micro TCL expression with the given variables defined in the
// environment.
func (e *Editor) Eval(cmd string, vars []interface{}) error {
	interp := gotcl.NewInterpFrom(e.interp)
	_, err := tclutil.EvalWithVars(interp, cmd, vars)
	return err
}

// EvalRet is the same as Eval but returns the object returned by the TCL
// expression as a string.
func (e *Editor) EvalRet(cmd string, vars []interface{}) (string, error) {
	interp := gotcl.NewInterpFrom(e.interp)
	obj, err := tclutil.EvalWithVars(interp, cmd, vars)
	if len(e.tabs) == 0 {
		return "", ErrQuit
	}
	if obj != nil && err == nil {
		return obj.AsString(), nil
	}
	return "", err
}
