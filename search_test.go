package main

import (
	"regexp"
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
