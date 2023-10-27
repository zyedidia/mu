package theme

import (
	"fmt"
	"strconv"

	"github.com/micro-editor/tcell/v2"
)

type Color struct {
	color tcell.Color
}

func NewHexColor(hex int32) Color {
	return Color{
		color: tcell.NewHexColor(hex),
	}
}

func NewPaletteColor(index int) Color {
	return Color{
		color: tcell.PaletteColor(index),
	}
}

func NewNamedColor(s string) Color {
	return Color{
		color: tcell.GetColor(s),
	}
}

func (c Color) Valid() bool {
	return c.color.Valid()
}

func (c Color) IsRGB() bool {
	return c.color.IsRGB()
}

func (c Color) RGB() (int32, int32, int32) {
	return c.color.RGB()
}

func (c Color) Hex() int32 {
	return c.color.Hex()
}

func (c Color) Palette() int {
	return int(c.color)
}

func ToHex(s string) (hex int32, err error) {
	if len(s) == 7 && s[0] == '#' {
		if v, e := strconv.ParseInt(s[1:], 16, 32); e == nil {
			return int32(v), nil
		} else {
			return 0, e
		}
	}
	return 0, fmt.Errorf("invalid hex code %s", s)
}

func (c *Color) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var i int
	err := unmarshal(&i)
	if err == nil {
		*c = NewPaletteColor(i)
		return nil
	}

	var str string
	err = unmarshal(&str)
	if err != nil {
		return err
	}
	v, err := ToHex(str)
	if err != nil {
		return err
	}
	*c = NewHexColor(v)
	return nil
}
