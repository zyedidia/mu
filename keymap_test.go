package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// newMapTestEditor creates a test editor with an isolated config directory
// and the given buffer text. The init script is NOT run automatically.
func newMapTestEditor(t *testing.T, text string) *Editor {
	t.Helper()
	configDirOverride = t.TempDir()
	t.Cleanup(func() { configDirOverride = "" })
	ed := newTestEditor()
	b := ed.ks.Buf()
	if len(text) > 0 {
		b.text.Insert(0, []byte(text))
	}
	*b.Cursor() = b.Cursor().MoveTo(0)
	return ed
}

func evalOrFatal(t *testing.T, ed *Editor, script string) {
	t.Helper()
	if err := ed.EvalTCL(script); err != nil {
		t.Fatalf("eval %q: %v", script, err)
	}
}

// --- Key notation parsing ---

func TestParseKeys(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"abc", []string{"a", "b", "c"}},
		{"0", []string{"0"}},
		{"gp", []string{"g", "p"}},
		{"<C-v>j", []string{"<C-v>", "j"}},
		{"<Esc>", []string{"<Esc>"}},
		{"<esc>", []string{"<Esc>"}},
		{"<ESC>", []string{"<Esc>"}},
		{"<CR>", []string{"<CR>"}},
		{"<Enter>", []string{"<CR>"}},
		{"<Return>", []string{"<CR>"}},
		{"<BS>", []string{"<BS>"}},
		{"<Tab>x", []string{"<Tab>", "x"}},
		{"<S-Tab>", []string{"<S-Tab>"}},
		{"<C-space>", []string{"<C-space>"}},
		{"<Space>x", []string{" ", "x"}},
		{"<lt>", []string{"<"}},
		{"<Nop>", []string{"<Nop>"}},
		{"<C-A>", []string{"<C-a>"}},
		{"<c-x>", []string{"<C-x>"}},
		{"<A-x>", []string{"<A-x>"}},
		{"<M-x>", []string{"<A-x>"}},
		{"<Up><Down><Left><Right>", []string{"<Up>", "<Down>", "<Left>", "<Right>"}},
		{"<Home><End><PgUp><PgDn>", []string{"<Home>", "<End>", "<PgUp>", "<PgDn>"}},
		{"i hello", []string{"i", " ", "h", "e", "l", "l", "o"}},
		// Unterminated '<' is a literal character.
		{"a<b", []string{"a", "<", "b"}},
	}
	for _, tt := range tests {
		got, err := ParseKeys(tt.in)
		if err != nil {
			t.Fatalf("ParseKeys(%q): %v", tt.in, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("ParseKeys(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseKeysErrors(t *testing.T) {
	for _, in := range []string{"<bogus>", "<Ctrl-a>", "<C-xy>", "<>"} {
		if _, err := ParseKeys(in); err == nil {
			t.Fatalf("ParseKeys(%q): expected error", in)
		}
	}
}

// --- Mapping behavior ---

func TestMapSwapZeroCaret(t *testing.T) {
	ed := newMapTestEditor(t, "  hello\n")
	evalOrFatal(t, ed, "map 0 ^; map ^ 0")

	feedKeys(ed.ks, "$")
	feedKeys(ed.ks, "0")
	if cursorPos(ed.ks) != 2 {
		t.Fatalf("mapped 0: pos=%d, want 2 (first non-blank)", cursorPos(ed.ks))
	}
	feedKeys(ed.ks, "^")
	if cursorPos(ed.ks) != 0 {
		t.Fatalf("mapped ^: pos=%d, want 0 (column 0)", cursorPos(ed.ks))
	}
	// 0 while a count is pending is still a count digit.
	feedKeys(ed.ks, "10l")
	if cursorPos(ed.ks) != 6 {
		t.Fatalf("10l with mapped 0: pos=%d, want 6", cursorPos(ed.ks))
	}
}

func TestMapMultiKeyLhs(t *testing.T) {
	ed := newMapTestEditor(t, "one\ntwo\n")
	evalOrFatal(t, ed, "nmap gp dd")

	feedKeys(ed.ks, "gp")
	if bufText(ed.ks) != "two\n" {
		t.Fatalf("gp mapping: got %q", bufText(ed.ks))
	}
	// The default gg binding still works alongside the g-prefixed mapping.
	feedKeys(ed.ks, "gg")
	if cursorPos(ed.ks) != 0 {
		t.Fatalf("gg after gp mapping: pos=%d, want 0", cursorPos(ed.ks))
	}
}

func TestMapShadowedOperatorStillWorks(t *testing.T) {
	// Mapping "dx" makes "d" ambiguous; a following non-x key must still
	// reach the default d operator.
	ed := newMapTestEditor(t, "hello world\n")
	evalOrFatal(t, ed, "nmap dx x")

	feedKeys(ed.ks, "dw")
	if bufText(ed.ks) != "world\n" {
		t.Fatalf("dw with dx mapped: got %q", bufText(ed.ks))
	}
	feedKeys(ed.ks, "dx")
	if bufText(ed.ks) != "orld\n" {
		t.Fatalf("dx mapping: got %q", bufText(ed.ks))
	}
}

func TestUnmap(t *testing.T) {
	ed := newMapTestEditor(t, "  hello\n")
	evalOrFatal(t, ed, "map 0 ^")

	feedKeys(ed.ks, "$0")
	if cursorPos(ed.ks) != 2 {
		t.Fatalf("mapped 0: pos=%d, want 2", cursorPos(ed.ks))
	}

	evalOrFatal(t, ed, "unmap 0")
	feedKeys(ed.ks, "$")
	feedKeys(ed.ks, "0")
	if cursorPos(ed.ks) != 0 {
		t.Fatalf("unmapped 0: pos=%d, want 0 (default restored)", cursorPos(ed.ks))
	}

	if err := ed.EvalTCL("unmap 0"); err == nil {
		t.Fatal("unmap of nonexistent mapping should error")
	}
}

func TestImap(t *testing.T) {
	ed := newMapTestEditor(t, "")
	evalOrFatal(t, ed, "imap jk <Esc>")

	feedKeys(ed.ks, "i")
	feedKeys(ed.ks, "jk")
	if ed.ks.ModeID() != ModeNormal {
		t.Fatalf("jk should escape insert mode, mode=%v", ed.ks.ModeID())
	}
	if bufText(ed.ks) != "" {
		t.Fatalf("jk should insert nothing, got %q", bufText(ed.ks))
	}

	// A non-completing sequence falls through: both characters are typed.
	ed2 := newMapTestEditor(t, "")
	evalOrFatal(t, ed2, "imap jk <Esc>")
	feedKeys(ed2.ks, "i")
	feedKeys(ed2.ks, "ja")
	if bufText(ed2.ks) != "ja" {
		t.Fatalf("ja should insert both chars, got %q", bufText(ed2.ks))
	}
	if ed2.ks.ModeID() != ModeInsert {
		t.Fatal("should still be in insert mode")
	}
}

func TestVmapScope(t *testing.T) {
	ed := newMapTestEditor(t, "x12\nx34\n")
	evalOrFatal(t, ed, "vmap Q d")

	// Q in normal mode: unmapped, does nothing.
	feedKeys(ed.ks, "Q")
	if bufText(ed.ks) != "x12\nx34\n" {
		t.Fatalf("Q in normal mode changed buffer: %q", bufText(ed.ks))
	}

	// Q in visual-block mode: runs the block delete.
	feedDisplay(ed.ks, "l", "<C-v>", "j", "Q")
	if bufText(ed.ks) != "x2\nx4\n" {
		t.Fatalf("vmap Q in block mode: got %q", bufText(ed.ks))
	}
	if ed.ks.ModeID() != ModeNormal {
		t.Fatal("should be back in normal mode after block delete")
	}
}

func TestMapSpecialKeyLhs(t *testing.T) {
	ed := newMapTestEditor(t, "one\ntwo\n")
	evalOrFatal(t, ed, "nmap <C-g> dd")

	feedSpecial(ed.ks, "<C-g>")
	if bufText(ed.ks) != "two\n" {
		t.Fatalf("<C-g> mapping: got %q", bufText(ed.ks))
	}
}

func TestMapNop(t *testing.T) {
	ed := newMapTestEditor(t, "abc\n")
	evalOrFatal(t, ed, "nmap x <Nop>")

	feedKeys(ed.ks, "x")
	if bufText(ed.ks) != "abc\n" {
		t.Fatalf("x mapped to <Nop> changed buffer: %q", bufText(ed.ks))
	}
}

func TestMapDotRepeat(t *testing.T) {
	ed := newMapTestEditor(t, "abcd\n")
	evalOrFatal(t, ed, "nmap Q x")

	feedKeys(ed.ks, "Q")
	if bufText(ed.ks) != "bcd\n" {
		t.Fatalf("Q mapping: got %q", bufText(ed.ks))
	}
	feedKeys(ed.ks, ".")
	if bufText(ed.ks) != "cd\n" {
		t.Fatalf("dot repeat of mapped Q: got %q", bufText(ed.ks))
	}
}

func TestMapWithCount(t *testing.T) {
	ed := newMapTestEditor(t, "abcd\n")
	evalOrFatal(t, ed, "nmap X x")

	feedKeys(ed.ks, "2X")
	if bufText(ed.ks) != "cd\n" {
		t.Fatalf("2X with X mapped to x: got %q", bufText(ed.ks))
	}
}

func TestMapRhsWithSpace(t *testing.T) {
	ed := newMapTestEditor(t, "world\n")
	// TCL passes multiple rhs words; they are joined with single spaces.
	evalOrFatal(t, ed, "nmap Q ihello<Space><Esc>")

	feedKeys(ed.ks, "Q")
	if bufText(ed.ks) != "hello world\n" {
		t.Fatalf("Q insert mapping: got %q", bufText(ed.ks))
	}
	if ed.ks.ModeID() != ModeNormal {
		t.Fatal("should be back in normal mode")
	}
}

func TestMapCharArgExpansion(t *testing.T) {
	// The expansion may contain a char-argument motion (f); the argument is
	// consumed during the replay.
	ed := newMapTestEditor(t, "hello world\n")
	evalOrFatal(t, ed, "nmap Q fo")

	feedKeys(ed.ks, "Q")
	if cursorPos(ed.ks) != 4 {
		t.Fatalf("Q mapped to fo: pos=%d, want 4", cursorPos(ed.ks))
	}
}

func TestMapOperatorPending(t *testing.T) {
	// "map" covers operator-pending: with the 0/^ swap, d0 deletes back to
	// the first non-blank.
	ed := newMapTestEditor(t, "  hello\n")
	evalOrFatal(t, ed, "map 0 ^; map ^ 0")

	feedKeys(ed.ks, "$d0")
	if bufText(ed.ks) != "  o\n" {
		t.Fatalf("d0 with swap: got %q", bufText(ed.ks))
	}
}

func TestMapErrors(t *testing.T) {
	ed := newMapTestEditor(t, "")
	if err := ed.EvalTCL("map 0"); err == nil {
		t.Fatal("map with one arg should error")
	}
	if err := ed.EvalTCL("map <bogus> x"); err == nil {
		t.Fatal("map with bad key notation should error")
	}
}

// --- init.tcl ---

func TestInitScriptUser(t *testing.T) {
	ed := newMapTestEditor(t, "one\ntwo\n")
	script := "nmap Q dd\n"
	if err := os.WriteFile(filepath.Join(configDirOverride, "init.tcl"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	ed.RunInitScript()

	feedKeys(ed.ks, "Q")
	if bufText(ed.ks) != "two\n" {
		t.Fatalf("init.tcl mapping: got %q", bufText(ed.ks))
	}
}

func TestInitScriptDefault(t *testing.T) {
	// With no user init.tcl, the embedded default applies: it swaps 0 and ^.
	ed := newMapTestEditor(t, "  hello\n")
	ed.RunInitScript()

	feedKeys(ed.ks, "$")
	feedKeys(ed.ks, "0")
	if cursorPos(ed.ks) != 2 {
		t.Fatalf("default init.tcl 0: pos=%d, want 2 (first non-blank)", cursorPos(ed.ks))
	}
	feedKeys(ed.ks, "^")
	if cursorPos(ed.ks) != 0 {
		t.Fatalf("default init.tcl ^: pos=%d, want 0 (column 0)", cursorPos(ed.ks))
	}
}

func TestInitScriptError(t *testing.T) {
	ed := newMapTestEditor(t, "")
	script := "not-a-command foo\n"
	if err := os.WriteFile(filepath.Join(configDirOverride, "init.tcl"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	ed.RunInitScript()
	if !strings.Contains(ed.infobar.message, "init.tcl") {
		t.Fatalf("expected init.tcl error in infobar, got %q", ed.infobar.message)
	}
}
