package main

import (
	"os"
	"path/filepath"
	"testing"
)

// newCommentState is a KeyState with a fixed "//" comment prefix.
func newCommentState(text string) *KeyState {
	ks := newVimState(text)
	ks.commentPrefix = func(*Buffer) string { return "//" }
	return ks
}

func TestCommentLineToggle(t *testing.T) {
	ks := newCommentState("code\nmore\n")

	feedKeys(ks, "gcc")
	if bufText(ks) != "// code\nmore\n" {
		t.Fatalf("gcc: got %q", bufText(ks))
	}
	feedKeys(ks, "gcc")
	if bufText(ks) != "code\nmore\n" {
		t.Fatalf("gcc toggle back: got %q", bufText(ks))
	}
}

func TestCommentIndented(t *testing.T) {
	// The prefix goes after the leading whitespace.
	ks := newCommentState("\tif x {\n")

	feedKeys(ks, "gcc")
	if bufText(ks) != "\t// if x {\n" {
		t.Fatalf("gcc indented: got %q", bufText(ks))
	}
	feedKeys(ks, "gcc")
	if bufText(ks) != "\tif x {\n" {
		t.Fatalf("gcc indented toggle back: got %q", bufText(ks))
	}
}

func TestCommentCount(t *testing.T) {
	ks := newCommentState("a\nb\nc\nd\n")

	feedKeys(ks, "3gcc")
	if bufText(ks) != "// a\n// b\n// c\nd\n" {
		t.Fatalf("3gcc: got %q", bufText(ks))
	}
}

func TestCommentGcGc(t *testing.T) {
	ks := newCommentState("code\n")

	feedKeys(ks, "gcgc")
	if bufText(ks) != "// code\n" {
		t.Fatalf("gcgc: got %q", bufText(ks))
	}
}

func TestCommentVisual(t *testing.T) {
	ks := newCommentState("one\ntwo\nthree\n")

	feedDisplay(ks, "V", "j", "gc")
	if bufText(ks) != "// one\n// two\nthree\n" {
		t.Fatalf("Vj gc: got %q", bufText(ks))
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be back in normal mode")
	}
	feedDisplay(ks, "gg", "V", "j", "gc")
	if bufText(ks) != "one\ntwo\nthree\n" {
		t.Fatalf("Vj gc toggle back: got %q", bufText(ks))
	}
}

func TestCommentVisualCharwise(t *testing.T) {
	// A charwise selection still comments whole lines.
	ks := newCommentState("hello\nworld\n")

	feedDisplay(ks, "v", "ll", "gc")
	if bufText(ks) != "// hello\nworld\n" {
		t.Fatalf("vll gc: got %q", bufText(ks))
	}
}

func TestCommentVisualBlock(t *testing.T) {
	ks := newCommentState("one\ntwo\n")

	feedDisplay(ks, "<C-v>", "j", "gc")
	if bufText(ks) != "// one\n// two\n" {
		t.Fatalf("block gc: got %q", bufText(ks))
	}
}

func TestCommentMotion(t *testing.T) {
	// gc is an operator: gcj comments two lines, gcip a paragraph.
	ks := newCommentState("a\nb\nc\n")
	feedDisplay(ks, "gcj")
	if bufText(ks) != "// a\n// b\nc\n" {
		t.Fatalf("gcj: got %q", bufText(ks))
	}

	ks2 := newCommentState("a\nb\n\nc\n")
	feedKeys(ks2, "gcip")
	if bufText(ks2) != "// a\n// b\n\nc\n" {
		t.Fatalf("gcip: got %q", bufText(ks2))
	}
}

func TestCommentMixedRegion(t *testing.T) {
	// If any line is uncommented, comment all; a second toggle removes one
	// comment level everywhere.
	ks := newCommentState("a\n// b\n")

	feedDisplay(ks, "V", "j", "gc")
	if bufText(ks) != "// a\n// // b\n" {
		t.Fatalf("mixed gc: got %q", bufText(ks))
	}
	feedDisplay(ks, "gg", "V", "j", "gc")
	if bufText(ks) != "a\n// b\n" {
		t.Fatalf("mixed gc toggle back: got %q", bufText(ks))
	}
}

func TestCommentBlankLines(t *testing.T) {
	// Blank lines are commented with the rest of the block, with a bare
	// prefix so the line does not end in whitespace, and come back empty.
	ks := newCommentState("a\n\nb\n")

	feedDisplay(ks, "V", "jj", "gc")
	if bufText(ks) != "// a\n//\n// b\n" {
		t.Fatalf("gc with blank line: got %q", bufText(ks))
	}
	feedDisplay(ks, "gg", "V", "jj", "gc")
	if bufText(ks) != "a\n\nb\n" {
		t.Fatalf("gc toggle back with blank line: got %q", bufText(ks))
	}
}

