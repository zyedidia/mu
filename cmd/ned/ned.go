package main

import (
	"flag"
	"fmt"

	"github.com/zyedidia/ned"
	"gopkg.in/readline.v1"
)

func main() {
	flag.Parse()

	args := flag.Args()

	var ed *ned.Editor
	if len(args) > 0 {
		ed = ned.NewEditorFromPath(args[0])
	} else {
		ed = ned.NewEditor()
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
		err = ed.Eval(line)
		if err == ned.ErrQuit {
			break
		} else if err != nil {
			fmt.Println("Error:", err)
		}
	}
}
