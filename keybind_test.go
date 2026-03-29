package main

import (
	"testing"
)

func TestBindingTrieLookup(t *testing.T) {
	bt := NewBindingTrie()
	called := ""
	bt.Bind(func(ks *KeyState) { called = "h" }, "h")
	bt.Bind(func(ks *KeyState) { called = "gg" }, "g", "g")
	bt.Bind(func(ks *KeyState) { called = "gU" }, "g", "U")
	bt.Bind(func(ks *KeyState) { called = "dd" }, "d", "d")

	// Exact match.
	action, result := bt.Lookup([]string{"h"})
	if result != TrieMatch {
		t.Fatal("h should match")
	}
	action(nil)
	if called != "h" {
		t.Fatalf("got %q", called)
	}

	// Prefix.
	_, result = bt.Lookup([]string{"g"})
	if result != TriePrefix {
		t.Fatal("g should be prefix")
	}

	// Two-key match.
	action, result = bt.Lookup([]string{"g", "g"})
	if result != TrieMatch {
		t.Fatal("gg should match")
	}
	action(nil)
	if called != "gg" {
		t.Fatalf("got %q", called)
	}

	// No match.
	_, result = bt.Lookup([]string{"x"})
	if result != TrieNone {
		t.Fatal("x should not match")
	}

	// No match (two keys).
	_, result = bt.Lookup([]string{"g", "x"})
	if result != TrieNone {
		t.Fatal("gx should not match")
	}
}

func TestBindingTrieHasPrefix(t *testing.T) {
	bt := NewBindingTrie()
	bt.Bind(func(ks *KeyState) {}, "g", "g")

	if !bt.HasPrefix([]string{"g"}) {
		t.Fatal("g should be a prefix")
	}
	if bt.HasPrefix([]string{"g", "g"}) {
		t.Fatal("gg should not be a prefix (it's a leaf)")
	}
	if bt.HasPrefix([]string{"x"}) {
		t.Fatal("x should not be a prefix")
	}
}

func TestKeyStateCount(t *testing.T) {
	ks := NewKeyState(NewEmptyBuffer(), NewRegisterSet())

	// No count set: effective count is 1.
	if ks.Count() != 1 {
		t.Fatalf("default count: got %d", ks.Count())
	}

	// Accumulate count: "35".
	ks.HandleKey("3")
	ks.HandleKey("5")
	if ks.Count() != 35 {
		t.Fatalf("count 35: got %d", ks.Count())
	}

	ks.ResetAction()
	if ks.Count() != 1 {
		t.Fatal("count should reset to 1")
	}
}

func TestKeyStateRegister(t *testing.T) {
	ks := NewKeyState(NewEmptyBuffer(), NewRegisterSet())

	// Default register.
	if ks.Register() != RegDefault {
		t.Fatal("default register should be \"")
	}

	// Select register "a.
	ks.HandleKey("\"")
	ks.HandleKey("a")
	if ks.Register() != 'a' {
		t.Fatalf("register: got %c", ks.Register())
	}

	ks.ResetAction()
	if ks.Register() != RegDefault {
		t.Fatal("register should reset to default")
	}
}

func TestKeyStateDispatch(t *testing.T) {
	ks := NewKeyState(NewEmptyBuffer(), NewRegisterSet())

	called := ""
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		called = "j"
	}, "j")
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		called = "gg"
	}, "g", "g")

	// Single key.
	ks.HandleKey("j")
	if called != "j" {
		t.Fatalf("got %q, want j", called)
	}

	// Prefix then complete.
	ks.HandleKey("g") // prefix, waits
	if called != "j" {
		t.Fatal("g alone should not trigger")
	}
	ks.HandleKey("g") // completes gg
	if called != "gg" {
		t.Fatalf("got %q, want gg", called)
	}
}

func TestKeyStateCharWait(t *testing.T) {
	ks := NewKeyState(NewEmptyBuffer(), NewRegisterSet())

	var received string
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.WaitForChar(func(ks *KeyState, ch string) {
			received = ch
		})
	}, "f")

	ks.HandleKey("f")
	if received != "" {
		t.Fatal("should not receive char yet")
	}
	ks.HandleKey("x")
	if received != "x" {
		t.Fatalf("char wait: got %q, want x", received)
	}
}

func TestKeyStateCountWithBinding(t *testing.T) {
	ks := NewKeyState(NewEmptyBuffer(), NewRegisterSet())

	var gotCount int
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		gotCount = ks.Count()
		ks.ResetAction()
	}, "j")

	ks.HandleKey("5")
	ks.HandleKey("j")
	if gotCount != 5 {
		t.Fatalf("count with binding: got %d, want 5", gotCount)
	}
}

func TestKeyStateModeSwitch(t *testing.T) {
	ks := NewKeyState(NewEmptyBuffer(), NewRegisterSet())

	var entered, left bool
	ks.modes[ModeInsert].OnEnter = func(ks *KeyState) { entered = true }
	ks.modes[ModeNormal].OnLeave = func(ks *KeyState) { left = true }

	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.SetMode(ModeInsert)
	}, "i")

	ks.HandleKey("i")
	if !left {
		t.Fatal("normal OnLeave not called")
	}
	if !entered {
		t.Fatal("insert OnEnter not called")
	}
	if ks.ModeID() != ModeInsert {
		t.Fatal("should be in insert mode")
	}
}

func TestRegisterSet(t *testing.T) {
	rs := NewRegisterSet()

	// Basic set/get.
	rs.Set('a', []byte("hello"), false)
	r := rs.Get('a')
	if string(r.Content) != "hello" {
		t.Fatalf("got %q", r.Content)
	}

	// Uppercase appends.
	rs.Set('A', []byte(" world"), false)
	r = rs.Get('a')
	if string(r.Content) != "hello world" {
		t.Fatalf("append: got %q", r.Content)
	}

	// Blackhole discards.
	rs.Set(RegBlackhole, []byte("gone"), false)
	r = rs.Get(RegBlackhole)
	if len(r.Content) != 0 {
		t.Fatal("blackhole should discard")
	}

	// SetDefault updates yank register.
	rs.SetDefault([]byte("yanked"), false, true)
	r = rs.Get(RegYank)
	if string(r.Content) != "yanked" {
		t.Fatalf("yank register: got %q", r.Content)
	}
}
