package mu

import (
	"github.com/mattn/go-runewidth"
	"github.com/zyedidia/mu/pane"
	"github.com/zyedidia/mu/pkg/grapheme"
	"github.com/zyedidia/mu/pkg/theme"
)

type StatusBar struct {
	pane pane.Pane
}

func NewStatusBar(e *Editor, p pane.Pane) *StatusBar {
	return &StatusBar{
		pane: p,
	}
}

func (s *StatusBar) Display(draw func(x, y int, mainc rune, combc []rune, style theme.Style), w int, th *theme.Theme) {
	left, right := s.pane.Status()

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
