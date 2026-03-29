package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gdamore/tcell/v2"
)

func main() {
	// Set up logging.
	logf, err := os.Create("/tmp/mu.log")
	if err == nil {
		log.SetOutput(logf)
		defer logf.Close()
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

	screen.EnableMouse()
	screen.SetStyle(th.Default().TCellStyle())

	// Create editor.
	ed := NewEditor(screen, cfg, th)

	// Open files from arguments, or an empty buffer.
	args := os.Args[1:]
	if len(args) > 0 {
		for _, path := range args {
			if err := ed.OpenFile(path); err != nil {
				ed.Error(fmt.Sprintf("open %s: %v", path, err))
			}
		}
	}
	if len(ed.tabs) == 0 {
		ed.OpenEmpty()
	}

	// Run the editor.
	ed.Run()
}
