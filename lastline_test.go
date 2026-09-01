package main

import "testing"

// A file's final newline ends the line it terminates rather than starting an
// empty one after it, as in vim: "a\nb\n" and "a\nb" are both two lines.
func TestLastLine(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"", 0},
		{"a", 0},
		{"a\n", 0},
		{"\n", 0},
		{"a\nb", 1},
		{"a\nb\n", 1},
		{"a\n\n", 1},
		{"a\nb\nc\n", 2},
	}
	for _, tt := range tests {
		b := NewEmptyBuffer()
		if tt.text != "" {
			b.text.Insert(0, []byte(tt.text))
		}
		if got := b.LastLine(); got != tt.want {
			t.Errorf("LastLine(%q) = %d, want %d", tt.text, got, tt.want)
		}
	}
}

// lineModelState is a KeyState with a view, so motions that consult the
// display geometry work.
func lineModelState(text string) (*KeyState, *View) {
	ks := newVimState(text)
	v := NewView(ks.Buf(), 4)
	v.Resize(20, 6)
	v.LineNums = false
	v.GutterWidth = 0
	ks.activeView = func() *View { return v }
	return ks, v
}

// Nothing can put the cursor past the final newline: the motions that reach
// for the end of the file stop on its last line, whether or not the file
// ends with a newline.
func TestNoLineAfterTrailingNewline(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{"G", []string{"G"}},
		{"j past the end", []string{"j", "j", "j", "j"}},
		{"a count past the end", []string{"9", "G"}},
		{"L", []string{"L"}},
		{"paragraph forward", []string{"}", "}", "}"}},
		{"gj", []string{"g", "j", "g", "j", "g", "j"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, text := range []string{"aa\nbb\ncc\n", "aa\nbb\ncc"} {
				ks, _ := lineModelState(text)
				feedDisplay(ks, tt.keys...)
				b := ks.Buf()
				line, _ := b.LineColAt(b.Cursor().Pos)
				if line != 2 {
					t.Errorf("%q: landed on line %d, want the last line (2)", text, line)
				}
				if b.Cursor().Pos > b.Len() {
					t.Errorf("%q: cursor past the end of the buffer", text)
				}
			}
		})
	}
}

// The clamp behind those motions: a cursor put past the final newline comes
// back to the last line, onto its final character.
func TestVimClampOffTrailingNewline(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"aa\nbb\n", 4}, // on the last "b"
		{"aa\n\n", 3},   // an empty last line: its own start
		{"aa\n", 1},     // the last "a"
		{"\n", 0},       // a single empty line
		{"aa\nbb", 5},   // no trailing newline: the position is a real one
		{"", 0},         // nothing to clamp
	}
	for _, tt := range tests {
		b := NewEmptyBuffer()
		if tt.text != "" {
			b.text.Insert(0, []byte(tt.text))
		}
		c := Cursor{Pos: b.Len()}.VimClamp(b)
		if c.Pos != tt.want {
			t.Errorf("VimClamp at the end of %q = %d, want %d", tt.text, c.Pos, tt.want)
		}
	}
}

// Editing at the end of the file still works: the last line can be deleted,
// a line can be opened after it, and the file keeps its final newline.
func TestEditingTheLastLine(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		keys    []string
		want    string
		wantPos int
	}{
		{"dd on the last line", "aa\nbb\ncc\n", []string{"G", "d", "d"}, "aa\nbb\n", 4},
		{"dj on the last line does nothing", "aa\nbb\n", []string{"G", "d", "j"}, "aa\nbb\n", 3},
		{"o on the last line", "aa\nbb\n", []string{"G", "o"}, "aa\nbb\n\n", 6},
		{"dG from the middle", "aa\nbb\ncc\n", []string{"j", "d", "G"}, "aa\n", 1},
		{"x on the last character", "aa\nbb\n", []string{"G", "$", "x"}, "aa\nb\n", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ks, _ := lineModelState(tt.text)
			feedDisplay(ks, tt.keys...)
			if got := bufText(ks); got != tt.want {
				t.Fatalf("text = %q, want %q", got, tt.want)
			}
			if got := ks.Buf().Cursor().Pos; got != tt.wantPos {
				t.Fatalf("cursor at %d, want %d", got, tt.wantPos)
			}
		})
	}
}

// A trailing newline adds no row to render and no line to count, so a file
// with one looks exactly like the same file without.
func TestTrailingNewlineRendersTheSame(t *testing.T) {
	render := func(text string) ([]string, int, int) {
		b := NewEmptyBuffer()
		b.text.Insert(0, []byte(text))
		v := NewView(b, 4)
		v.Resize(14, 5)
		v.LineNums = true
		v.GutterWidth = 0
		v.CursorLine = false
		ks := NewKeyState(b, NewRegisterSet())
		SetupBindings(ks)
		ks.activeView = func() *View { return v }
		feedDisplay(ks, "G")

		runes := map[[2]int]rune{}
		cx, cy := -1, -1
		v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
			runes[[2]int{x, y}] = mainc
		}, func(x, y int, main bool) {
			if main {
				cx, cy = x, y
			}
		}, DefaultTheme, true)

		rows := make([]string, 5)
		for y := 0; y < 5; y++ {
			line := make([]rune, 0, 14)
			for x := 0; x < 14; x++ {
				r := runes[[2]int{x, y}]
				if r == 0 {
					r = ' '
				}
				line = append(line, r)
			}
			rows[y] = string(line)
		}
		return rows, cx, cy
	}

	withNL, wx, wy := render("aa\nbb\ncc\n")
	without, ox, oy := render("aa\nbb\ncc")
	for y := range withNL {
		if withNL[y] != without[y] {
			t.Fatalf("row %d differs: %q with the trailing newline, %q without", y, withNL[y], without[y])
		}
	}
	if wx != ox || wy != oy {
		t.Fatalf("cursor at (%d,%d) with the trailing newline, (%d,%d) without", wx, wy, ox, oy)
	}
	// Line 3 is the last one drawn; there is no fourth.
	if got := withNL[2][0]; got != '3' {
		t.Fatalf("third row starts with %q, want line number 3", got)
	}
	if got := withNL[3][0]; got != ' ' {
		t.Fatalf("a fourth row was drawn: %q", withNL[3])
	}
}

// Vertical motion onto the last line keeps its column. The guard that skips
// the visual-column walk past the end of the buffer used to catch the last
// line itself, sending the cursor to the end of it.
func TestVisualLocOnLastLine(t *testing.T) {
	for _, text := range []string{"abcdef\nghijkl\n", "abcdef\nghijkl"} {
		b := NewEmptyBuffer()
		b.text.Insert(0, []byte(text))
		if got, want := b.VisualLoc(1, 2), b.OffsetAt(1, 2); got != want {
			t.Errorf("VisualLoc(1, 2) in %q = %d, want %d", text, got, want)
		}

		ks, _ := lineModelState(text)
		feedDisplay(ks, "l", "l", "j") // column 2, then down
		bb := ks.Buf()
		line, col := bb.LineColAt(bb.Cursor().Pos)
		if line != 1 || col != 2 {
			t.Errorf("j onto the last line of %q landed at line %d col %d, want line 1 col 2", text, line, col)
		}
	}
}
