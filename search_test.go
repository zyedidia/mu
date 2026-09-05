package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestFindDown(t *testing.T) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte("hello world hello"))
	re := regexp.MustCompile("hello")

	loc := b.FindDown(re, 0)
	if loc == nil || loc[0] != 0 {
		t.Fatalf("first match: %v", loc)
	}

	loc = b.FindDown(re, 1)
	if loc == nil || loc[0] != 12 {
		t.Fatalf("second match: %v", loc)
	}

	// From past last match, should wrap to first.
	loc = b.FindDown(re, 13)
	if loc == nil || loc[0] != 0 {
		t.Fatalf("wrap: %v", loc)
	}
}

func TestFindUp(t *testing.T) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte("hello world hello"))
	re := regexp.MustCompile("hello")

	loc := b.FindUp(re, 17) // from end
	if loc == nil || loc[0] != 12 {
		t.Fatalf("last match: %v", loc)
	}

	loc = b.FindUp(re, 12) // before second match
	if loc == nil || loc[0] != 0 {
		t.Fatalf("first match: %v", loc)
	}

	// From before first match, should wrap to last.
	loc = b.FindUp(re, 0)
	if loc == nil || loc[0] != 12 {
		t.Fatalf("wrap: %v", loc)
	}
}

func TestSearchForwardWrap(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("aaa bbb aaa"))
	*b.Cursor() = b.Cursor().MoveTo(4) // on 'b'

	ed.searchForward("aaa")
	// Should find the second "aaa" at position 8.
	if b.Cursor().Pos != 8 {
		t.Fatalf("forward: pos=%d, want 8", b.Cursor().Pos)
	}

	ed.searchForward("aaa")
	// Should wrap to the first "aaa" at position 0.
	if b.Cursor().Pos != 0 {
		t.Fatalf("wrap: pos=%d, want 0", b.Cursor().Pos)
	}
}

func TestSearchBackward(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("aaa bbb aaa"))
	*b.Cursor() = b.Cursor().MoveTo(11) // past end

	ed.searchBackward("aaa")
	if b.Cursor().Pos != 8 {
		t.Fatalf("backward: pos=%d, want 8", b.Cursor().Pos)
	}

	ed.searchBackward("aaa")
	if b.Cursor().Pos != 0 {
		t.Fatalf("backward again: pos=%d, want 0", b.Cursor().Pos)
	}
}

func TestSearchNext(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("foo bar foo baz foo"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	ed.searchForward("foo")
	// First search from pos 0 starts at pos 1, finds "foo" at 8.
	if b.Cursor().Pos != 8 {
		t.Fatalf("first: pos=%d, want 8", b.Cursor().Pos)
	}

	ed.searchNext()
	if b.Cursor().Pos != 16 {
		t.Fatalf("next: pos=%d, want 16", b.Cursor().Pos)
	}

	ed.searchPrev()
	if b.Cursor().Pos != 8 {
		t.Fatalf("prev: pos=%d, want 8", b.Cursor().Pos)
	}
}

func TestSearchNotFound(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("hello world"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	ed.searchForward("zzz")
	if !ed.infobar.msgErr {
		t.Fatal("should show error for not found")
	}
}

func TestSubstituteAll(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("foo bar foo baz foo"))

	ed.RunCommand("s foo replaced all")
	got := string(b.Slice(0, b.Len()))
	if got != "replaced bar replaced baz replaced" {
		t.Fatalf("substitute all: got %q", got)
	}
}

func TestSubstituteRegex(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("cat bat hat"))

	ed.RunCommand("s {[cbh]at} dog all")
	got := string(b.Slice(0, b.Len()))
	if got != "dog dog dog" {
		t.Fatalf("substitute regex: got %q", got)
	}
}

func TestSubstituteNotFound(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("hello"))

	ed.RunCommand("s zzz replaced all")
	if !ed.infobar.msgErr {
		t.Fatal("should show error for pattern not found")
	}
}

