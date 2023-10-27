package buf

import (
	"github.com/zyedidia/mu/pkg/expand"
	"github.com/zyedidia/mu/pkg/tclutil"
)

const (
	defLeft  = "{{keyword}}$name $modified($(cursor-line),$(cursor-col)) | ft:$filetype"
	defRight = "mu $version"
)

func (bp *BufPane) evalStatus(expr string) (string, error) {
	obj, err := tclutil.EvalWithVars(bp.status, expr, nil)
	if obj != nil && err == nil {
		return obj.AsString(), nil
	}
	return "", err
}

func (bp *BufPane) Status() (string, string) {
	resolve := func(expr string) (string, error) {
		s, err := bp.evalStatus(expr)
		if err != nil {
			return bp.editor.EvalRet(expr, nil)
		}
		return s, nil
	}
	left, _ := expand.Expand(defLeft, resolve, resolve)
	right, _ := expand.Expand(defRight, resolve, resolve)

	return left, right
}
