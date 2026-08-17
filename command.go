package main

import (
	"fmt"
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
	{"write!", cmdForceWrite, "write! [filename]: save, overriding read-only and existing-file checks"},
	{"writeall", cmdWriteAll, "writeall: save all modified buffers"},
	{"edit", cmdEdit, "edit [filename]: open a file, or reload the current one"},
	{"edit!", cmdForceEdit, "edit! [filename]: reload, discarding unsaved changes"},
	{"set", cmdSet, "set <name> [value]: get or set an option"},
	{"substitute", cmdSubstitute, "substitute <pattern> <replacement>: replace all matches in buffer"},
	{"vsplit", cmdVSplit, "vsplit [filename]: vertical split"},
	{"hsplit", cmdHSplit, "hsplit [filename]: horizontal split"},
	{"split", cmdHSplit, "split [filename]: horizontal split (alias)"},
	{"tabnew", cmdTabNew, "tabnew [filename]: open file in new tab"},
	{"tabnext", cmdTabNext, "tabnext: switch to next tab"},
	{"tabprev", cmdTabPrev, "tabprev: switch to previous tab"},
	{"goto", cmdGoto, "goto <line>: go to line number"},
	{"ls", cmdLs, "ls: list buffers"},
	{"buffers", cmdLs, "buffers: list buffers"},
	{"buffer", cmdBuffer, "buffer <n|name|#>: show a buffer in this pane"},
	{"bnext", cmdBNext, "bnext: next buffer"},
	{"bprev", cmdBPrev, "bprev: previous buffer"},
	{"bdelete", cmdBDelete, "bdelete [n|name]: remove a buffer from the buffer list"},
	{"bdelete!", cmdForceBDelete, "bdelete! [n|name]: remove a buffer, discarding unsaved changes"},
	{"jumps", cmdJumps, "jumps: list the jump list"},
	{"map", makeMapCmd(mapModeSets["map"]), "map <keys> <expansion>: map keys in normal/visual/pending modes (non-recursive)"},
	{"nmap", makeMapCmd(mapModeSets["nmap"]), "nmap <keys> <expansion>: map keys in normal mode"},
	{"vmap", makeMapCmd(mapModeSets["vmap"]), "vmap <keys> <expansion>: map keys in visual modes"},
	{"imap", makeMapCmd(mapModeSets["imap"]), "imap <keys> <expansion>: map keys in insert mode"},
	{"omap", makeMapCmd(mapModeSets["omap"]), "omap <keys> <expansion>: map keys in operator-pending mode"},
	{"unmap", makeUnmapCmd(mapModeSets["map"]), "unmap <keys>: remove a normal/visual/pending mode mapping"},
	{"nunmap", makeUnmapCmd(mapModeSets["nmap"]), "nunmap <keys>: remove a normal mode mapping"},
	{"vunmap", makeUnmapCmd(mapModeSets["vmap"]), "vunmap <keys>: remove a visual mode mapping"},
	{"iunmap", makeUnmapCmd(mapModeSets["imap"]), "iunmap <keys>: remove an insert mode mapping"},
	{"ounmap", makeUnmapCmd(mapModeSets["omap"]), "ounmap <keys>: remove an operator-pending mode mapping"},
}

// vimAliases maps vim-style short commands to TCL command strings.
var vimAliases = map[string]string{
	"q":       "quit",
	"q!":      "quit!",
	"qa":      "quitall",
	"qa!":     "quitall!",
	"w":       "write",
	"w!":      "write!",
	"wa":      "writeall",
	"wq":      "write; quit",
	"wq!":     "write!; quit",
	"x":       "write; quit",
	"x!":      "write!; quit",
	"wqa":     "writeall; quitall",
	"wqa!":    "writeall; quitall!",
	"xa":      "writeall; quitall",
	"xa!":     "writeall; quitall!",
	"e":       "edit",
	"e!":      "edit!",
	"s":       "substitute",
	"vs":      "vsplit",
	"sp":      "split",
	"vsp":     "vsplit",
	"tabe":    "tabnew",
	"tabedit": "tabnew",
	"b":       "buffer",
	"bn":      "bnext",
	"bp":      "bprev",
	"bd":      "bdelete",
	"bd!":     "bdelete!",
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
	// Mid-session, closing a pane just hides its buffer (which keeps any
	// unsaved changes in the buffer list). Closing the last pane exits, so
	// no buffer anywhere may have unsaved changes.
	if e.lastPane() {
		if mb := e.anyModifiedBuffer(); mb != nil {
			return fmt.Errorf("No write since last change for %s (use :q! to override)", bufDisplayName(mb))
		}
	}
	e.ClosePane()
	return nil
}

func cmdForceQuit(e *Editor, args []string) error {
	e.ClosePane()
	return nil
}

