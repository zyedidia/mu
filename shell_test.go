package main

import (
	"os"
	"path/filepath"
	"testing"
)

// :!cmd runs the command through the shell (without a screen, the
// suspend/prompt steps are skipped); a failing command must not disturb
// the editor.
func TestShellCommand(t *testing.T) {
	ed := newTestEditor()
	path := filepath.Join(t.TempDir(), "out.txt")

	ed.RunCommand("!touch " + path)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("shell command did not run: %v", err)
	}

	ed.RunCommand("!false")
	ed.RunCommand("!")
	if !ed.running {
		t.Fatal("shell command must not stop the editor")
	}
}
