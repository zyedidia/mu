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
	e.ActivePane().Help(os.Stdout)
}

// --- Buffer management ---

func (e *Editor) Open(path string) error {
	in := &input.File{
		Path: path,
	}
	out := &output.File{
		Path: path,
	}
	bp, err := e.NewBufPane(in, out)
	if err != nil {
		return err
	}
	e.ActiveTab().Open(e, bp)
	return nil
}

func (e *Editor) Tab(path string) error {
	bp, err := e.NewBufPaneFromPath(path)
	if err != nil {
		return err
	}
	e.OpenTabPane(bp)
	return nil
}

func (e *Editor) TabNext() {
	if e.curtab < len(e.tabs)-1 {
		e.curtab++
		e.ActivatePane(e.ActiveTab().ActivePane())
	}
}

func (e *Editor) TabPrev() {
	if e.curtab > 0 {
		e.curtab--
		e.ActivatePane(e.ActiveTab().ActivePane())
	}
}

func (e *Editor) VSplit(path string) error {
	bp, err := e.NewBufPaneFromPath(path)
	if err != nil {
		return err
	}
	e.ActiveTab().VSplit(e, bp)
	return nil
}

func (e *Editor) HSplit(path string) error {
	bp, err := e.NewBufPaneFromPath(path)
	if err != nil {
		return err
	}
	e.ActiveTab().HSplit(e, bp)
	return nil
}

func (e *Editor) SplitSelectNext() {
	e.ActiveTab().next()
	e.ActivatePane(e.ActiveTab().ActivePane())
}

func (e *Editor) SplitSelectRight() {
}

func (e *Editor) SplitSelectLeft() {
}

func (e *Editor) SplitSelectUp() {
}

func (e *Editor) SplitSelectDown() {
}

func (e *Editor) Quit() error {
	if err := e.ActivePane().Close(); err != nil {
		return err
	}

	if len(e.ActiveTab().panes) <= 1 {
		e.CloseTabPane()
	} else {
		e.ActiveTab().Unsplit(e)
	}

	return nil
}

func (e *Editor) QuitAll() {
	// ln := len(e.tabs)
	// for _, t := range e.tabs {
	// 	t.Quit()
	// }
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
		err := shell.Run(cmd, true)
		if err != nil {
			e.infobar.Error(err.Error())
		}
		shell.EnterToContinue()
	}
	e.Suspend <- run
	e.Resume <- struct{}{}
}

func (e *Editor) Run(args []string) error {
	// TODO: use selection as stdin?
	cmd := strings.Join(args, " ")
	go func() {
		err := shell.RunWith(cmd, os.Stdin, e.log, e.log, false)
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
	return e.ActivePane().SetOpt(name, val)
}

func (e *Editor) Get(name string) (string, error) {
	if e.config.IsGlobalOpt(name) {
		return fmt.Sprintf("%v", e.config.MustGlobalOpt(name)), nil
	}
	v, ok := e.ActivePane().GetOpt(name)
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
	{
		"tab",
		(*Editor).Tab,
		"tab <path>: open new tab",
	},
	{
		"tab-next",
		(*Editor).TabNext,
		"tab-next: select next tab",
	},
	{
		"tab-prev",
		(*Editor).TabPrev,
		"tab-prev: select previous tab",
	},
	{
		"vsplit",
		(*Editor).VSplit,
		"vsplit <path>: open a new vertical split",
	},
	{
		"hsplit",
		(*Editor).HSplit,
		"hsplit <path>: open a new horizontal split",
	},
	{
		"split-select-next",
		(*Editor).SplitSelectNext,
		"split-select-next: select the next split",
	},
}
