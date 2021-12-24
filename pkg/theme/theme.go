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

func (t *Theme) LoadYAML(data []byte) error {
	t.rules = make(map[string]Style)

	err := yaml.Unmarshal(data, &t.rules)
	if err != nil {
		return err
	}

	if s, ok := t.rules["default"]; ok {
		t.def = s
	} else {
		return errors.New("no default style")
	}

	return nil
}

func (t *Theme) Load(def Style, rules map[string]Style) {
	t.rules = make(map[string]Style)
	t.rules = rules
	t.def = def
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
