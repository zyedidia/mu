package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/go-errors/errors"
	"github.com/micro-editor/tcell/v2"
	"github.com/zyedidia/kbd"
	"github.com/zyedidia/ned"
	"github.com/zyedidia/ned/pkg/theme"
)

const errmsg = `Please report this issue online on GitHub.`

func main() {
	f, err := os.Create("log.txt")
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

	prog := vimkeys()

	vm := kbd.NewVM(prog.Compile())

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

	cursor := func(x, y int) {
		s.ShowCursor(x, y)
	}

	for {
		ev := s.PollEvent()

		if rev, ok := ev.(*tcell.EventResize); ok {
			w, h := rev.Size()
			ed.Resize(w, h)
		}

		action, ok, more := vm.Exec(ev)
		if !more {
			vm.Reset()
		}
		if ok {
			log.Println(action.Cmd, action.Vars)
			err := ed.EvalWithVars(action.Cmd, action.Vars)
			if err == ned.ErrQuit {
				s.Fini()
				break
			} else if err != nil {
				log.Println("ERR", err)
			}
		}
		s.Clear()
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
