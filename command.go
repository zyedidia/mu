package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CommandDef defines an ex command.
type CommandDef struct {
	Name string
	Fn   func(e *Editor, args []string) error
	Doc  string
}

// editorCommands is the registry of all ex commands. These are registered
// as TCL commands in tcl.go.
var editorCommands = []CommandDef{
	{"quit", cmdQuit, "quit: close pane/tab/editor"},
	{"quit!", cmdForceQuit, "quit!: close without saving"},
	{"quitall", cmdQuitAll, "quitall: close all panes and tabs"},
	{"quitall!", cmdForceQuitAll, "quitall!: close all without saving"},
	{"write", cmdWrite, "write [filename]: save the buffer"},
	{"edit", cmdEdit, "edit <filename>: open a file"},
	{"set", cmdSet, "set <name> [value]: get or set an option"},
	{"substitute", cmdSubstitute, "substitute <pattern> <replacement>: replace all matches in buffer"},
	{"vsplit", cmdVSplit, "vsplit [filename]: vertical split"},
	{"hsplit", cmdHSplit, "hsplit [filename]: horizontal split"},
	{"split", cmdHSplit, "split [filename]: horizontal split (alias)"},
	{"tabnew", cmdTabNew, "tabnew [filename]: open file in new tab"},
	{"tabnext", cmdTabNext, "tabnext: switch to next tab"},
	{"tabprev", cmdTabPrev, "tabprev: switch to previous tab"},
	{"goto", cmdGoto, "goto <line>: go to line number"},
}

// vimAliases maps vim-style short commands to TCL command strings.
var vimAliases = map[string]string{
	"q":   "quit",
	"q!":  "quit!",
	"qa":  "quitall",
	"qa!": "quitall!",
	"w":   "write",
	"wq":  "write; quit",
	"x":   "write; quit",
	"e":   "edit",
	"s":   "substitute",
	"vs":  "vsplit",
	"sp":  "split",
	"vsp": "vsplit",
}

// RunCommand parses and executes an ex command string. It expands vim
// aliases, then evaluates the result as TCL. A bare number goes to that line.
func (e *Editor) RunCommand(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}

	// Bare number: go to line.
	if _, err := strconv.Atoi(input); err == nil {
		input = "goto " + input
	}

	expanded := expandAlias(input)

	err := e.EvalTCL(expanded)
	if err != nil {
		e.infobar.Error(err.Error())
	}
}

// expandAlias expands vim command aliases. For compound commands (e.g.
// "wq" → "write; quit"), only the alias is expanded; trailing args are
// appended to the first command.
func expandAlias(input string) string {
	parts := strings.SplitN(input, " ", 2)
	cmd := parts[0]
	args := ""
	if len(parts) > 1 {
		args = " " + parts[1]
	}

	if tcl, ok := vimAliases[cmd]; ok {
		// If the alias is a compound command (;), append args to first part.
		if strings.Contains(tcl, ";") && args != "" {
			cmds := strings.SplitN(tcl, ";", 2)
			return strings.TrimSpace(cmds[0]) + args + "; " + strings.TrimSpace(cmds[1])
		}
		return tcl + args
	}
	return input
}

// --- Command implementations ---

func cmdQuit(e *Editor, args []string) error {
	if e.ActiveView() != nil && e.ActiveView().buf.Modified() {
		return fmt.Errorf("No write since last change (use :q! to override)")
	}
	e.ClosePane()
	return nil
}

func cmdForceQuit(e *Editor, args []string) error {
	e.ClosePane()
	return nil
}

func cmdQuitAll(e *Editor, args []string) error {
	// Check all buffers for unsaved changes.
	for _, t := range e.tabs {
		for _, v := range t.panes {
			if v.buf.Modified() {
				return fmt.Errorf("No write since last change (use :qa! to override)")
			}
		}
	}
	e.running = false
	return nil
}

func cmdForceQuitAll(e *Editor, args []string) error {
	e.running = false
	return nil
}

