package ned

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/zyedidia/ned/pkg/input"
	"github.com/zyedidia/ned/pkg/output"
	"github.com/zyedidia/ned/pkg/tclutil"
)

// --- Basic ---
func (e *Editor) Help() {
	for _, cmd := range commands {
		fmt.Println(cmd.Doc)
	}
	e.Active().Help(os.Stdout)
}

// --- Buffer management ---

func (e *Editor) Open(path string) error {
	in := &input.File{
		Path: path,
	}
	out := &output.File{
		Path: path,
	}
	return e.open(in, out)
}

func (e *Editor) Quit() {
	i := e.cur
	e.panes[i].Unregister(e.interp)
	copy(e.panes[i:], e.panes[i+1:])
	e.panes[len(e.panes)-1] = nil
	e.panes = e.panes[:len(e.panes)-1]
	if !e.valid() {
		e.cur = len(e.panes) - 1
	}
}

func (e *Editor) QuitAll() {
	ln := len(e.panes)
	for i := 0; i < ln; i++ {
		e.Quit()
	}
}

func (e *Editor) ShowBuffers() {
	for i, b := range e.panes {
		if e.cur == i {
			fmt.Printf("[%d: %v]\n", i, b.Name())
		} else {
			fmt.Printf("%d: %v\n", i, b.Name())
		}
	}
}

func (e *Editor) SetBuffer(name string) error {
	for i, b := range e.panes {
		if b.Name() == name {
			e.cur = i
			return nil
		}
	}
	return fmt.Errorf("buffer '%s' not found", name)
}

func (e *Editor) SetBufferIdx(idx int) error {
	if idx < 0 || idx >= len(e.panes) {
		return fmt.Errorf("invalid buffer index: %d", idx)
	}
	e.cur = idx
	return nil
}

func (e *Editor) NewBuffer() {
	e.MakePane()
	e.open(input.NewReader(strings.NewReader(""), "no name"), &output.Discard{})
}

// --- Options ---

func (e *Editor) Opt(name string, val string) error {
	if v, err := strconv.Atoi(val); err == nil {
		return e.panes[e.cur].Set(name, v)
	} else if v, err := strconv.ParseBool(val); err == nil {
		return e.panes[e.cur].Set(name, v)
	}
	return e.panes[e.cur].Set(name, val)
}

func (e *Editor) Get(name string) (string, error) {
	v := e.panes[e.cur].Get(name)
	if v == nil {
		return "", fmt.Errorf("option %s not found", name)
	}
	return fmt.Sprintf("%v", v), nil
}

var commands = []tclutil.Command{
	{
		"open",
		(*Editor).Open,
		"open <file>: open <file> as a buffer in the current pane",
	},
	{
		"quit",
		(*Editor).Quit,
		"quit: close the current pane",
	},
	{
		"quit-all",
		(*Editor).QuitAll,
		"quit-all: close all panes",
	},
	{
		"show-panes",
		(*Editor).ShowBuffers,
		"show-panes: display all open panes",
	},
	{
		"set-pane-idx",
		(*Editor).SetBufferIdx,
		"set-pane-idx <idx>: set the currently active pane to the <idx>-th pane",
	},
	{
		"set-pane",
		(*Editor).SetBuffer,
		"set-pane <name>: set the currently active pane to the pane with name <name>",
	},
	{
		"new-buffer",
		(*Editor).NewBuffer,
		"new-buffer: open a new empty buffer",
	},
	{
		"opt",
		(*Editor).Opt,
		"opt <name> <val>: assign option <name> to <val>",
	},
	{
		"get",
		(*Editor).Get,
		"get <name>: return the value of option <name>",
	},
}