// gcc on a blank line comments just that line, and toggles back.
func TestCommentBlankLineAlone(t *testing.T) {
	ks := newCommentState("\n")

	feedKeys(ks, "gcc")
	if bufText(ks) != "//\n" {
		t.Fatalf("gcc on empty line: got %q", bufText(ks))
	}
	feedKeys(ks, "gcc")
	if bufText(ks) != "\n" {
		t.Fatalf("gcc toggle back: got %q", bufText(ks))
	}
}

// A blank line takes the alignment indent of the block, in whatever mix of
// tabs and spaces the block itself uses, and a whitespace-only line is
// normalized to that same form rather than keeping its stray whitespace.
func TestCommentBlankLineIndent(t *testing.T) {
	tests := []struct {
		name, in, commented, back string
	}{
		{"tabs", "\tx := 1\n\n\ty := 2\n",
			"\t// x := 1\n\t//\n\t// y := 2\n", "\tx := 1\n\n\ty := 2\n"},
		{"spaces", "    a\n\n    b\n",
			"    // a\n    //\n    // b\n", "    a\n\n    b\n"},
		{"deeper line keeps its indent", "\tx := 1\n\n\t\ty := 2\n",
			"\t// x := 1\n\t//\n\t// \ty := 2\n", "\tx := 1\n\n\t\ty := 2\n"},
		{"whitespace-only line", "\tx := 1\n   \n\ty := 2\n",
			"\t// x := 1\n\t//\n\t// y := 2\n", "\tx := 1\n\n\ty := 2\n"},
		{"leading blank", "\na\n", "//\n// a\n", "\na\n"},
		{"all blank", "\n\n", "//\n//\n", "\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ks := newCommentState(tt.in)
			feedDisplay(ks, "V", "jj", "gc")
			if got := bufText(ks); got != tt.commented {
				t.Fatalf("gc: got %q, want %q", got, tt.commented)
			}
			feedDisplay(ks, "gg", "V", "jj", "gc")
			if got := bufText(ks); got != tt.back {
				t.Fatalf("gc back: got %q, want %q", got, tt.back)
			}
		})
	}
}

// A blank line still does not vote on comment-vs-uncomment: an uncommented
// one between two commented paragraphs must not turn the uncomment into a
// second round of commenting.
func TestCommentBlankLineDoesNotBlockUncomment(t *testing.T) {
	ks := newCommentState("// a\n\n// b\n")

	feedDisplay(ks, "V", "jj", "gc")
	if got := bufText(ks); got != "a\n\nb\n" {
		t.Fatalf("uncomment across a blank line: got %q", got)
	}
}

func TestCommentUncommentNoSpace(t *testing.T) {
	// Comments written without a space after the prefix still uncomment.
	ks := newCommentState("//code\n")

	feedKeys(ks, "gcc")
	if bufText(ks) != "code\n" {
		t.Fatalf("gcc on //code: got %q", bufText(ks))
	}
}

func TestCommentNoPrefixNoop(t *testing.T) {
	ks := newVimState("code\n") // no commentPrefix set

	feedKeys(ks, "gcc")
	if bufText(ks) != "code\n" {
		t.Fatalf("gcc without prefix: got %q", bufText(ks))
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be in normal mode")
	}
}

func TestCommentDotRepeat(t *testing.T) {
	ks := newCommentState("a\nb\n")

	feedKeys(ks, "gcc")
	feedKeys(ks, "j.")
	if bufText(ks) != "// a\n// b\n" {
		t.Fatalf("gcc j .: got %q", bufText(ks))
	}
}

func TestCommentUndo(t *testing.T) {
	ks := newCommentState("a\nb\nc\n")

	feedDisplay(ks, "V", "jj", "gc")
	if bufText(ks) != "// a\n// b\n// c\n" {
		t.Fatalf("gc: got %q", bufText(ks))
	}
	feedKeys(ks, "u")
	if bufText(ks) != "a\nb\nc\n" {
		t.Fatalf("undo after gc: got %q", bufText(ks))
	}
}

// --- Alignment at the block's lowest indentation ---

