package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/zyedidia/mu"
	"gopkg.in/readline.v1"
)

func main() {
	flag.Parse()

	args := flag.Args()

	var ed *mu.Editor
	var err error
	if len(args) > 0 {
		ed, err = mu.NewEditorFromPath(args[0], 0, 0, nil, nil)
	} else {
		ed, err = mu.NewEditor(0, 0, nil, nil)
	}
	if err != nil {
		log.Fatal(err)
	}

	rl, err := readline.New("> ")
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil { // io.EOF
			break
		}
		err = ed.Eval(line, nil)
		if err == mu.ErrQuit {
			break
		} else if err != nil {
			fmt.Println("Error:", err)
		}
	}
}
