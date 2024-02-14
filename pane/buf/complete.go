package buf

import (
	"bytes"
	"fmt"

	"github.com/zyedidia/mu/pkg/completer"
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

func (bp *BufPane) fillCompletion(comp []byte) {
	bp.Remove(bp.Cursor().Complete, bp.Cursor().Pos)
	bp.Insert(bp.Cursor().Pos, comp[len(bp.complete.prefix):])
}

func (bp *BufPane) Complete(allowEmpty bool) bool {
	prefix := bp.WordPrefix()
	if !allowEmpty && prefix == "" {
		return false
	}
	comps := completer.FileComplete(prefix, ".")
	if len(comps) == 0 {
		return false
	}
	bp.complete.StartCompletion(comps, prefix)
	bp.Cursor().Complete = bp.Cursor().Pos
	bp.Cursor().CompleteCur = 0
	bp.Insert(bp.Cursor().Pos, []byte(comps[0])[len(bp.complete.prefix):])
	return true
}

func (bp *BufPane) NextCompletion() {
	c := bp.Cursor()
	if c.CompleteCur < len(bp.complete.suggestions)-1 {
		c.CompleteCur++
	} else {
		c.CompleteCur = 0
	}
	bp.fillCompletion([]byte(bp.complete.suggestions[c.CompleteCur]))
}

func (bp *BufPane) PrevCompletion() {
	c := bp.Cursor()
	if c.CompleteCur > 0 {
		c.CompleteCur--
	} else {
		c.CompleteCur = len(bp.complete.suggestions) - 1
	}
	bp.fillCompletion([]byte(bp.complete.suggestions[c.CompleteCur]))
}

func (bp *BufPane) CancelCompletion() {
	if !bp.activeComplete() {
		return
	}
	bp.Remove(bp.Cursor().Complete, bp.Cursor().Pos)
	bp.complete.StopCompletion()
}
