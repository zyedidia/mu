package mu

import (
	"bytes"
	"fmt"

	"github.com/zyedidia/mu/pkg/grapheme"
	"github.com/zyedidia/mu/pkg/theme"
)

type CompleteBar struct {
	suggestions []string
	cur         int
	active      bool
	next        bool
}

func (c *CompleteBar) Display(draw DrawFn, w int, th *theme.Theme) {
	if !c.active || len(c.suggestions) == 0 {
		return
	}

	b := &bytes.Buffer{}
	for i, s := range c.suggestions {
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

func (e *Editor) StartCompletion(suggestions []string) {
	e.complete.suggestions = suggestions
	e.complete.cur = 0
}

func (e *Editor) StopCompletion() {
	e.complete.suggestions = nil
}

func (e *Editor) NextCompletion() {
	if e.complete.cur < len(e.complete.suggestions)-1 {
		e.complete.cur++
	} else {
		e.complete.cur = 0
	}
}

func (e *Editor) PrevCompletion() {
	if e.complete.cur > 0 {
		e.complete.cur--
	} else {
		e.complete.cur = len(e.complete.suggestions) - 1
	}
}

func (e *Editor) ActiveCompletion() bool {
	return e.complete.active
}

func (e *Editor) KeepCompleting() {
	e.complete.next = true
}
