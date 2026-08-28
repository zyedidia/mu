package main

import (
	"regexp"
)

// Multi-cursor select-next-occurrence flow (PLAN.md Step 16, modeled after
// vim-multiple-cursors):
//
//	<C-n>  normal mode: select the word under the cursor and enter visual
//	       mode; visual mode: spawn a cursor selecting the next occurrence
//	       (wrapping at EOF)
//	<C-x>  visual mode: skip this occurrence — move the newest selection
//	       to the next occurrence instead of adding one
//	<C-p>  visual mode: remove the most recently added cursor
//	<Esc>  (and <C-c>) visual mode: leave visual mode with the cursors
//	       intact; normal mode: collapse to the primary cursor
//
// Once cursors exist, everything else already applies to all of them:
// operators, motions, and insert-mode typing.

// mcSelect sets a cursor's selection to [start, end), placing the cursor on
// the last character as charwise visual mode does.
func mcSelect(b *Buffer, c *Cursor, start, end int) {
	_, _, fsz := b.DecodeGraphemeAt(start)
	_, _, lsz := b.DecodeGraphemeBefore(end)
	c.HasSel = true
	c.BlockSel = false
	c.BlockEOL = false
	c.Orig = [2]int{start, start + fsz}
	c.Sel = [2]int{start, end}
	c.Pos = end - lsz
	c.Vx = b.VisualCol(c.Pos)
}

// mcStart begins the flow from normal mode: select the word under the
// cursor and use it (word-bounded, like *) as the search pattern.
func mcStart(ks *KeyState) {
	b := ks.Buf()
	if b.NumCursors() > 1 {
		b.RemoveCursors()
	}
	c := b.Cursor()
	start, end := toInnerWord(b, c.Pos, 1)
	if start >= end {
		return
	}
	text := string(b.Slice(start, end))
	pat := regexp.QuoteMeta(text)
	rs := []rune(text)
	if IsWordChar(rs[0]) {
		pat = `\b` + pat
	}
	if IsWordChar(rs[len(rs)-1]) {
		pat += `\b`
	}
	ks.mcPattern = pat
	mcSelect(b, c, start, end)
	ks.SetMode(ModeVisual)
}

// mcNext finds the next occurrence of the pattern after the newest
// selection. With skip false it spawns a new cursor there; with skip true
// it moves the newest cursor there instead. A manual visual selection is
// adopted as a literal pattern on first use.
func mcNext(ks *KeyState, skip bool) {
	b := ks.Buf()
	c := b.Cursor()
	if !c.HasSelection() {
		return
	}
	if ks.mcPattern == "" {
		text := c.Selection(b)
		if len(text) == 0 {
			return
		}
		ks.mcPattern = regexp.QuoteMeta(string(text))
	}
	re, err := regexp.Compile(ks.mcPattern)
	if err != nil {
		return
	}
	loc := b.FindDown(re, c.Sel[1])
	if loc == nil {
		return
	}
	// An occurrence already claimed by a cursor means we have cycled
	// through every match: stop.
	for i := 0; i < b.NumCursors(); i++ {
		if b.cursors[i].HasSel && b.cursors[i].Sel[0] == loc[0] {
			return
		}
	}
	if skip {
		mcSelect(b, b.Cursor(), loc[0], loc[1])
	} else {
		b.SpawnCursor(loc[0])
		mcSelect(b, b.Cursor(), loc[0], loc[1])
	}
}

// RegisterMultiCursor registers the <C-n>/<C-p>/<C-x> bindings and the
// Escape collapse in normal mode.
func RegisterMultiCursor(ks *KeyState) {
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		mcStart(ks)
		ks.ClearCounts()
	}, "<C-n>")

	ks.modes[ModeVisual].Bindings.Bind(func(ks *KeyState) {
		mcNext(ks, false)
		ks.ClearCounts()
	}, "<C-n>")

	ks.modes[ModeVisual].Bindings.Bind(func(ks *KeyState) {
		mcNext(ks, true)
		ks.ClearCounts()
	}, "<C-x>")

	ks.modes[ModeVisual].Bindings.Bind(func(ks *KeyState) {
		ks.Buf().PopCursor()
		ks.ClearCounts()
	}, "<C-p>")

	// Escape in normal mode: collapse to the primary cursor. <C-c> acts
	// like Escape, as in every other mode.
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.mcPattern = ""
		b := ks.Buf()
		if b.NumCursors() > 1 {
			b.RemoveCursors()
		}
		ks.ResetAction()
	}, KeyEscape)
	ks.modes[ModeNormal].Bindings.Bind(ks.modes[ModeNormal].Bindings.root.children[KeyEscape].action, "<C-c>")
}
