package mu

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/micro-editor/tcell/v2"
	"github.com/zyedidia/kbd/cbind"
	"github.com/zyedidia/mu/pkg/input"
	"github.com/zyedidia/mu/pkg/output"
	"github.com/zyedidia/mu/pkg/shell"
	"github.com/zyedidia/mu/pkg/tclutil"
)

// --- Basic ---

func (e *Editor) Help() {
	for _, cmd := range commands {
		fmt.Fprintln(e.log, cmd.Doc)
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

func (e *Editor) Quit() error {
	if err := e.Active().Close(); err != nil {
		return err
	}

	i := e.cur
	e.panes[i].Unregister(e.interp)
	copy(e.panes[i:], e.panes[i+1:])
	e.panes[len(e.panes)-1] = nil
	e.panes = e.panes[:len(e.panes)-1]
	if !e.valid() && len(e.panes) > 0 {
		e.SetPane(len(e.panes) - 1)
	}
	return nil
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
			fmt.Fprintf(e.log, "[%d: %v]\n", i, b.Name())
		} else {
			fmt.Fprintf(e.log, "%d: %v\n", i, b.Name())
		}
	}
}

func (e *Editor) SetBuffer(name string) error {
	for i, b := range e.panes {
		if b.Name() == name {
			e.SetPane(i)
			return nil
		}
	}
	return fmt.Errorf("buffer '%s' not found", name)
}

func (e *Editor) SetBufferIdx(idx int) error {
	if idx < 0 || idx >= len(e.panes) {
		return fmt.Errorf("invalid buffer index: %d", idx)
	}
	e.SetPane(idx)
	return nil
}

func (e *Editor) NewBuffer() {
	e.MakePane()
	e.open(input.NewReader(strings.NewReader(""), "no name"), &output.Discard{})
}

// --- Display ---

func (e *Editor) Refresh() {
	e.infobar.Clear()
}

// --- Commands ---

func (e *Editor) Command() error {
	out, canceled := e.infobar.Prompt("> ")
	if canceled {
		return nil
	}
	s, err := e.EvalRet(out, nil)
	if s != "" && err == nil {
		e.infobar.Message(s)
	}
	return err
}

func (e *Editor) Shell() {
	cmd, cancel := e.infobar.Prompt("$ ")
	if cancel {
		return
	}
	run := func() {
		err := shell.Run(cmd)
		if err != nil {
			e.infobar.Error(err.Error())
		}
	}
	e.Suspend <- run
}

func (e *Editor) Run(args []string) error {
	// TODO: use selection as stdin?
	cmd := strings.Join(args, " ")
	go func() {
		err := shell.RunWith(cmd, os.Stdin, e.log, e.log)
		e.displayLock.Lock()
		defer e.SendRedraw()
		defer e.displayLock.Unlock()
		if err != nil {
			e.Error(err.Error())
		} else {
			e.Message("completed: " + cmd)
		}
	}()
	return nil
}

// --- Options ---

func (e *Editor) Opt(name string, val string) error {
	if v, err := strconv.Atoi(val); err == nil {
		return e.setOpt(name, v)
	} else if v, err := strconv.ParseBool(val); err == nil {
		return e.setOpt(name, v)
	}
	return e.setOpt(name, val)
}

func (e *Editor) setOpt(name string, val interface{}) error {
	if e.config.IsGlobalOpt(name) {
		return e.config.SetGlobalOpt(name, val)
	}
	return e.panes[e.cur].SetOpt(name, val)
}

func (e *Editor) Get(name string) (string, error) {
	if e.config.IsGlobalOpt(name) {
		return fmt.Sprintf("%v", e.config.MustGlobalOpt(name)), nil
	}
	v, ok := e.panes[e.cur].GetOpt(name)
	if !ok {
		return "", fmt.Errorf("option %s not found", name)
	}
	return fmt.Sprintf("%v", v), nil
}

// --- Key events ---

func (e *Editor) Key(ev string) error {
	evs := strings.Split(ev, " ")
	for _, ev := range evs {
		mod, key, ch, err := cbind.Decode(ev)
		if err != nil {
			return err
		}
		kev := tcell.NewEventKey(key, ch, mod)
		e.HandleEvent(kev)
	}
	return nil
}

// --- Information ---

func (e *Editor) Mode() string {
	return e.GetMode()
}

var commands = []tclutil.Command{
	{
		"mode",
		(*Editor).Mode,
		"mode: return the current mode",
	},
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
	{
		"key",
		(*Editor).Key,
		"key <event>: execute <event> as if it had been typed in",
	},
	{
		"command",
		(*Editor).Command,
		"command: open a command prompt",
	},
	{
		"shell",
		(*Editor).Shell,
		"shell: open a shell prompt",
	},
	{
		"run",
		(*Editor).Run,
		"run: run a shell command in the background",
	},
	{
		"refresh",
		(*Editor).Refresh,
		"refresh: refresh the display",
	},
}
