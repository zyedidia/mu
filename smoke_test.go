package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// TestEditorSmoke boots a full editor on a tcell simulation screen, drives
// it through the real event loop with a mix of editing commands, and exits
// with :q!. This covers the Run/Display wiring that unit tests bypass.
func TestEditorSmoke(t *testing.T) {
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	defer func() {
		configDirOverride = ""
		dataDirOverride = ""
	}()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	ed := NewEditor(screen, cfg, DefaultTheme)

	dir := t.TempDir()
	path := filepath.Join(dir, "smoke.txt")
	os.WriteFile(path, []byte("hello world\nsecond line\nthird line\n"), 0644)
	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}

	keys := func(s string) {
		for _, r := range s {
			screen.InjectKey(tcell.KeyRune, r, tcell.ModNone)
		}
	}
	esc := func() { screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone) }
	enter := func() { screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) }

	go func() {
		keys("itext ") // insert
		esc()
		keys("dd")  // delete line
		keys("vjy") // visual select and yank
		esc()
		keys("p")   // paste
		keys("u")   // undo
		keys("2w")  // motion with count
		keys("d2d") // linewise with count
		keys("/line")
		enter()
		keys("n")
		keys(":s {line} LINE all")
		enter()
		keys(":q!")
		enter()
	}()

	done := make(chan struct{})
	go func() {
		ed.Run()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("editor did not exit within 15s")
	}
}
