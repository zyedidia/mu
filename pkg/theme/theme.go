package theme

import (
	"bytes"
	"errors"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

var Default = &Theme{
	def:   Style{},
	rules: make(map[string]Style),
}

type Theme struct {
	def   Style
	rules map[string]Style
}

func LoadYAML(data []byte) (*Theme, error) {
	t := &Theme{
		rules: make(map[string]Style),
	}

	err := yaml.Unmarshal(data, &t.rules)
	if err != nil {
		return nil, err
	}

	if s, ok := t.rules["default"]; ok {
		t.def = s
	} else {
		return nil, errors.New("no default style")
	}

	return t, nil
}

func Load(def Style, rules map[string]Style) *Theme {
	return &Theme{
		def:   def,
		rules: rules,
	}
}

func (t *Theme) Style(group string) Style {
	if t == nil {
		return Style{}
	}

	st := t.def
	parts := strings.Split(group, ":")
	fg := false

	for _, p := range parts {
		if r, ok := t.rules[p]; ok {
			st = r
		} else if a, err := Attr(p); err == nil {
			st = st.Add(a)
		} else {
			i := strings.LastIndexByte(p, '.')
			if i == -1 {
				if fg {
					st.Bg = NewNamedColor(p)
				} else {
					st.Fg = NewNamedColor(p)
				}
			} else {
				st = t.Style(group[:i])
			}
		}
	}

	return st
}

func (t *Theme) Default() Style {
	if t == nil {
		return Style{}
	}
	return t.def
}

type ColorSegment struct {
	Style Style
	Text  string
}

var parseReRaw = `\{\{[a-z0-9:_-]+\}\}`
var parseRe = regexp.MustCompile(`(?i)` + parseReRaw)

func (th *Theme) ColorString(v string) []ColorSegment {
	matches := parseRe.FindAllStringIndex(v, -1)
	if len(matches) == 0 {
		return []ColorSegment{{
			Style: th.Default().Add(AttrReverse),
			Text:  v,
		}}
	}

	var segments []ColorSegment
	var st Style
	result := new(bytes.Buffer)
	m := []int{0, 0}
	for _, nm := range matches {
		// Write the text in between this match and the last.
		segments = append(segments, ColorSegment{
			Style: st,
			Text:  v[m[1]:nm[0]],
		})
		result.WriteString(v[m[1]:nm[0]])
		m = nm

		st = th.Style(v[m[0]+2 : m[1]-2])
	}
	result.WriteString(v[m[1]:])
	segments = append(segments, ColorSegment{
		Style: st,
		Text:  v[m[1]:],
	})

	return segments
}
