package theme

import (
	"strings"
)

type Style struct {
	Fg, Bg Color
	Attr   AttrMask
}

func (s Style) Add(a AttrMask) Style {
	s.Attr |= a
	return s
}

func (s Style) ParseStyle(v string) Style {
	parts := strings.Split(v, ":")
	fg := false
	for _, p := range parts {
		if a, err := Attr(p); err != nil {
			if fg {
				s.Bg = NewNamedColor(p)
			} else {
				s.Fg = NewNamedColor(p)
			}
		} else {
			s = s.Add(a)
		}
	}
	return s
}