func TestWordUnderCursor(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("hello world"))
	*b.Cursor() = b.Cursor().MoveTo(7) // on 'o' in "world"

	word := ed.wordUnderCursor()
	if word != "world" {
		t.Fatalf("word: got %q, want %q", word, "world")
	}
}

func TestSearchViaKeybinding(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("abc def abc"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	// Type /def<CR> via key dispatch.
	ed.ks.HandleKey("/")
	if !ed.infobar.IsActive() {
		t.Fatal("/ should activate prompt")
	}
	ed.infobar.HandleKey("d")
	ed.infobar.HandleKey("e")
	ed.infobar.HandleKey("f")
	ed.infobar.HandleKey(KeyEnter)

	if b.Cursor().Pos != 4 {
		t.Fatalf("search /def: pos=%d, want 4", b.Cursor().Pos)
	}

	// n should find next (wraps to same match since only one).
	ed.ks.HandleKey("n")
	if b.Cursor().Pos != 4 {
		t.Fatalf("n after single match: pos=%d, want 4", b.Cursor().Pos)
	}
}

// --- Incremental search viewport restore ---

// setupIncSearchTest builds an editor with 40 lines, a small view scrolled
// to a distinctive position, and "needle" on line 35.
func setupIncSearchTest(t *testing.T) (*Editor, *View, *Buffer, Viewport, int) {
	t.Helper()
	configDirOverride = t.TempDir()
	t.Cleanup(func() { configDirOverride = "" })
	ed := newTestEditor()
	v := ed.ActiveView()
	b := v.buf
	var text strings.Builder
	for i := 0; i < 40; i++ {
		if i == 35 {
			text.WriteString("needle\n")
		} else {
			fmt.Fprintf(&text, "line %d\n", i)
		}
	}
	b.text.Insert(0, []byte(text.String()))
	v.LineNums = false
	v.GutterWidth = 0
	v.ScrollMargin = 2
	v.Resize(20, 10)

	// Settle on a scrolled position: cursor on line 20.
	*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(20, 2))
	v.Relocate()
	origVp := v.Viewport()
	if origVp.TopLine == 0 {
		t.Fatal("setup: viewport should be scrolled")
	}
	return ed, v, b, origVp, b.Cursor().Pos
}

func TestSearchCancelRestoresViewport(t *testing.T) {
	ed, v, b, origVp, origPos := setupIncSearchTest(t)

	ed.ks.HandleKey("/")
	if !ed.infobar.IsActive() {
		t.Fatal("search prompt should be active")
	}
	for _, ch := range "needle" {
		ed.infobar.HandleKey(string(ch))
	}
	v.Relocate()
	if line, _ := b.LineColAt(b.Cursor().Pos); line != 35 {
		t.Fatalf("incremental search cursor on line %d, want 35", line)
	}
	if v.Viewport() == origVp {
		t.Fatal("incremental search should have scrolled the viewport")
	}

	// Escape: cursor AND viewport return exactly.
	ed.infobar.HandleKey(KeyEscape)
	if b.Cursor().Pos != origPos {
		t.Fatalf("cursor after cancel = %d, want %d", b.Cursor().Pos, origPos)
	}
	if v.Viewport() != origVp {
		t.Fatalf("viewport after cancel = %+v, want %+v", v.Viewport(), origVp)
	}
	// The restored state is stable under the next relocate.
	v.Relocate()
	if v.Viewport() != origVp {
		t.Fatalf("viewport after relocate = %+v, want %+v", v.Viewport(), origVp)
	}
}

