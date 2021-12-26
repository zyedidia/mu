package ned

import (
	"errors"
	"reflect"
	"strconv"

	tcl "github.com/zyedidia/gotcl"
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

func (e *Editor) Eval(cmd string) error {
	_, err := e.interp.EvalString(cmd)
	e.interp.ClearError()

	if len(e.panes) == 0 {
		return ErrQuit
	}

	return err
}

func (e *Editor) EvalWithVars(cmd string, vars []interface{}) error {
	for i, v := range vars {
		name := strconv.Itoa(i)
		switch v := v.(type) {
		case string:
			e.interp.SetVarRaw(name, tcl.FromStr(v))
		case int:
			e.interp.SetVarRaw(name, tcl.FromInt(v))
		}
	}
	return e.Eval(cmd)
}
