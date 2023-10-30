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
		set p [cursor-pos]
		for { set i 0 } { $i < $n } { incr i } {
			set p [uplevel [concat $fn $p]]
		}
		return $p
	}
}
proc repeat-fn {n fn} {
	if { [== "" $n] } {
		uplevel $fn
	} else {
		for { set i 0 } { $i < $n } { incr i } {
			uplevel $fn
		}
	}
}
proc zero {} {
	return 0
}
`

func (e *Editor) Eval(cmd string, vars []interface{}) error {
	interp := gotcl.NewInterpFrom(e.interp)
	_, err := tclutil.EvalWithVars(interp, cmd, vars)
	return err
}

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