func cmdWrite(e *Editor, args []string) error {
	v := e.ActiveView()
	if v == nil {
		return fmt.Errorf("no buffer")
	}
	b := v.buf
	path := b.Path
	if len(args) > 0 {
		path = args[0]
	}
	if path == "" {
		return fmt.Errorf("no file name")
	}

	// Check if file is readonly before attempting save.
	if fileExists(path) && isReadonly(path) {
		e.infobar.Prompt("File is read-only. Save with sudo? (y/n)", func(key string) {
			if key == "y" {
				if err := e.saveWithSudo(b, path); err != nil {
					e.infobar.Error(err.Error())
				} else {
					e.infobar.Message(fmt.Sprintf("\"%s\" written (sudo)", b.Path))
				}
			} else {
				e.infobar.Message("Save canceled")
			}
		})
		return nil
	}

	if err := b.SaveAs(path); err != nil {
		return err
	}
	// Persist undo history after successful save.
	if saveUndo, _ := GetOptBool(e.config.opts.top, "saveundo"); saveUndo {
		b.SaveUndoHistory()
	}
	e.infobar.Message(fmt.Sprintf("\"%s\" written", b.Path))
	return nil
}

func cmdEdit(e *Editor, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("edit: missing filename")
	}
	return e.OpenFile(args[0])
}

func cmdSet(e *Editor, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("set: usage: set <name> [value]")
	}
	name := args[0]

	// No value: show current setting.
	if len(args) == 1 {
		if IsGlobalOpt(name) {
			val := e.config.GlobalOpt(name)
			e.infobar.Message(fmt.Sprintf("%s=%v", name, val))
		} else if v := e.ActiveView(); v != nil {
			opts := e.config.BufferOptions(v.buf.Path, "")
			if val, ok := opts[name]; ok {
				e.infobar.Message(fmt.Sprintf("%s=%v", name, val))
			} else {
				return fmt.Errorf("unknown option: %s", name)
			}
		}
		return nil
	}

	value := args[1]

	// Coerce value to match existing type.
	coerced, err := coerceOptValue(e.config.GlobalOpt(name), value)
	if err != nil {
		// Try buffer options for type hint.
		if v := e.ActiveView(); v != nil {
			opts := e.config.BufferOptions(v.buf.Path, "")
			coerced, err = coerceOptValue(opts[name], value)
		}
		if err != nil {
			// No existing value to match; store as string.
			coerced = value
		}
	}

	if IsGlobalOpt(name) {
		if err := e.config.SetGlobalOpt(name, coerced); err != nil {
			return err
		}
		// Apply theme change immediately.
		if name == "theme" {
			th, loadErr := e.config.LoadTheme(value)
			if loadErr != nil {
				return fmt.Errorf("theme: %w", loadErr)
			}
			e.theme = th
			if e.screen != nil {
				e.screen.SetStyle(th.Default().TCellStyle())
			}
		}
	} else {
		// TODO: apply to active buffer view options
	}

	e.infobar.Message(fmt.Sprintf("%s=%v", name, coerced))
	return nil
}

// coerceOptValue converts a string value to match the type of an existing
// option value (bool, int64, string).
func coerceOptValue(existing any, s string) (any, error) {
	switch existing.(type) {
	case bool:
		switch s {
		case "true", "on", "1":
			return true, nil
		case "false", "off", "0":
			return false, nil
		}
		return nil, fmt.Errorf("expected bool")
	case int64:
		var n int64
		_, err := fmt.Sscanf(s, "%d", &n)
		return n, err
	case float64:
		var f float64
		_, err := fmt.Sscanf(s, "%f", &f)
		return f, err
	case string:
		return s, nil
	}
	return nil, fmt.Errorf("unknown type")
}

func cmdVSplit(e *Editor, args []string) error {
	e.VSplit(args)
	return nil
}

func cmdHSplit(e *Editor, args []string) error {
	e.HSplit(args)
	return nil
}

func cmdTabNew(e *Editor, args []string) error {
	var v *View
	if len(args) > 0 {
		data, err := os.ReadFile(args[0])
		if err != nil {
			if os.IsNotExist(err) {
				data = []byte{}
			} else {
				return err
			}
		}
		buf, err := NewBuffer(data, args[0])
		if err != nil {
			return err
		}
		v = e.configureView(buf, args[0])
	} else {
		buf := NewEmptyBuffer()
		v = e.configureView(buf, "")
	}
	e.NewTabWithView(v)
	return nil
}

func cmdTabNext(e *Editor, args []string) error {
	e.NextTab()
	return nil
}

func cmdTabPrev(e *Editor, args []string) error {
	e.PrevTab()
	return nil
}

func cmdGoto(e *Editor, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("goto: missing line number")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("goto: invalid line number %q", args[0])
	}
	v := e.ActiveView()
	if v == nil {
		return fmt.Errorf("no buffer")
	}
	b := v.buf
	line := n - 1
	if line < 0 {
		line = 0
	}
	if line > b.NumLines() {
		line = b.NumLines()
	}
	*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(line, 0))
	return nil
}
