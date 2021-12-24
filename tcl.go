package ned

import (
	"errors"
	"reflect"
	"strconv"

	tcl "github.com/zyedidia/gotcl"
)

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