func cmdQuitAll(e *Editor, args []string) error {
	// Check every listed buffer (hidden ones included) for unsaved changes.
	if mb := e.anyModifiedBuffer(); mb != nil {
		return fmt.Errorf("No write since last change for %s (use :qa! to override)", bufDisplayName(mb))
	}
	e.running = false
	return nil
}

func cmdForceQuitAll(e *Editor, args []string) error {
	e.running = false
	return nil
}

func cmdWrite(e *Editor, args []string) error {
	return writeCmd(e, args, false)
}

// cmdForceWrite is :w! — write, overriding the read-only check and the
// existing-file check for :w <path>.
func cmdForceWrite(e *Editor, args []string) error {
	return writeCmd(e, args, true)
}

func writeCmd(e *Editor, args []string, force bool) error {
	v := e.ActiveView()
	if v == nil {
		return fmt.Errorf("no buffer")
	}
	b := v.buf

	// :w <path> naming a different file writes a copy there; the buffer
	// keeps its own name and modified state (vim). Overwriting an existing
	// file this way requires ! (vim E13). Only an unnamed buffer adopts
	// the argument as its file name — and never one already open in
	// another buffer, which would leave two buffers claiming one file.
	if len(args) > 0 && b.Path != "" && !samePath(args[0], b.Path) {
		if !force && fileExists(args[0]) {
			return fmt.Errorf("file exists: %s (use :w! to override)", args[0])
		}
		if err := b.saveTo(args[0], force); err != nil {
			return err
		}
		e.infobar.Message(fmt.Sprintf("\"%s\" written", args[0]))
		return nil
	}
	if len(args) > 0 && b.Path == "" {
		if ob := e.findBuffer(args[0]); ob != nil {
			return fmt.Errorf("%s is open in another buffer", args[0])
		}
	}

	path := b.Path
	if len(args) > 0 {
		path = args[0]
	}
	if path == "" {
		return fmt.Errorf("no file name")
	}

	// Check if file is readonly before attempting save; :w! skips the
	// prompt and forces the write directly.
	if !force && fileExists(path) && isReadonly(path) {
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

	if err := b.saveAs(path, force); err != nil {
		return err
	}
	// Persist undo history after successful save.
	if saveUndo, _ := GetOptBool(e.config.opts.top, "saveundo"); saveUndo {
		b.SaveUndoHistory()
	}
	e.infobar.Message(fmt.Sprintf("\"%s\" written", b.Path))
	return nil
}

// cmdWriteAll writes every modified buffer that has a file name (vim :wa).
// Buffers that cannot be written (no name, read-only, I/O error) are
// reported but don't stop the others from being written; the returned error
// makes a compound like "writeall; quitall" (:wqa) refuse to quit.
func cmdWriteAll(e *Editor, args []string) error {
	saveUndo, _ := GetOptBool(e.config.opts.top, "saveundo")
	written := 0
	var firstErr error
	for _, b := range e.buffers {
		if !b.Modified() {
			continue
		}
		if err := b.Save(); err != nil {
			if firstErr == nil {
				if b.Path != "" {
					err = fmt.Errorf("%s: %v", b.Path, err)
				}
				firstErr = err
			}
			continue
		}
		if saveUndo {
			b.SaveUndoHistory()
		}
		written++
	}
	if firstErr != nil {
		return firstErr
	}
	if written == 1 {
		e.infobar.Message("1 buffer written")
	} else if written > 1 {
		e.infobar.Message(fmt.Sprintf("%d buffers written", written))
	}
	return nil
}

func cmdEdit(e *Editor, args []string) error {
	return editCmd(e, args, false)
}

// cmdForceEdit is :e! — reload, discarding unsaved changes.
func cmdForceEdit(e *Editor, args []string) error {
	return editCmd(e, args, true)
}

func editCmd(e *Editor, args []string, force bool) error {
	v := e.ActiveView()
	// :e with no filename, or naming the file already shown in this pane,
	// reloads it from disk (vim), refusing to drop unsaved changes unless
	// forced.
	reload := len(args) == 0
	if !reload && v != nil && v.buf.Path != "" && samePath(v.buf.Path, args[0]) {
		reload = true
	}
	if reload {
		if v == nil || v.buf.Path == "" {
			return fmt.Errorf("edit: no file name")
		}
		if !force && v.buf.Modified() {
			return fmt.Errorf("No write since last change (use :e! to override)")
		}
		if err := v.buf.Reload(); err != nil {
			return err
		}
		e.infobar.Message(fmt.Sprintf("\"%s\" reloaded", v.buf.Path))
		return nil
	}
	e.pushJump()
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
			opts := v.Opts
			if opts == nil {
				opts = e.config.BufferOptions(v.buf.Path, v.buf.Filetype)
			}
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
			opts := e.config.BufferOptions(v.buf.Path, v.buf.Filetype)
			coerced, err = coerceOptValue(opts[name], value)
		}
		if err != nil {
			// No existing value to match; store as string.
			coerced = value
		}
	}

	if IsGlobalOpt(name) {
		// Validate a theme before storing the option so a typo doesn't
		// leave a broken value behind.
		if name == "theme" {
			th, loadErr := e.config.LoadTheme(value)
			if loadErr != nil {
				return fmt.Errorf("theme: %w", loadErr)
			}
			if err := e.config.SetGlobalOpt(name, coerced); err != nil {
				return err
			}
			e.theme = th
			if e.screen != nil {
				e.screen.SetStyle(th.Default().TCellStyle())
			}
		} else if err := e.config.SetGlobalOpt(name, coerced); err != nil {
			return err
		}
		// Reconnect the clipboard registers when the mode changes.
		if name == "clipboard" {
			if err := e.initClipboard(); err != nil {
				return err
			}
		}
	} else {
		// Buffer-scoped option: update the top-level default and re-apply
		// options to all open views.
		if err := e.config.SetGlobalOpt(name, coerced); err != nil {
			return err
		}
		e.refreshViewOptions()
		// Tear down or (re)build highlighting state to match.
		if name == "syntax" {
			e.applySyntaxOption()
		}
		// Attach or detach language servers to match.
		if name == "lsp" {
			e.applyLspOption()
		}
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
	if len(args) > 0 {
		return e.OpenFileInTab(args[0])
	}
	buf := NewEmptyBuffer()
	e.NewTabWithView(e.configureView(buf, ""))
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

// --- Buffer list commands ---

// resolveBuffer maps a :buffer/:bdelete argument to a listed buffer:
// a buffer number, "#" for the alternate buffer, or a unique substring of
// a buffer's path.
func (e *Editor) resolveBuffer(arg string) (*Buffer, error) {
	if arg == "#" {
		if e.altBuf == nil {
			return nil, fmt.Errorf("no alternate buffer")
		}
		return e.altBuf, nil
	}
	if n, err := strconv.Atoi(arg); err == nil {
		for _, b := range e.buffers {
			if b.bufnum == n {
				return b, nil
			}
		}
		return nil, fmt.Errorf("buffer %d does not exist", n)
	}
	var matches []*Buffer
	for _, b := range e.buffers {
		if b.Path != "" && strings.Contains(b.Path, arg) {
			matches = append(matches, b)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("no matching buffer for %q", arg)
	default:
		return nil, fmt.Errorf("more than one match for %q", arg)
	}
}

func cmdLs(e *Editor, args []string) error {
	var cur *Buffer
	if v := e.ActiveView(); v != nil {
		cur = v.buf
	}
	var parts []string
	for _, b := range e.buffers {
		marks := ""
		if b == cur {
			marks += "%"
		} else if b == e.altBuf {
			marks += "#"
		}
		if b.Modified() {
			marks += "+"
		}
		parts = append(parts, fmt.Sprintf("%d%s %s", b.bufnum, marks, bufDisplayName(b)))
	}
	e.infobar.Message(strings.Join(parts, "  "))
	return nil
}

func cmdBuffer(e *Editor, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("buffer: usage: buffer <n|name|#>")
	}
	b, err := e.resolveBuffer(args[0])
	if err != nil {
		return err
	}
	e.pushJump()
	e.showBuffer(b)
	return nil
}

// cycleBuffer shows the listed buffer dir steps away from the current one.
func (e *Editor) cycleBuffer(dir int) error {
	v := e.ActiveView()
	if v == nil || len(e.buffers) == 0 {
		return fmt.Errorf("no buffers")
	}
	idx := -1
	for i, b := range e.buffers {
		if b == v.buf {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	}
	n := len(e.buffers)
	e.pushJump()
	e.showBuffer(e.buffers[((idx+dir)%n+n)%n])
	return nil
}

func cmdBNext(e *Editor, args []string) error {
	return e.cycleBuffer(1)
}

func cmdBPrev(e *Editor, args []string) error {
	return e.cycleBuffer(-1)
}

func cmdBDelete(e *Editor, args []string) error {
	return bdeleteCmd(e, args, false)
}

// cmdForceBDelete is :bd! — remove a buffer, discarding unsaved changes.
func cmdForceBDelete(e *Editor, args []string) error {
	return bdeleteCmd(e, args, true)
}

func bdeleteCmd(e *Editor, args []string, force bool) error {
	var b *Buffer
	if len(args) > 0 {
		var err error
		if b, err = e.resolveBuffer(args[0]); err != nil {
			return err
		}
	} else if v := e.ActiveView(); v != nil {
		b = v.buf
	}
	if b == nil {
		return fmt.Errorf("no buffer")
	}
	if !force && b.Modified() {
		return fmt.Errorf("No write since last change for %s (use :bd! to override)", bufDisplayName(b))
	}
	e.deleteBuffer(b)
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
	e.pushJump()
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
