package main

import (
	"regexp"
	"strings"
	"testing"
)

// The status bar is drawn in shaded sections: the mode and cursor position
// at the ends in the outer shade, the file name and file information one
// shade in, and the gap between them in the darkest.
func TestStatusBarShadedSections(t *testing.T) {
	th := loadEmbeddedTheme(t, "monokai")
	outer, info, fill := statusShades(th)

	b := NewEmptyBuffer()
	b.text.Insert(0, []byte("package main\n"))
	b.Path = "main.go"
	b.Filetype = "go"
	v := NewView(b, 4)
	tab := NewTab(v, 60, 6)

	cells := map[[2]int]Style{}
	runes := map[[2]int]rune{}
	tab.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		cells[[2]int{x, y}] = style
		runes[[2]int{x, y}] = mainc
	}, func(int, int, bool) {}, th, "NORMAL")

	y := 5 // the status row of a 6-row tab
	var text, shades strings.Builder
	for x := 0; x < 60; x++ {
		text.WriteRune(runes[[2]int{x, y}])
		switch cells[[2]int{x, y}] {
		case outer:
			shades.WriteByte('A')
		case info:
			shades.WriteByte('B')
		case fill:
			shades.WriteByte('C')
		default:
			shades.WriteByte('?')
		}
	}

	got := text.String()
	for _, want := range []string{"NORMAL", "main.go", "go", "1:1", "Top"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status bar %q is missing %q", got, want)
		}
	}
	// Sections in order, outermost shade at both ends, darkest in the
	// middle, and nothing left unshaded.
	pattern := regexp.MustCompile(`^A+B+C+B+A+$`)
	if !pattern.MatchString(shades.String()) {
		t.Fatalf("shading %q does not run outer → info → fill → info → outer", shades.String())
	}
}

// An inactive pane shows no mode, so its file name starts the bar; the
// shading still runs inward from both ends.
func TestStatusBarInactivePaneShading(t *testing.T) {
	th := loadEmbeddedTheme(t, "monokai")
	outer, info, fill := statusShades(th)

	b := NewEmptyBuffer()
	b.text.Insert(0, []byte("x\n"))
	b.Path = "main.go"
	b.Filetype = "go"
	v := NewView(b, 4)
	tab := NewTab(v, 60, 8)
	tab.HSplit(NewView(b, 4)) // two panes: the top one is inactive

	cells := map[[2]int]Style{}
	runes := map[[2]int]rune{}
	tab.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		cells[[2]int{x, y}] = style
		runes[[2]int{x, y}] = mainc
	}, func(int, int, bool) {}, th, "NORMAL")

	// Find the inactive pane's status row: the one without the mode.
	row := -1
	for y := 0; y < 8; y++ {
		var line strings.Builder
		for x := 0; x < 60; x++ {
			line.WriteRune(runes[[2]int{x, y}])
		}
		if strings.Contains(line.String(), "main.go") && !strings.Contains(line.String(), "NORMAL") {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatal("no inactive status bar found")
	}

	var shades strings.Builder
	for x := 0; x < 60; x++ {
		switch cells[[2]int{x, row}] {
		case outer:
			shades.WriteByte('A')
		case info:
			shades.WriteByte('B')
		case fill:
			shades.WriteByte('C')
		default:
			shades.WriteByte('?')
		}
	}
	if !regexp.MustCompile(`^B+C+B+A+$`).MatchString(shades.String()) {
		t.Fatalf("inactive shading %q does not run info → fill → info → outer", shades.String())
	}
}
