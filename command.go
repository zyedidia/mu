package main

import (
	"fmt"
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
	{"quit", cmdQuit, "quit: close the editor"},
	{"quit!", cmdForceQuit, "quit!: close without saving"},
	{"write", cmdWrite, "write [filename]: save the buffer"},
	{"edit", cmdEdit, "edit <filename>: open a file"},
	{"set", cmdSet, "set <name> [value]: get or set an option"},
	{"substitute", cmdSubstitute, "substitute <pattern> <replacement>: replace all matches in buffer"},
}

// vimAliases maps vim-style short commands to TCL command strings.
var vimAliases = map[string]string{
	"q":  "quit",
	"q!": "quit!",
	"w":  "write",
	"wq": "write; quit",
	"x":  "write; quit",
	"e":  "edit",
	"s":  "substitute",
}

// RunCommand parses and executes an ex command string. It expands vim
// aliases, then evaluates the result as TCL.
func (e *Editor) RunCommand(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
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
	e.running = false
	return nil
}

func cmdForceQuit(e *Editor, args []string) error {
	e.running = false
	return nil
}

func cmdWrite(e *Editor, args []string) error {
	// TODO: implement saving (step 12)
	v := e.ActiveView()
	if v == nil {
		return fmt.Errorf("no buffer")
	}
	path := v.buf.Path
	if len(args) > 0 {
		path = args[0]
	}
	if path == "" {
		return fmt.Errorf("no file name")
	}
	e.infobar.Message(fmt.Sprintf("TODO: write %q not implemented yet", path))
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
