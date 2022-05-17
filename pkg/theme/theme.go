package theme

import (
	"errors"
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

	if r, ok := t.rules[group]; ok {
		return r
	}
	i := strings.LastIndexByte(group, '.')
	if i == -1 {
		return t.def
	}
	return t.Style(group[:i])
}

func (t *Theme) Default() Style {
	if t == nil {
		return Style{}
	}
	return t.def
}
