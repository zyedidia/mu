package main

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
)

// drawStatusBar renders the status line at the given screen row.
func (e *Editor) drawStatusBar(y int) {
	if y < 0 {
		return
	}
	style := e.theme.Style("statusline")
	ts := style.TCellStyle()

	v := e.ActiveView()
	b := v.buf

	// Left: mode | filename [+]
	mode := e.ks.Mode().Name
	name := b.Path
	if name == "" {
		name = "[No Name]"
	}
	mod := ""
	if b.Modified() {
		mod = " [+]"
	}
	left := fmt.Sprintf(" %s | %s%s ", mode, name, mod)

	// Right: encoding | endings | line:col
	line, col := b.LineColAt(b.Cursor().Pos)
	endings := ""
	if b.Text().Opts.Endings != nil {
		endings = b.Text().Opts.Endings.String() + " | "
	}
	right := fmt.Sprintf(" %s%d:%d ", endings, line+1, col+1)

	drawStatusLine(e.screen, y, e.w, left, right, ts)
}

// drawStatusLine renders a left-justified and right-justified string pair
// on a single row, filling the gap with the given style.
func drawStatusLine(screen tcell.Screen, y, w int, left, right string, ts tcell.Style) {
	x := 0
	for _, r := range left {
		if x >= w {
			break
		}
		screen.SetContent(x, y, r, nil, ts)
		x++
	}
	rightStart := w - len([]rune(right))
	if rightStart < x {
		rightStart = x
	}
	for x < rightStart {
		screen.SetContent(x, y, ' ', nil, ts)
		x++
	}
	for _, r := range right {
		if x >= w {
			break
		}
		screen.SetContent(x, y, r, nil, ts)
		x++
	}
}
