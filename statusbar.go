package mu

import (
	"github.com/zyedidia/mu/pane"
	"github.com/zyedidia/mu/pkg/grapheme"
	"github.com/zyedidia/mu/pkg/theme"
	"github.com/zyedidia/mu/pkg/uniseg"
)

type StatusBar struct {
	pane pane.Pane
}

func NewStatusBar(e *Editor, p pane.Pane) *StatusBar {
	return &StatusBar{
		pane: p,
	}
}

func (s *StatusBar) Display(draw DrawFn, w int, th *theme.Theme) {
	l, r := s.pane.Status()
	leftseg, rightseg := th.ColorString(l), th.ColorString(r)

	x := 0
	var style theme.Style
	for _, seg := range leftseg {
		style = seg.Style
		left := seg.Text
		for len(left) > 0 && x < w {
			r, combc, size := grapheme.DecodeInString(left)
			draw(x, 0, r, combc, style)
			left = left[size:]
			x++
		}
	}

	rw := 0
	for _, seg := range rightseg {
		rw += uniseg.StringWidth(seg.Text)
	}

	for x < w-rw {
		draw(x, 0, ' ', nil, style)
		x++
	}

	for _, seg := range rightseg {
		style = seg.Style
		right := seg.Text
		for len(right) > 0 && x < w {
			r, combc, size := grapheme.DecodeInString(right)
			draw(x, 0, r, combc, style)
			right = right[size:]
			x++
		}
	}
}
