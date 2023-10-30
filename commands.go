package mu

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/micro-editor/tcell/v2"
	"github.com/zyedidia/kbd/cbind"
	"github.com/zyedidia/mu/build"
	"github.com/zyedidia/mu/pane/buf"
	"github.com/zyedidia/mu/pane/term"
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

func (e *Editor) Term(args []string) (err error) {
	var tp *term.TermPane
	if len(args) == 0 {
		tp, err = term.NewTermPaneShell(e.Redraw)
	} else {
		tp, err = term.NewTermPane(e.Redraw, args[0], args[1:]...)
	}
	if err != nil {
		return err
	}
	e.ActiveTab().Open(e, tp)
	return nil
}

func (e *Editor) Open(path string) error {
	bp, err := e.NewBufPaneFromPath(path)
	if err != nil {
		return err
	}
	return e.ActiveTab().Open(e, bp)
}

func (e *Editor) Tab(args []string) (err error) {
	var bp *buf.BufPane
	if len(args) == 0 {
		bp = e.NewEmptyBufPane()
	} else {
		bp, err = e.NewBufPaneFromPath(args[0])
	}
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

func (e *Editor) VSplit(args []string) (err error) {
	var bp *buf.BufPane
	if len(args) == 0 {
		bp = e.NewEmptyBufPane()
	} else {
		bp, err = e.NewBufPaneFromPath(args[0])
	}
	if err != nil {
		return err
	}
	e.ActiveTab().VSplit(e, bp)
	return nil
}

func (e *Editor) HSplit(args []string) (err error) {
	var bp *buf.BufPane
	if len(args) == 0 {
		bp = e.NewEmptyBufPane()
	} else {
		bp, err = e.NewBufPaneFromPath(args[0])
	}
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

func (e *Editor) QuitAll() error {
	for len(e.tabs) > 0 {
		err := e.Quit()
		if err != nil {
			return err
		}
	}
	return nil
}

// --- Display ---

func (e *Editor) Refresh() {
	e.infobar.Clear()
}

func (e *Editor) Print(s string) {
	e.infobar.Message(s)
}

// --- Commands ---

func (e *Editor) Command() error {
	out, canceled := e.infobar.Prompt("cmd", "> ")
	if canceled {
		return nil
	}
	s, err := e.EvalRet(out, nil)
	if s != "" && err == nil {
		e.infobar.Message(s)
	}
	return err
}

func (e *Editor) CommandEdit(p string) error {
	out, canceled := e.infobar.prompt("cmd", "> ", p, "cmd", nil)
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
	cmd, cancel := e.infobar.Prompt("shell", "$ ")
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
	} else if v, err := parseBoolOpt(val); err == nil {
		return e.setOpt(name, v)
	}
	return e.setOpt(name, val)
}

func parseBoolOpt(val string) (bool, error) {
	switch val {
	case "on", "ON":
		return true, nil
	case "off", "OFF":
		return false, nil
	}
	return strconv.ParseBool(val)
}

func (e *Editor) setOpt(name string, val interface{}) error {
	if e.config.IsGlobalOpt(name) {
		return e.config.SetGlobalOpt(name, val)
	}
	return e.ActivePane().SetOpt(name, val)
}

func (e *Editor) Get(name string) (string, error) {
	if e.config.IsGlobalOpt(name) {
		return fmt.Sprintf("%v", e.config.GlobalOpt(name)), nil
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

func (e *Editor) Version() string {
	return build.Version
}

var commands = []tclutil.Command{
	{
		Name: "mode",
		Fn:   (*Editor).Mode,
		Doc:  "mode: return the current mode",
	},
	{
		Name: "open",
		Fn:   (*Editor).Open,
		Doc:  "open <file>: open <file> as a buffer in the current pane",
	},
	{
		Name: "quit",
		Fn:   (*Editor).Quit,
		Doc:  "quit: close the current pane",
	},
	{
		Name: "quit-all",
		Fn:   (*Editor).QuitAll,
		Doc:  "quit-all: close all panes",
	},
	{
		Name: "opt",
		Fn:   (*Editor).Opt,
		Doc:  "opt <name> <val>: assign option <name> to <val>",
	},
	{
		Name: "get",
		Fn:   (*Editor).Get,
		Doc:  "get <name>: return the value of option <name>",
	},
	{
		Name: "key",
		Fn:   (*Editor).Key,
		Doc:  "key <event>: execute <event> as if it had been typed in",
	},
	{
		Name: "command",
		Fn:   (*Editor).Command,
		Doc:  "command: open a command prompt",
	},
	{
		Name: "command-edit",
		Fn:   (*Editor).CommandEdit,
		Doc:  "command <cmd>: open a command prompt with a placeholder command",
	},
	{
		Name: "shell",
		Fn:   (*Editor).Shell,
		Doc:  "shell: open a shell prompt",
	},
	{
		Name: "run",
		Fn:   (*Editor).Run,
		Doc:  "run: run a shell command in the background",
	},
	{
		Name: "refresh",
		Fn:   (*Editor).Refresh,
		Doc:  "refresh: refresh the display",
	},
	{
		Name: "tab",
		Fn:   (*Editor).Tab,
		Doc:  "tab <path>: open new tab",
	},
	{
		Name: "tab-next",
		Fn:   (*Editor).TabNext,
		Doc:  "tab-next: select next tab",
	},
	{
		Name: "tab-prev",
		Fn:   (*Editor).TabPrev,
		Doc:  "tab-prev: select previous tab",
	},
	{
		Name: "vsplit",
		Fn:   (*Editor).VSplit,
		Doc:  "vsplit <path>: open a new vertical split",
	},
	{
		Name: "hsplit",
		Fn:   (*Editor).HSplit,
		Doc:  "hsplit <path>: open a new horizontal split",
	},
	{
		Name: "split-select-next",
		Fn:   (*Editor).SplitSelectNext,
		Doc:  "split-select-next: select the next split",
	},
	{
		Name: "term",
		Fn:   (*Editor).Term,
		Doc:  "term <cmd>: run cmd in a terminal pane",
	},
	{
		Name: "version",
		Fn:   (*Editor).Version,
		Doc:  "version: returns the version number",
	},
	{
		Name: "print",
		Fn:   (*Editor).Print,
		Doc:  "print: displays a value",
	},
}
