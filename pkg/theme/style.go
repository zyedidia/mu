package theme

type Style struct {
	Fg, Bg Color
	Attr   AttrMask
}

func (s Style) Add(a AttrMask) Style {
	s.Attr |= a
	return s
}
