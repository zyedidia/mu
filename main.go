package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/gdamore/tcell/v2"
)

func main() {
	// Set up logging. Never leave the default stderr output in place: once
	// tcell owns the terminal, stray log writes would corrupt the screen.
	logf, err := os.Create(filepath.Join(os.TempDir(), fmt.Sprintf("mu-%d.log", os.Getpid())))
	if err == nil {
		log.SetOutput(logf)
		defer logf.Close()
	} else {
		log.SetOutput(io.Discard)
	}

	// Load config.
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	// Load theme.
	themeName := cfg.GlobalStrOpt("theme")
	if themeName == "" {
		themeName = "monokai"
	}
	th, err := cfg.LoadTheme(themeName)
	if err != nil {
		log.Printf("theme %q: %v, using default\n", themeName, err)
		th = DefaultTheme
	}

	// Initialize tcell screen.
	screen, err := tcell.NewScreen()
	if err != nil {
		fmt.Fprintf(os.Stderr, "screen: %v\n", err)
		os.Exit(1)
	}
	if err := screen.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "screen init: %v\n", err)
		os.Exit(1)
	}
	defer screen.Fini()

	// Mouse support is deferred (see PLAN.md); leaving mouse reporting off
	// keeps the terminal's native text selection working.
	screen.SetStyle(th.Default().TCellStyle())

	// Create editor.
	ed := NewEditor(screen, cfg, th)

	// Open files from arguments (the first in the current pane, the rest in
	// their own tabs), or an empty buffer.
	args := os.Args[1:]
	if len(args) > 0 {
		if err := ed.OpenFiles(args); err != nil {
			ed.Error(fmt.Sprintf("open: %v", err))
		}
	}
	if len(ed.tabs) == 0 {
		ed.OpenEmpty()
	}

	// Run the editor.
	ed.Run()
}
