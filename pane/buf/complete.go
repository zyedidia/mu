package buf

import (
	"bytes"
	"fmt"

	"github.com/zyedidia/mu/pkg/grapheme"
	"github.com/zyedidia/mu/pkg/theme"
)

const suggestionMax = 20

type DrawFn func(x, y int, mainc rune, combc []rune, style theme.Style)

type CompleteBar struct {
	suggestions []string
	cur         int
	active      bool
	pos         int
	prefix      string
}

func (c *CompleteBar) Display(draw DrawFn, w int, th *theme.Theme) {
	if !c.active || len(c.suggestions) == 0 {
		return
	}

	b := &bytes.Buffer{}
	for i, s := range c.suggestions {
		if len(s) >= suggestionMax {
			s = s[:suggestionMax] + "..."
		}
		if i == c.cur {
			fmt.Fprintf(b, "[%s]", s)
		} else {
			fmt.Fprint(b, s)
		}
		if i != len(c.suggestions)-1 {
			b.WriteRune(' ')
		}
	}
	bar := b.String()

	style := th.Default().Add(theme.AttrReverse)
	x := 0
	for len(bar) > 0 && x < w {
		r, combc, size := grapheme.DecodeInString(bar)
		draw(x, 0, r, combc, style)
		bar = bar[size:]
		x++
	}

	for x < w {
		draw(x, 0, ' ', nil, style)
		x++
	}
}

func (c *CompleteBar) StartCompletion(suggestions []string, pos int, prefix string) {
	c.suggestions = suggestions
	c.cur = 0
	c.pos = pos
	c.prefix = prefix
	c.active = true
}

func (c *CompleteBar) StopCompletion() {
	c.suggestions = nil
	c.active = false
	c.cur = 0
}

func (c *CompleteBar) NextCompletion() {
	if c.cur < len(c.suggestions)-1 {
		c.cur++
	} else {
		c.cur = 0
	}
}

func (c *CompleteBar) PrevCompletion() {
	if c.cur > 0 {
		c.cur--
	} else {
		c.cur = len(c.suggestions) - 1
	}
}

func (c *CompleteBar) ActiveCompletion() bool {
	return c.active
}

func (c *CompleteBar) Suggestion() string {
	return c.suggestions[c.cur]
}
