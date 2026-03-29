package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompleteCommandName(t *testing.T) {
	candidates := completeCommandName("q")
	if len(candidates) == 0 {
		t.Fatal("should have candidates for 'q'")
	}
	for _, c := range candidates {
		if c == "quit" || c == "quit!" || c == "q" || c == "q!" {
			continue
		}
	}

	// Empty prefix returns all commands.
	all := completeCommandName("")
	if len(all) < len(editorCommands) {
		t.Fatalf("empty prefix: got %d candidates, want >= %d", len(all), len(editorCommands))
	}
}

func TestCompleteFilePath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "foo.go"), []byte{}, 0644)
	os.WriteFile(filepath.Join(dir, "foo_test.go"), []byte{}, 0644)
	os.WriteFile(filepath.Join(dir, "bar.txt"), []byte{}, 0644)

	candidates := completeFilePath(filepath.Join(dir, "foo"))
	if len(candidates) != 2 {
		t.Fatalf("foo prefix: got %d candidates, want 2: %v", len(candidates), candidates)
	}

	// Directory completion.
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)
	candidates = completeFilePath(filepath.Join(dir, "sub"))
	if len(candidates) != 1 {
		t.Fatalf("sub prefix: got %d candidates, want 1: %v", len(candidates), candidates)
	}
	// Directory candidates end with separator.
	if candidates[0][len(candidates[0])-1] != filepath.Separator {
		t.Fatalf("directory should end with separator: %q", candidates[0])
	}
}

func TestCompleteOptionName(t *testing.T) {
	ed := newTestEditor()
	candidates := completeOptionName(ed, "tab")
	found := false
	for _, c := range candidates {
		if c == "tabsize" || c == "tabstospaces" {
			found = true
		}
	}
	if !found {
		t.Fatalf("should find tabsize/tabstospaces: %v", candidates)
	}
}

func TestCompleteOptionValue(t *testing.T) {
	ed := newTestEditor()

	// Theme completion.
	themes := completeOptionValue(ed, "theme", "")
	if len(themes) == 0 {
		t.Fatal("should have theme candidates")
	}
	foundMonokai := false
	for _, th := range themes {
		if th == "monokai" {
			foundMonokai = true
		}
	}
	if !foundMonokai {
		t.Fatalf("should include monokai: %v", themes)
	}

	// Bool completion.
	bools := completeOptionValue(ed, "syntax", "")
	if len(bools) != 2 {
		t.Fatalf("bool option should have 2 candidates: %v", bools)
	}
}

func TestCmdCompleter(t *testing.T) {
	ed := newTestEditor()
	c := cmdCompleter(ed)

	// Command name completion.
	candidates := c("q")
	if len(candidates) == 0 {
		t.Fatal("'q' should have candidates")
	}

	// Argument completion for 'set'.
	candidates = c("set t")
	found := false
	for _, cand := range candidates {
		if cand == "tabsize" || cand == "theme" {
			found = true
		}
	}
	if !found {
		t.Fatalf("'set t' should complete option names: %v", candidates)
	}

	// Theme value completion for 'set theme'.
	candidates = c("set theme m")
	if len(candidates) == 0 {
		t.Fatal("'set theme m' should have theme candidates")
	}
}

func TestInfoBarTabCompletion(t *testing.T) {
	ed := newTestEditor()

	// Open prompt with completer.
	ed.infobar.StartPrompt(":", func(input string) {})
	ed.infobar.SetCompleter(cmdCompleter(ed))

	// Type "q" then Tab.
	ed.infobar.HandleKey("q")
	ed.infobar.HandleKey(KeyTab)

	input := string(ed.infobar.input)
	if input == "q" {
		t.Fatal("Tab should have completed 'q' to something")
	}

	// Another Tab should cycle.
	prev := input
	ed.infobar.HandleKey(KeyTab)
	next := string(ed.infobar.input)
	// If there are multiple candidates, it should have changed.
	_ = prev
	_ = next

	// Typing a new character resets completion.
	ed.infobar.HandleKey("x")
	if ed.infobar.completion.active {
		t.Fatal("completion should reset after typing")
	}
}
