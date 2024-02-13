package theme

import (
	"strings"
)

type Style struct {
	Fg, Bg Color
	Attr   AttrMask `yaml:"attr,omitempty"`
}

func (s Style) Add(a AttrMask) Style {
	s.Attr |= a
	return s
}

func (s Style) ParseStyle(v string) Style {
	parts := strings.Split(v, ":")
	fg := true
	for _, p := range parts {
		if a, err := Attr(p); err != nil {
			if fg {
				s.Fg = NewNamedColor(p)
				fg = false
			} else {
				s.Bg = NewNamedColor(p)
			}
		} else {
			s = s.Add(a)
		}
	}
	return s
}
