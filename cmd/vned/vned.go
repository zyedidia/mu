package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/go-errors/errors"
	"github.com/micro-editor/tcell/v2"
	"github.com/zyedidia/ned"
	"github.com/zyedidia/ned/build"
	"github.com/zyedidia/ned/pkg/theme"
)

type Map[K comparable, V any] map[K]V

func (m Map[K, V]) Get(k K) (V, bool) {
	v, ok := m[k]
	return v, ok
}

func (m Map[K, V]) Put(k K, v V) {
	m[k] = v
}

const errmsg = `Please report this issue online on GitHub.`

func main() {
	if build.Debug == "ON" {
		f, err := os.Create(filepath.Join("/tmp", "vned.log"))
		if err != nil {
			log.Fatalf("error opening file: %v", err)
		} else {
			defer f.Close()
			log.SetOutput(f)
		}
	} else {
		log.SetOutput(io.Discard)
	}

	flag.Parse()

	args := flag.Args()

	s, e := tcell.NewScreen()
	if e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
	}
	if e := s.Init(); e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
	}

	var ed *ned.Editor
	if len(args) > 0 {
		ed = ned.NewEditorFromPath(args[0], s)
	} else {
		ed = ned.NewEditor(s)
	}

	defer func() {
		if err := recover(); err != nil {
			s.Fini()
			fmt.Printf("%s\n%v\n%s\n", "a fatal error occurred", errors.Wrap(err, 2).ErrorStack(), errmsg)
			os.Exit(1)
		}
	}()

	draw := func(vx, vy int, mainc rune, combc []rune, style theme.Style) {
		s.SetContent(vx, vy, mainc, combc, tcellStyle(style))
	}
	fill := func(x rune, style theme.Style) {
		s.Fill(x, tcellStyle(style))
	}

	cursor := func(x, y int) {
		s.ShowCursor(x, y)
	}

	evs := make(chan tcell.Event)

	ed.Display(fill, draw, cursor)
	s.Show()

	go func() {
		for {
			evs <- s.PollEvent()
		}
	}()

	// Set up a signal receiver so we can exit gracefully if the user/OS shuts
	// us down (closing the screen, saving backups, etc.).
	sigterm := make(chan os.Signal, 1)
	quit := make(chan struct{})
	terminate := make(chan int)
	signal.Notify(sigterm, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP)

	go func() {
		for {
			<-sigterm
			quit <- struct{}{}
		}
	}()

	go func() {
	loop:
		for {
			select {
			case err := <-ed.Errors:
				if err == ned.ErrQuit {
					break loop
				}
			case <-ed.Redraw:
				ed.Display(fill, draw, cursor)
				s.Show()
			case <-quit:
				break loop
			}
		}
		s.Fini()
		terminate <- 0
	}()

	for {
		select {
		case ev := <-evs:
			ed.HandleEvent(ev)
		case code := <-terminate:
			os.Exit(code)
		}
	}
}

func tcellColor(color theme.Color) tcell.Color {
	if !color.Valid() {
		return tcell.ColorDefault
	}
	if color.IsRGB() {
		return tcell.NewHexColor(color.Hex())
	}
	return tcell.PaletteColor(color.Palette())
}

func tcellStyle(style theme.Style) tcell.Style {
	return tcell.StyleDefault.
		Foreground(tcellColor(style.Fg)).
		Background(tcellColor(style.Bg)).
		Attributes(tcell.AttrMask(style.Attr)) // small cheat
}
