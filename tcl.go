package ned

import (
	"errors"
	"reflect"

	"github.com/zyedidia/gotcl"
	"github.com/zyedidia/ned/pkg/tclutil"
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
	e.evalLock.Lock()
	defer e.evalLock.Unlock()
	interp := gotcl.NewInterpFrom(e.interp)
	return tclutil.EvalWithVars(interp, cmd, vars)
}
