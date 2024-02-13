package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/zyedidia/mu/pkg/theme"
	"gopkg.in/yaml.v2"
)

var colorRe = regexp.MustCompile(`color-link ([.a-zA-Z-]+) "(.*?)"`)

func parseOldStyle(style string) theme.Style {
	fields := strings.Fields(style)
	attr := ""
	colors := fields[0]
	if len(fields) > 1 {
		attr = fields[0]
		colors = fields[1]
	}

	var st theme.Style

	if attr != "" {
		a, err := theme.Attr(attr)
		if err != nil {
			panic(err)
		}
		st = st.Add(a)
	}

	before, after, found := strings.Cut(colors, ",")
	var fg, bg theme.Color
	hexfg, err := theme.ToHex(before)
	if err != nil {
		idx, err := strconv.Atoi(before)
		if err != nil {
			fg = theme.NewNamedColor(before)
		} else {
			fg = theme.NewPaletteColor(idx)
		}
	} else {
		fg = theme.NewHexColor(hexfg)
	}
	if found {
		hexbg, err := theme.ToHex(after)
		if err != nil {
			idx, err := strconv.Atoi(after)
			if err != nil {
				bg = theme.NewNamedColor(after)
			} else {
				bg = theme.NewPaletteColor(idx)
			}
		} else {
			bg = theme.NewHexColor(hexbg)
		}
	}
	st.Fg = fg
	st.Bg = bg

	return st
}

func main() {
	th := make(map[string]theme.Style)

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	lines := bytes.Split(data, []byte{'\n'})
	for _, l := range lines {
		s := string(l)
		matches := colorRe.FindAllStringSubmatch(s, -1)
		if matches != nil {
			class := matches[0][1]
			style := matches[0][2]
			switch class {
			case "indent-char":
				class = "hidden-char"
			case "statement":
				class = "keyword"
			case "identifier":
				continue
			}
			if strings.Contains(class, "symbol") {
				continue
			}
			th[class] = parseOldStyle(style)
			if class == "special" {
				// also add function
				th["function"] = parseOldStyle(style)
			}
		}
	}

	for k := range th {
		var bg theme.Color
		if th[k].Bg == bg {
			th[k] = theme.Style{
				Fg:   th[k].Fg,
				Bg:   th["default"].Bg,
				Attr: th[k].Attr,
			}
		}
	}

	yml, err := yaml.Marshal(th)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(string(yml))
}
