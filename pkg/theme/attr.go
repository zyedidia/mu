package theme

import (
	"fmt"
)

type AttrMask int

const (
	AttrBold = 1 << iota
	AttrBlink
	AttrReverse
	AttrUnderline
	AttrDim
	AttrItalic
	AttrStrikethrough
	AttrHidden
	AttrNone AttrMask = 0
)

func Attr(s string) (AttrMask, error) {
	switch s {
	case "bold":
		return AttrBold, nil
	case "blink":
		return AttrBlink, nil
	case "reverse":
		return AttrReverse, nil
	case "underline":
		return AttrUnderline, nil
	case "dim":
		return AttrDim, nil
	case "italic":
		return AttrItalic, nil
	case "strikethrough":
		return AttrStrikethrough, nil
	case "hidden":
		return AttrHidden, nil
	}
	return AttrNone, fmt.Errorf("invalid attribute: %s", s)
}

func (a *AttrMask) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var attrs []string
	err := unmarshal(&attrs)
	if err != nil {
		return err
	}

	for _, s := range attrs {
		attr, err := Attr(s)
		if err != nil {
			return err
		}
		*a |= attr
	}
	return nil
}

func (a AttrMask) MarshalYAML() (interface{}, error) {
	var arr []string
	kinds := []AttrMask{
		AttrBold,
		AttrBlink,
		AttrReverse,
		AttrUnderline,
		AttrDim,
		AttrItalic,
		AttrStrikethrough,
		AttrHidden,
	}
	for _, k := range kinds {
		if (a & k) != 0 {
			var name string
			switch k {
			case AttrBold:
				name = "bold"
			case AttrBlink:
				name = "blink"
			case AttrReverse:
				name = "reverse"
			case AttrUnderline:
				name = "underline"
			case AttrDim:
				name = "dim"
			case AttrItalic:
				name = "italic"
			case AttrStrikethrough:
				name = "strikethrough"
			case AttrHidden:
				name = "hidden"
			}
			arr = append(arr, name)
		}
	}
	return arr, nil
}
