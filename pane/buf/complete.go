package buf

import (
	"bytes"
	"fmt"

	"github.com/zyedidia/mu/pkg/grapheme"
	"github.com/zyedidia/mu/pkg/theme"
)

const suggestionMax = 25

type DrawFn func(x, y int, mainc rune, combc []rune, style theme.Style)

type CompleteBar struct {
	suggestions []string
	prefix      string
}

func (bp *BufPane) activeComplete() bool {
	return bp.mode == "complete"
}

func (bp *BufPane) DisplayStatus(draw func(x, y int, mainc rune, combc []rune, style theme.Style), w int, theme *theme.Theme) bool {
	if !bp.activeComplete() {
		return false
	}
	bp.complete.Display(draw, w, theme, bp.Cursor().CompleteCur)
	return true
}

func (c *CompleteBar) Display(draw DrawFn, w int, th *theme.Theme, choice int) {
	b := &bytes.Buffer{}
	for i, s := range c.suggestions {
		if len(s) > suggestionMax {
			s = "..." + s[len(s)-suggestionMax:]
		}
		if i == choice {
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

func (c *CompleteBar) StartCompletion(suggestions []string, prefix string) {
	c.suggestions = suggestions
	c.prefix = prefix
}

func (c *CompleteBar) StopCompletion() {
	c.suggestions = nil
}
