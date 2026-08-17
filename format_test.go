package main

import (
	"testing"
)

// newFormatState creates a KeyState whose view has the given textwidth and
// a "//" comment prefix.
func newFormatState(text string, width int) *KeyState {
	b := NewEmptyBuffer()
	if len(text) > 0 {
		b.text.Insert(0, []byte(text))
	}
	*b.Cursor() = b.Cursor().MoveTo(0)
	ks := NewKeyState(b, NewRegisterSet())
	SetupBindings(ks)
	v := NewView(b, 4)
	v.Opts = map[string]any{"textwidth": width, "tabsize": 4}
	ks.activeView = func() *View { return v }
	ks.commentPrefix = func(*Buffer) string { return "//" }
	return ks
}

func TestFormatSplitLongLine(t *testing.T) {
	ks := newFormatState("aaa bbb ccc ddd\n", 10)

	feedKeys(ks, "gqq")
	if bufText(ks) != "aaa bbb\nccc ddd\n" {
		t.Fatalf("gqq: got %q", bufText(ks))
	}
	// Cursor on the first non-blank of the last formatted line.
	if cursorPos(ks) != 8 {
		t.Fatalf("gqq cursor: got %d, want 8", cursorPos(ks))
	}
}

func TestFormatJoinShortLines(t *testing.T) {
	ks := newFormatState("aaa\nbbb\nccc\n", 20)

	feedDisplay(ks, "V", "jj", "gq")
	if bufText(ks) != "aaa bbb ccc\n" {
		t.Fatalf("join gq: got %q", bufText(ks))
	}
}

func TestFormatParagraphsSeparate(t *testing.T) {
	// Blank lines separate blocks and are preserved.
	ks := newFormatState("aa\nbb\n\ncc\ndd\n", 20)

	feedDisplay(ks, "V", "jjjj", "gq")
	if bufText(ks) != "aa bb\n\ncc dd\n" {
		t.Fatalf("paragraph gq: got %q", bufText(ks))
	}
}

func TestFormatIndented(t *testing.T) {
	// The leader (tab, width 4) counts toward the wrap column and is
	// replicated on every wrapped line.
	ks := newFormatState("\taaa bbb ccc\n", 12)

	feedKeys(ks, "gqq")
	if bufText(ks) != "\taaa bbb\n\tccc\n" {
		t.Fatalf("indented gqq: got %q", bufText(ks))
	}
}

func TestFormatComment(t *testing.T) {
	ks := newFormatState("// aaa bbb ccc ddd\n", 15)

	feedKeys(ks, "gqq")
	if bufText(ks) != "// aaa bbb ccc\n// ddd\n" {
		t.Fatalf("comment gqq: got %q", bufText(ks))
	}
}

func TestFormatCommentJoin(t *testing.T) {
	ks := newFormatState("// aa\n// bb\n// cc\n", 40)

	feedDisplay(ks, "V", "jj", "gq")
	if bufText(ks) != "// aa bb cc\n" {
		t.Fatalf("comment join gq: got %q", bufText(ks))
	}
}

func TestFormatCommentIndented(t *testing.T) {
	ks := newFormatState("\t// aaa bbb\n", 12)

	feedKeys(ks, "gqq")
	if bufText(ks) != "\t// aaa\n\t// bbb\n" {
		t.Fatalf("indented comment gqq: got %q", bufText(ks))
	}
}

func TestFormatCommentCodeSeparate(t *testing.T) {
	// Comment lines never merge with code lines.
	ks := newFormatState("// c1 c2\ncode1 code2\n", 60)

	feedDisplay(ks, "V", "j", "gq")
	if bufText(ks) != "// c1 c2\ncode1 code2\n" {
		t.Fatalf("mixed gq: got %q", bufText(ks))
	}
}

func TestFormatMotion(t *testing.T) {
	// gq is an operator: gqip formats the paragraph under the cursor.
	ks := newFormatState("aa\nbb\n\nother\n", 20)

	feedKeys(ks, "gqip")
	if bufText(ks) != "aa bb\n\nother\n" {
		t.Fatalf("gqip: got %q", bufText(ks))
	}

	ks2 := newFormatState("aa\nbb\ncc\n", 20)
	feedDisplay(ks2, "gqj")
	if bufText(ks2) != "aa bb\ncc\n" {
		t.Fatalf("gqj: got %q", bufText(ks2))
	}
}

func TestFormatGqGq(t *testing.T) {
	ks := newFormatState("aaa bbb ccc ddd\n", 10)

	feedKeys(ks, "gqgq")
	if bufText(ks) != "aaa bbb\nccc ddd\n" {
		t.Fatalf("gqgq: got %q", bufText(ks))
	}
}

func TestFormatLongWord(t *testing.T) {
	// Words are never split: an overlong word overflows on its own line.
	ks := newFormatState("aaaaaaaaaa bb\n", 5)

	feedKeys(ks, "gqq")
	if bufText(ks) != "aaaaaaaaaa\nbb\n" {
		t.Fatalf("long word gqq: got %q", bufText(ks))
	}
}

func TestFormatEOFNoNewline(t *testing.T) {
	ks := newFormatState("aaa bbb ccc", 7)

	feedKeys(ks, "gqq")
	if bufText(ks) != "aaa bbb\nccc" {
		t.Fatalf("gqq at EOF: got %q", bufText(ks))
	}
}

func TestFormatEmptyCommentLine(t *testing.T) {
	ks := newFormatState("//\n", 20)

	feedKeys(ks, "gqq")
	if bufText(ks) != "//\n" {
		t.Fatalf("gqq on bare comment: got %q", bufText(ks))
	}
}

func TestFormatUndo(t *testing.T) {
	ks := newFormatState("aaa bbb ccc ddd\n", 10)

	feedKeys(ks, "gqq")
	feedKeys(ks, "u")
	if bufText(ks) != "aaa bbb ccc ddd\n" {
		t.Fatalf("undo after gqq: got %q", bufText(ks))
	}
}

func TestFormatDefaultWidth(t *testing.T) {
	// Without a view/option, the vim-style fallback of 79 applies.
	ks := newVimState("short line\n")

	feedKeys(ks, "gqq")
	if bufText(ks) != "short line\n" {
		t.Fatalf("gqq default width: got %q", bufText(ks))
	}
}

func TestFormatTextwidthOption(t *testing.T) {
	// :set textwidth changes the wrap column.
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()
	ed.RunCommand("set textwidth 10")
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("aaa bbb ccc ddd\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	feedKeys(ed.ks, "gqq")
	if bufText(ed.ks) != "aaa bbb\nccc ddd\n" {
		t.Fatalf("gqq with :set textwidth 10: got %q", bufText(ed.ks))
	}
}
