package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/micro-editor/tcell/v2"
	"github.com/zyedidia/kbd"
	"github.com/zyedidia/ned"
)

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

	prog := keybinds()

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

	for {
		ev := s.PollEvent()
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
	}
}
