package main

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gdamore/tcell/v2"
	"gopkg.in/yaml.v2"
)

// --- Attributes ---

// AttrMask represents text display attributes (bold, italic, etc.).
type AttrMask int

const (
	AttrBold AttrMask = 1 << iota
	AttrBlink
	AttrReverse
	AttrUnderline
	AttrDim
	AttrItalic
	AttrStrikethrough
	AttrHidden
	AttrNone AttrMask = 0
)

// ParseAttr converts a string to an AttrMask.
func ParseAttr(s string) (AttrMask, error) {
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
	if err := unmarshal(&attrs); err != nil {
		return err
	}
	for _, s := range attrs {
		attr, err := ParseAttr(s)
		if err != nil {
			return err
		}
		*a |= attr
	}
	return nil
}

// --- Color ---

// Color wraps a tcell.Color for use in themes.
type Color struct {
	color tcell.Color
}

// NewHexColor creates a color from a 24-bit hex value.
func NewHexColor(hex int32) Color {
	return Color{tcell.NewHexColor(hex)}
}

// NewPaletteColor creates a color from a 256-palette index.
func NewPaletteColor(index int) Color {
	return Color{tcell.PaletteColor(index)}
}

// TCellColor returns the underlying tcell.Color.
func (c Color) TCellColor() tcell.Color {
	return c.color
}

func (c *Color) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var i int
	if err := unmarshal(&i); err == nil {
		c.color = tcell.PaletteColor(i)
		return nil
	}
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	c.color = tcell.GetColor(str)
	return nil
}

// --- Style ---

// Style represents a complete text style with foreground, background, and
// attributes.
type Style struct {
	Fg   Color    `yaml:"fg"`
	Bg   Color    `yaml:"bg"`
	Attr AttrMask `yaml:"attr,omitempty"`
}

// Add returns a new style with the given attribute added.
func (s Style) Add(a AttrMask) Style {
	s.Attr |= a
	return s
}

// TCellStyle converts this Style to a tcell.Style for rendering.
func (s Style) TCellStyle() tcell.Style {
	ts := tcell.StyleDefault.
		Foreground(s.Fg.color).
		Background(s.Bg.color)
	if s.Attr&AttrBold != 0 {
		ts = ts.Bold(true)
	}
	if s.Attr&AttrBlink != 0 {
		ts = ts.Blink(true)
	}
	if s.Attr&AttrReverse != 0 {
		ts = ts.Reverse(true)
	}
	if s.Attr&AttrUnderline != 0 {
		ts = ts.Underline(true)
	}
	if s.Attr&AttrDim != 0 {
		ts = ts.Dim(true)
	}
	if s.Attr&AttrItalic != 0 {
		ts = ts.Italic(true)
	}
	if s.Attr&AttrStrikethrough != 0 {
		ts = ts.StrikeThrough(true)
	}
	return ts
}

// --- Theme ---

// DefaultTheme is the fallback theme with no style rules.
var DefaultTheme = &Theme{
	def:   Style{},
	rules: make(map[string]Style),
}

// Theme maps style group names to styles, with hierarchical lookup.
type Theme struct {
	def   Style
	rules map[string]Style
}

// LoadThemeYAML parses a YAML theme definition. The YAML must contain a
// "default" style entry.
func LoadThemeYAML(data []byte) (*Theme, error) {
	t := &Theme{
		rules: make(map[string]Style),
	}
	if err := yaml.Unmarshal(data, &t.rules); err != nil {
		return nil, err
	}
	if s, ok := t.rules["default"]; ok {
		t.def = s
	} else {
		return nil, errors.New("theme: no 'default' style defined")
	}
	return t, nil
}

// Style returns the style for the given group name. Groups support
// hierarchical lookup: "constant.string" falls back to "constant" if
// "constant.string" is not defined. Colon-separated parts are resolved
// independently and can include attribute names (e.g. "keyword:bold").
func (t *Theme) Style(group string) Style {
	if t == nil {
		return Style{}
	}

	st := t.def
	parts := strings.Split(group, ":")

	for _, p := range parts {
		if r, ok := t.rules[p]; ok {
			st = r
		} else if a, err := ParseAttr(p); err == nil {
			st = st.Add(a)
		} else {
			// Hierarchical fallback: "constant.string" -> "constant"
			i := strings.LastIndexByte(p, '.')
			if i != -1 {
				st = t.Style(p[:i])
			}
		}
	}

	return st
}

// Default returns the default style.
func (t *Theme) Default() Style {
	if t == nil {
		return Style{}
	}
	return t.def
}

// HasStyle returns true if a rule exists for the group.
func (t *Theme) HasStyle(group string) bool {
	_, ok := t.rules[group]
	return ok
}

// ColorSegment is a styled piece of text, used for status line rendering.
type ColorSegment struct {
	Style Style
	Text  string
}

var colorStringRe = regexp.MustCompile(`(?i)\{\{[a-z0-9:._-]+\}\}`)

// ColorString parses a string with {{style-name}} placeholders into styled
// segments.
func (t *Theme) ColorString(v string, defstyle Style) []ColorSegment {
	matches := colorStringRe.FindAllStringIndex(v, -1)
	if len(matches) == 0 {
		return []ColorSegment{{Style: defstyle, Text: v}}
	}

	var segments []ColorSegment
	st := defstyle
	result := new(bytes.Buffer)
	m := []int{0, 0}
	for _, nm := range matches {
		segments = append(segments, ColorSegment{
			Style: st,
			Text:  v[m[1]:nm[0]],
		})
		result.WriteString(v[m[1]:nm[0]])
		m = nm
		st = t.Style(v[m[0]+2 : m[1]-2])
	}
	result.WriteString(v[m[1]:])
	segments = append(segments, ColorSegment{
		Style: st,
		Text:  v[m[1]:],
	})
	return segments
}