func TestCommentAligned(t *testing.T) {
	// Deeper lines get the prefix at the block's minimal indent, keeping
	// their extra indentation after it.
	ks := newCommentState("\tx := 1\n\t\ty := 2\n")

	feedDisplay(ks, "V", "j", "gc")
	if bufText(ks) != "\t// x := 1\n\t// \ty := 2\n" {
		t.Fatalf("aligned gc: got %q", bufText(ks))
	}
	feedDisplay(ks, "gg", "V", "j", "gc")
	if bufText(ks) != "\tx := 1\n\t\ty := 2\n" {
		t.Fatalf("aligned gc toggle back: got %q", bufText(ks))
	}
}

func TestCommentAlignedAtZero(t *testing.T) {
	ks := newCommentState("func main() {\n\tx := 1\n}\n")

	feedDisplay(ks, "V", "jj", "gc")
	if bufText(ks) != "// func main() {\n// \tx := 1\n// }\n" {
		t.Fatalf("aligned at 0: got %q", bufText(ks))
	}
	feedDisplay(ks, "gg", "V", "jj", "gc")
	if bufText(ks) != "func main() {\n\tx := 1\n}\n" {
		t.Fatalf("toggle back: got %q", bufText(ks))
	}
}

func TestCommentAlignedMixedWhitespace(t *testing.T) {
	// Four spaces and one tab have the same visual width (tabsize 4): the
	// prefix goes after each line's own equal-width indent.
	ks := newCommentState("    a\n\tb\n")

	feedDisplay(ks, "V", "j", "gc")
	if bufText(ks) != "    // a\n\t// b\n" {
		t.Fatalf("mixed ws gc: got %q", bufText(ks))
	}
	feedDisplay(ks, "gg", "V", "j", "gc")
	if bufText(ks) != "    a\n\tb\n" {
		t.Fatalf("toggle back: got %q", bufText(ks))
	}
}

func TestCommentAlignedTabOvershoot(t *testing.T) {
	// Min indent is 2 (spaces); the tab line can't split its tab, so its
	// prefix lands before the tab. Uncommenting still round-trips.
	ks := newCommentState("  a\n\tb\n")

	feedDisplay(ks, "V", "j", "gc")
	if bufText(ks) != "  // a\n// \tb\n" {
		t.Fatalf("overshoot gc: got %q", bufText(ks))
	}
	feedDisplay(ks, "gg", "V", "j", "gc")
	if bufText(ks) != "  a\n\tb\n" {
		t.Fatalf("toggle back: got %q", bufText(ks))
	}
}

func TestCommentAlignedIgnoresBlankLines(t *testing.T) {
	// A blank line must not drag the alignment column to zero: it is
	// commented at the block's indent, not at column 0.
	ks := newCommentState("\ta\n\n\tb\n")

	feedDisplay(ks, "V", "jj", "gc")
	if bufText(ks) != "\t// a\n\t//\n\t// b\n" {
		t.Fatalf("blank line alignment: got %q", bufText(ks))
	}
}

// --- Filetype configuration ---

func TestLoadCommentsMerge(t *testing.T) {
	configDirOverride = t.TempDir()
	t.Cleanup(func() { configDirOverride = "" })
	os.WriteFile(filepath.Join(configDirOverride, "comments.toml"),
		[]byte("go = \";;\"\nmylang = \"%%\"\n"), 0644)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	m := cfg.LoadComments()
	if m["go"] != ";;" {
		t.Fatalf("user override: go = %q, want %q", m["go"], ";;")
	}
	if m["mylang"] != "%%" {
		t.Fatalf("user addition: mylang = %q, want %q", m["mylang"], "%%")
	}
	if m["python"] != "#" {
		t.Fatalf("embedded default: python = %q, want %q", m["python"], "#")
	}
}

func TestCommentFiletype(t *testing.T) {
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()

	path := filepath.Join(t.TempDir(), "x.go")
	os.WriteFile(path, []byte("package main\n"), 0644)
	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	if ft := ed.ActiveView().buf.Filetype; ft != "go" {
		t.Fatalf("filetype = %q, want go", ft)
	}
	feedKeys(ed.ks, "gcc")
	if got := bufText(ed.ks); got != "// package main\n" {
		t.Fatalf("gcc on go file: got %q", got)
	}

	// A shell file uses its own prefix.
	path2 := filepath.Join(t.TempDir(), "x.sh")
	os.WriteFile(path2, []byte("echo hi\n"), 0644)
	if err := ed.OpenFile(path2); err != nil {
		t.Fatal(err)
	}
	feedKeys(ed.ks, "gcc")
	if got := bufText(ed.ks); got != "# echo hi\n" {
		t.Fatalf("gcc on shell file: got %q", got)
	}
}
