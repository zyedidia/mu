package mu

import (
	"errors"
	"reflect"

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
`

var errInterface reflect.Type

func init() {
	errInterface = reflect.TypeOf((*error)(nil)).Elem()
}

var ErrQuit = errors.New("quit")

func (e *Editor) Eval(cmd string, vars []interface{}) error {
	interp := gotcl.NewInterpFrom(e.interp)
	_, err := tclutil.EvalWithVars(interp, cmd, vars)
	if len(e.panes) == 0 {
		return ErrQuit
	}
	return err
}

func (e *Editor) EvalRet(cmd string, vars []interface{}) (string, error) {
	interp := gotcl.NewInterpFrom(e.interp)
	obj, err := tclutil.EvalWithVars(interp, cmd, vars)
	if len(e.panes) == 0 {
		return "", ErrQuit
	}
	if obj != nil && err == nil {
		return obj.AsString(), nil
	}
	return "", err
}