func TestSearchNoMatchRestoresViewport(t *testing.T) {
	ed, v, b, origVp, origPos := setupIncSearchTest(t)

	ed.ks.HandleKey("/")
	for _, ch := range "needle" {
		ed.infobar.HandleKey(string(ch))
	}
	v.Relocate()
	if v.Viewport() == origVp {
		t.Fatal("incremental search should have scrolled the viewport")
	}

	// Extending the pattern so nothing matches snaps the view back while
	// still typing.
	for _, ch := range "zzz" {
		ed.infobar.HandleKey(string(ch))
	}
	if b.Cursor().Pos != origPos || v.Viewport() != origVp {
		t.Fatalf("no-match state = (%d, %+v), want (%d, %+v)",
			b.Cursor().Pos, v.Viewport(), origPos, origVp)
	}
	ed.infobar.HandleKey(KeyEscape)
	if b.Cursor().Pos != origPos || v.Viewport() != origVp {
		t.Fatalf("cancel state = (%d, %+v), want (%d, %+v)",
			b.Cursor().Pos, v.Viewport(), origPos, origVp)
	}
}

// --- smartcase ---

func TestPatternHasUpper(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		{"foo", false},
		{"Foo", true},
		{"foO", true},
		{`\bfoo\b`, false},
		{`\Sfoo`, false}, // escaped class: not an uppercase letter
		{`\W`, false},
		{`\\Foo`, true}, // escaped backslash, then a real F
		{`foo\`, false},
		{"héllo", false},
		{"héllO", true},
		{"École", true},
		{"[a-z]+", false},
	}
	for _, tt := range tests {
		if got := patternHasUpper(tt.pattern); got != tt.want {
			t.Errorf("patternHasUpper(%q) = %v, want %v", tt.pattern, got, tt.want)
		}
	}
}

func TestCompileSearchSmartcase(t *testing.T) {
	re, err := compileSearch("foo", true)
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("FOO") {
		t.Error("smartcase lowercase pattern should match uppercase text")
	}

	re, err = compileSearch("Foo", true)
	if err != nil {
		t.Fatal(err)
	}
	if re.MatchString("foo") {
		t.Error("smartcase pattern with an uppercase letter should be case-sensitive")
	}

	re, err = compileSearch("foo", false)
	if err != nil {
		t.Fatal(err)
	}
	if re.MatchString("FOO") {
		t.Error("smartcase off should be case-sensitive")
	}
}

func TestSmartcaseSearchDefault(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("xxx FOO yyy"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	// Enabled by default: a lowercase pattern matches uppercase text.
	ed.searchForward("foo")
	if b.Cursor().Pos != 4 {
		t.Fatalf("smartcase search: pos=%d, want 4", b.Cursor().Pos)
	}
}

func TestSmartcaseSearchUpperIsSensitive(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("xxx foo yyy FOO"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	ed.searchForward("FOO")
	if b.Cursor().Pos != 12 {
		t.Fatalf("uppercase pattern: pos=%d, want 12", b.Cursor().Pos)
	}
}

func TestSmartcaseOff(t *testing.T) {
	configDirOverride = t.TempDir()
	t.Cleanup(func() { configDirOverride = "" })
	ed := newTestEditor()
	if err := ed.config.SetGlobalOpt("smartcase", false); err != nil {
		t.Fatal(err)
	}
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("xxx FOO yyy"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	ed.searchForward("foo")
	if b.Cursor().Pos != 0 {
		t.Fatalf("smartcase off: pos=%d, want 0 (no match)", b.Cursor().Pos)
	}
}

// * and # search for the word under the cursor case-sensitively, as vim
// does not apply 'smartcase' to them.
func TestStarIgnoresSmartcase(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("foo FOO foo"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	ed.ks.HandleKey("*")
	if b.Cursor().Pos != 8 {
		t.Fatalf("* search: pos=%d, want 8", b.Cursor().Pos)
	}
}

func TestSmartcaseSubstitute(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("Foo foo FOO"))

	if err := cmdSubstitute(ed, []string{"foo", "bar", "all"}); err != nil {
		t.Fatal(err)
	}
	if got := string(b.Slice(0, b.Len())); got != "bar bar bar" {
		t.Fatalf("substitute: got %q, want %q", got, "bar bar bar")
	}
}
