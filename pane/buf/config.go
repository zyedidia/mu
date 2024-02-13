package buf

import (
	"github.com/zyedidia/mu/buffer"
)

type Config interface {
	buffer.Config
}

func (bp *BufPane) SetOpt(opt string, val interface{}) error {
	err := bp.Buffer.SetOpt(opt, val)
	if err != nil {
		return err
	}
	switch opt {
	case "linenums":
		bp.linenums = val.(bool)
	case "scrollmargin":
		bp.scrollmargin = int(val.(int64))
	case "hscrollmargin":
		bp.hscrollmargin = int(val.(int64))
	case "softwrap":
		bp.softwrap = val.(bool)
	case "wordwrap":
		bp.wordwrap = val.(bool)
	case "mode":
		bp.SetMode(val.(string))
	case "tabsize":
		bp.tabsize = int(val.(int64))
		bp.vis.(*buffer.Visualizer).TabSize = bp.tabsize
	case "charmap":
		bp.vis.(*buffer.Visualizer).CharMap = parseCharMap(val.(string))
	case "autoindent":
		bp.autoindent = val.(bool)
	case "cursorline":
		bp.cursorline = val.(bool)
	}
	return nil
}

func (bp *BufPane) GetOpt(opt string) (interface{}, bool) {
	return bp.Buffer.GetOpt(opt)
}

func parseCharMap(s string) map[rune]string {
	// charmap encoded as rune for '\t', '\n', ' '
	runes := []rune{'\t', '\n', ' '}
	m := make(map[rune]string)
	for i, r := range s {
		if i >= len(runes) {
			break
		}
		m[runes[i]] = string(r)
	}
	return m
}
