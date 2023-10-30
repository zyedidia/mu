package tclutil

import (
	"fmt"
	"reflect"
	"strconv"
	"unicode/utf8"

	tcl "github.com/zyedidia/gotcl"
)

type Command struct {
	Name string
	Fn   interface{}
	Doc  string
	Pre  func() error
}

var errInterface reflect.Type

func init() {
	errInterface = reflect.TypeOf((*error)(nil)).Elem()
}

func Unregister(interp *tcl.Interp, name string) {
	interp.SetCmd(name, nil)
}

func Register(interp *tcl.Interp, name string, fn, arg0 interface{}, pre func() error, post func()) {
	v := reflect.ValueOf(fn)
	t := v.Type()

	cmd := func(itp *tcl.Interp, args []*tcl.TclObj) tcl.TclStatus {
		if pre != nil {
			if err := pre(); err != nil {
				return itp.Fail(err)
			}
		}

		argv := make([]reflect.Value, 0, len(args))
		argv = append(argv, reflect.ValueOf(arg0))

		var ret []reflect.Value
		if t.NumIn() == 2 && t.In(1).Kind() == reflect.Slice && t.In(1).Elem().Kind() == reflect.String {
			slice := make([]string, 0, len(args))
			for i := range args {
				slice = append(slice, args[i].AsString())
			}
			argv = append(argv, reflect.ValueOf(slice))
			ret = v.Call(argv)
		} else {
			if len(args) != t.NumIn()-1 {
				return itp.Fail(fmt.Errorf("invalid number of arguments. got: %v, want %v", len(args), t.NumIn()-1))
			}

			for i := range args {
				switch t.In(i + 1).Kind() {
				case reflect.String:
					argv = append(argv, reflect.ValueOf(args[i].AsString()))
				case reflect.Int:
					num, err := args[i].AsInt()
					if err != nil {
						return itp.Fail(fmt.Errorf("expected 'int' for argument %d (parse error %w)", i+1, err))
					}
					argv = append(argv, reflect.ValueOf(num))
				case reflect.Int32: // rune
					// ignore size
					r, _ := utf8.DecodeRuneInString(args[i].AsString())
					argv = append(argv, reflect.ValueOf(r))
				}
			}
			ret = v.Call(argv)
		}

		if post != nil {
			defer post()
		}

		if len(ret) == 0 {
			return 0
		} else if len(ret) == 1 {
			switch ret[0].Kind() {
			case reflect.Interface:
				if ret[0].Type().Implements(errInterface) && !ret[0].IsNil() {
					return itp.FailStr(fmt.Sprintf("%v", ret[0]))
				}
				return itp.Return(tcl.FromStr(""))
			case reflect.Bool:
				return itp.Return(tcl.FromBool(ret[0].Bool()))
			case reflect.Int:
				return itp.Return(tcl.FromInt(int(ret[0].Int())))
			case reflect.String:
				return itp.Return(tcl.FromStr(ret[0].String()))
			case reflect.Slice:
				switch ret[0].Type().Elem().Kind() {
				case reflect.Int:
					return itp.Return(tcl.FromIntList(ret[0].Interface().([]int)))
				case reflect.String:
					return itp.Return(tcl.FromList(ret[0].Interface().([]string)))
				}
			}
		} else if len(ret) == 2 {
			switch ret[1].Kind() {
			case reflect.Interface:
				if ret[1].Type().Implements(errInterface) && !ret[1].IsNil() {
					return itp.FailStr(fmt.Sprintf("%v", ret[1]))
				}
			}
			switch ret[0].Kind() {
			case reflect.Int:
				return itp.Return(tcl.FromInt(int(ret[0].Int())))
			case reflect.String:
				return itp.Return(tcl.FromStr(ret[0].String()))
			case reflect.Slice:
				switch ret[0].Type().Elem().Kind() {
				case reflect.Int:
					return itp.Return(tcl.FromIntList(ret[0].Interface().([]int)))
				case reflect.String:
					return itp.Return(tcl.FromList(ret[0].Interface().([]string)))
				}
			}
		}
		return 0
	}
	interp.SetCmd(name, cmd)
}

func Eval(interp *tcl.Interp, cmd string) (*tcl.TclObj, error) {
	obj, err := interp.EvalString(cmd)
	interp.ClearError()
	return obj, err
}

func EvalWithVars(interp *tcl.Interp, cmd string, vars []interface{}) (*tcl.TclObj, error) {
	for i, v := range vars {
		name := strconv.Itoa(i)
		switch v := v.(type) {
		case string:
			interp.SetVarRaw(name, tcl.FromStr(v))
		case int:
			interp.SetVarRaw(name, tcl.FromInt(v))
		}
	}
	return Eval(interp, cmd)
}
