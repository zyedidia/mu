package main

import (
	"testing"
)

func TestInfoBarHistory(t *testing.T) {
	ib := NewInfoBar()

	// Enter three commands.
	for _, cmd := range []string{"first", "second", "third"} {
		ib.StartPrompt(":", func(input string) {})
		for _, r := range cmd {
			ib.HandleKey(string(r))
		}
		ib.HandleKey(KeyEnter)
	}

	// History for ":" should have 3 entries.
	if len(ib.history[":"]) != 3 {
		t.Fatalf("history length: got %d, want 3", len(ib.history[":"]))
	}

	// Open a new prompt and press Up to get "third".
	ib.StartPrompt(":", func(input string) {})
	ib.HandleKey(KeyUp)
	if got := string(ib.input); got != "third" {
		t.Fatalf("Up 1: got %q, want %q", got, "third")
	}

	// Up again: "second".
	ib.HandleKey(KeyUp)
	if got := string(ib.input); got != "second" {
		t.Fatalf("Up 2: got %q, want %q", got, "second")
	}

	// Down: back to "third".
	ib.HandleKey(KeyDown)
	if got := string(ib.input); got != "third" {
		t.Fatalf("Down: got %q, want %q", got, "third")
	}

	// Down again: back to empty (original input).
	ib.HandleKey(KeyDown)
	if got := string(ib.input); got != "" {
		t.Fatalf("Down to original: got %q, want empty", got)
	}
}

func TestInfoBarHistoryNoDuplicates(t *testing.T) {
	ib := NewInfoBar()

	for _, cmd := range []string{"same", "same", "same"} {
		ib.StartPrompt(":", func(input string) {})
		for _, r := range cmd {
			ib.HandleKey(string(r))
		}
		ib.HandleKey(KeyEnter)
	}

	if len(ib.history[":"]) != 1 {
		t.Fatalf("consecutive duplicates: got %d entries, want 1", len(ib.history[":"]))
	}
}

func TestInfoBarHistoryPerPrompt(t *testing.T) {
	ib := NewInfoBar()

	// Command history.
	ib.StartPrompt(":", func(input string) {})
	ib.HandleKey("q")
	ib.HandleKey(KeyEnter)

	// Search history.
	ib.StartPrompt("/", func(input string) {})
	ib.HandleKey("f")
	ib.HandleKey("o")
	ib.HandleKey("o")
	ib.HandleKey(KeyEnter)

	if len(ib.history[":"]) != 1 {
		t.Fatalf(": history: got %d", len(ib.history[":"]))
	}
	if len(ib.history["/"]) != 1 {
		t.Fatalf("/ history: got %d", len(ib.history["/"]))
	}

	// Up in : prompt should show "q", not "foo".
	ib.StartPrompt(":", func(input string) {})
	ib.HandleKey(KeyUp)
	if got := string(ib.input); got != "q" {
		t.Fatalf(": Up: got %q, want %q", got, "q")
	}
}
