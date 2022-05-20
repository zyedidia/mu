package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/go-errors/errors"
	"github.com/micro-editor/tcell/v2"
	"github.com/zyedidia/ned"
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
	f, err := os.Create(filepath.Join(os.TempDir(), "vned.log"))
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()

	log.SetOutput(f)

	flag.Parse()

	args := flag.Args()

	var ed *ned.Editor
	if len(args) > 0 {
		ed = ned.NewEditorFromPath(args[0])
	} else {
		ed = ned.NewEditor()
	}

	s, e := tcell.NewScreen()
	if e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
	}
	if e := s.Init(); e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
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

	for {
		ev := s.PollEvent()

		err := ed.HandleEvent(ev)
		if err == ned.ErrQuit {
			s.Fini()
			os.Exit(0)
		} else if err != nil {
			log.Println("Error:", err)
		}

		ed.Clear(fill)
		ed.Display(draw, cursor)
		s.Show()
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
