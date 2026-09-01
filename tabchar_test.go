package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A marked tab draws the mark in its first cell and pads the rest out to
// the tab stop, so the line keeps the width it had unmarked.
func TestVisualizerTabChar(t *testing.T) {
	vis := &Visualizer{TabSize: 4, CharMap: map[rune]string{'\t': "|"}}
	plain := &Visualizer{TabSize: 4, CharMap: map[rune]string{}}

	tests := []struct {
		vx   int
		want string
	}{
		{0, "|   "},
		{1, "|  "},
		{2, "| "},
		{3, "|"},
		{4, "|   "},
	}
	for _, tt := range tests {
		got, style := vis.String('\t', tt.vx, DefaultTheme)
		if got != tt.want {
			t.Errorf("tab at column %d = %q, want %q", tt.vx, got, tt.want)
		}
		if style != DefaultTheme.Style("hidden-char") {
			t.Errorf("tab at column %d is not drawn in the hidden-char style", tt.vx)
		}
		if marked, unmarked := vis.Size('\t', tt.vx, 1), plain.Size('\t', tt.vx, 1); marked != unmarked {
			t.Errorf("marking a tab changed its width at column %d: %d, want %d", tt.vx, marked, unmarked)
		}
		if len([]rune(got)) != vis.Size('\t', tt.vx, 1) {
			t.Errorf("tab at column %d draws %d cells, want %d", tt.vx, len([]rune(got)), vis.Size('\t', tt.vx, 1))
		}
	}

	// Without the option there is nothing to see.
	if got, _ := plain.String('\t', 0, DefaultTheme); got != "    " {
		t.Errorf("unmarked tab = %q, want four spaces", got)
	}
}

func TestTabCharOf(t *testing.T) {
	tests := []struct {
		in   string
		want rune
	}{
		{"|", '|'},
		{"›", '›'},
		{"", 0},
		{"||", 0},   // more than one cell
		{"世", 0},    // double width
		{"\xff", 0}, // not valid UTF-8
	}
	for _, tt := range tests {
		if got := tabCharOf(tt.in); got != tt.want {
			t.Errorf("tabCharOf(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// The option reaches the renderer, and turning it off stops the marking.
func TestTabCharOption(t *testing.T) {
	dir := t.TempDir()
	configDirOverride = dir
	dataDirOverride = t.TempDir()
	t.Cleanup(func() { configDirOverride, dataDirOverride = "", "" })

	p := filepath.Join(dir, "f.go")
	if err := os.WriteFile(p, []byte("func x() {\n\tif y {\n\t\tz()\n\t}\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ed := newTestEditor()
	if err := ed.OpenFile(p); err != nil {
		t.Fatal(err)
	}
	v := ed.ActiveView()
	if got := v.vis.CharMap['\t']; got != "|" {
		t.Fatalf("tab mark = %q, want the default |", got)
	}

	ed.RunCommand(`set tabchar ""`)
	if got, ok := v.vis.CharMap['\t']; ok {
		t.Fatalf(`tab mark after set tabchar "" = %q, want none`, got)
	}

	ed.RunCommand("set tabchar >")
	if got := v.vis.CharMap['\t']; got != ">" {
		t.Fatalf("tab mark after set tabchar > = %q, want >", got)
	}

	// A mark that would not fit one cell is refused, leaving the old one.
	for _, bad := range []string{"||", "世"} {
		ed.RunCommand("set tabchar " + bad)
		if got := v.vis.CharMap['\t']; got != ">" {
			t.Fatalf("set tabchar %s changed the mark to %q", bad, got)
		}
		if ed.infobar.message == "" {
			t.Fatalf("set tabchar %s reported nothing", bad)
		}
	}
}

// End to end: a tab-indented file shows the mark at each tab stop, in the
// theme's hidden-char style, with the text still in its own columns.
func TestTabCharRendered(t *testing.T) {
	dir := t.TempDir()
	configDirOverride = dir
	dataDirOverride = t.TempDir()
	t.Cleanup(func() { configDirOverride, dataDirOverride = "", "" })

	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("a\n\t\tb\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ed := newTestEditor()
	if err := ed.OpenFile(p); err != nil {
		t.Fatal(err)
	}
	th := loadEmbeddedTheme(t, "monokai")
	v := ed.ActiveView()
	v.Resize(16, 4)
	v.LineNums = false
	v.GutterWidth = 0
	v.CursorLine = false

	runes := map[[2]int]rune{}
	styles := map[[2]int]Style{}
	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		runes[[2]int{x, y}] = mainc
		styles[[2]int{x, y}] = style
	}, func(int, int, bool) {}, th, true)

	hidden := th.Style("hidden-char")
	// Row 1 is "\t\tb": marks at columns 0 and 4, the text at column 8.
	for _, x := range []int{0, 4} {
		if got := runes[[2]int{x, 1}]; got != '|' {
			t.Errorf("column %d = %q, want the tab mark", x, got)
		}
		if styles[[2]int{x, 1}] != hidden {
			t.Errorf("the mark at column %d is not in the hidden-char style", x)
		}
	}
	if got := runes[[2]int{8, 1}]; got != 'b' {
		t.Errorf("column 8 = %q, want b", got)
	}
	if styles[[2]int{8, 1}] == hidden {
		t.Error("the text after the tabs is drawn as a hidden char")
	}
	// An unindented line is untouched.
	if got := runes[[2]int{0, 0}]; got != 'a' {
		t.Errorf("row 0 column 0 = %q, want a", got)
	}
}
