package mu

import (
	"github.com/mattn/go-runewidth"
	"github.com/zyedidia/mu/pane"
	"github.com/zyedidia/mu/pkg/expand"
	"github.com/zyedidia/mu/pkg/grapheme"
	"github.com/zyedidia/mu/pkg/theme"
)

const (
	defLeft  = "$name $modified($(cursor-line),$(cursor-col)) | ft:$filetype"
	defRight = "mu $version"
)

type StatusBar struct {
	resolve func(expr string) (string, error)

	left  string
	right string
}

func NewStatusBar(e *Editor, p pane.Pane, left, right string) *StatusBar {
	return &StatusBar{
		left:  left,
		right: right,
		resolve: func(expr string) (string, error) {
			s, err := p.EvalRet(expr, nil)
			if err != nil {
				return e.EvalRet(expr, nil)
			}
			return s, nil
		},
	}
}

func (s *StatusBar) Display(draw func(x, y int, mainc rune, combc []rune, style theme.Style), w int, th *theme.Theme) {
	left, _ := expand.Expand(s.left, s.resolve, s.resolve)
	right, _ := expand.Expand(s.right, s.resolve, s.resolve)

	style := th.Default().Add(theme.AttrReverse)
	x := 0
	for len(left) > 0 && x < w {
		r, combc, size := grapheme.DecodeInString(left)
		draw(x, 0, r, combc, style)
		left = left[size:]
		x++
	}

	rw := runewidth.StringWidth(right)

	for x < w-rw {
		draw(x, 0, ' ', nil, style)
		x++
	}

	for len(right) > 0 && x < w {
		r, combc, size := grapheme.DecodeInString(right)
		draw(x, 0, r, combc, style)
		right = right[size:]
		x++
	}
}
