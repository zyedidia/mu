package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Completer returns completion candidates for the current input.
type Completer func(input string) []string

// completionState tracks cycling through tab-completion candidates.
type completionState struct {
	candidates  []string
	replaceFrom int    // rune index in input where replacement starts
	origWord    string // the original word before any completion
	index       int    // -1 = original input, 0..n-1 = candidates
	active      bool
}

func (cs *completionState) reset() {
	cs.candidates = nil
	cs.origWord = ""
	cs.replaceFrom = 0
	cs.index = -1
	cs.active = false
}

// --- Command-line completion for the : prompt ---

// cmdCompleter returns a Completer for the : command prompt. It dispatches
// to command-name completion for the first word and argument completion
// for subsequent words based on the command.
func cmdCompleter(e *Editor) Completer {
	return func(input string) []string {
		parts := strings.SplitN(input, " ", 2)
		cmd := parts[0]

		if len(parts) == 1 {
			// Completing the command name.
			return completeCommandName(cmd)
		}

		// Completing an argument. Figure out which command and which arg.
		argStr := parts[1]
		return completeCommandArg(e, cmd, argStr)
	}
}

// completeCommandName returns command names and aliases matching the prefix.
func completeCommandName(prefix string) []string {
	var candidates []string
	seen := make(map[string]bool)

	// TCL command names.
	for _, cmd := range editorCommands {
		if strings.HasPrefix(cmd.Name, prefix) && !seen[cmd.Name] {
			candidates = append(candidates, cmd.Name)
			seen[cmd.Name] = true
		}
	}
	// Vim aliases.
	for alias := range vimAliases {
		if strings.HasPrefix(alias, prefix) && !seen[alias] {
			candidates = append(candidates, alias)
			seen[alias] = true
		}
	}

	sort.Strings(candidates)
	return candidates
}

// completeCommandArg returns argument completions based on the command.
func completeCommandArg(e *Editor, cmd, argStr string) []string {
	// Resolve alias to canonical command.
	canonical := cmd
	if expanded, ok := vimAliases[cmd]; ok {
		// Take the first word of the expansion.
		canonical = strings.Fields(expanded)[0]
	}

	args := strings.Fields(argStr)
	lastArg := ""
	if len(argStr) > 0 && !strings.HasSuffix(argStr, " ") {
		lastArg = args[len(args)-1]
	}
	argIndex := len(args)
	if lastArg != "" {
		argIndex = len(args) - 1
	}

	switch canonical {
	case "edit", "write", "vsplit", "hsplit", "split", "tabnew":
		return completeFilePath(lastArg)
	case "set":
		if argIndex == 0 {
			return completeOptionName(e, lastArg)
		}
		if argIndex == 1 && len(args) > 0 {
			return completeOptionValue(e, args[0], lastArg)
		}
	}

	return nil
}

// completeFilePath returns filesystem path completions.
func completeFilePath(prefix string) []string {
	// Expand ~ to home directory.
	expanded := prefix
	if strings.HasPrefix(expanded, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = filepath.Join(home, expanded[1:])
		}
	}

	dir := filepath.Dir(expanded)
	base := filepath.Base(expanded)
	if prefix == "" || strings.HasSuffix(prefix, string(filepath.Separator)) {
		dir = expanded
		base = ""
	}
	if dir == "" {
		dir = "."
	}

	var candidates []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, base) {
			continue
		}
		full := filepath.Join(dir, name)
		// Use the original prefix form for ~ paths.
		if strings.HasPrefix(prefix, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				full = "~" + strings.TrimPrefix(full, home)
			}
		} else {
			// Make relative if prefix was relative.
			if !filepath.IsAbs(prefix) {
				if rel, err := filepath.Rel(".", full); err == nil {
					full = rel
				}
			}
		}
		if entry.IsDir() {
			full += string(filepath.Separator)
		}
		candidates = append(candidates, full)
	}
	sort.Strings(candidates)
	return candidates
}

// completeOptionName returns option names matching the prefix.
func completeOptionName(e *Editor, prefix string) []string {
	var candidates []string
	seen := make(map[string]bool)

	// Global options.
	for name := range e.config.opts.top {
		if strings.HasPrefix(name, prefix) && !seen[name] {
			candidates = append(candidates, name)
			seen[name] = true
		}
	}
	// Buffer options from defaults.
	if v := e.ActiveView(); v != nil {
		for name := range e.config.BufferOptions(v.buf.Path, "") {
			if strings.HasPrefix(name, prefix) && !seen[name] {
				candidates = append(candidates, name)
				seen[name] = true
			}
		}
	}
	sort.Strings(candidates)
	return candidates
}

// completeOptionValue returns value completions for a specific option.
func completeOptionValue(e *Editor, optName, prefix string) []string {
	switch optName {
	case "theme":
		return completeThemeName(e.config, prefix)
	case "clipboard":
		return filterPrefix([]string{"internal", "external", "terminal"}, prefix)
	case "cursor":
		return filterPrefix([]string{"block", "bar", "underline"}, prefix)
	}

	// Check if the option is a bool.
	if val := e.config.GlobalOpt(optName); val != nil {
		if _, ok := val.(bool); ok {
			return filterPrefix([]string{"true", "false"}, prefix)
		}
	}
	if v := e.ActiveView(); v != nil {
		opts := e.config.BufferOptions(v.buf.Path, "")
		if val, ok := opts[optName]; ok {
			if _, isBool := val.(bool); isBool {
				return filterPrefix([]string{"true", "false"}, prefix)
			}
		}
	}

	return nil
}

// completeThemeName returns theme names matching the prefix, drawn from the
// user config directory as well as the embedded themes. A user theme shadows
// an embedded one of the same name (see Config.ReadFile), so each name is
// offered once no matter where it came from.
func completeThemeName(cfg *Config, prefix string) []string {
	var names []string
	seen := make(map[string]bool)
	collect := func(fsys fs.FS, dir string) {
		fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return nil
			}
			name := strings.TrimSuffix(filepath.Base(path), ".yaml")
			if strings.HasPrefix(name, prefix) && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
			return nil
		})
	}
	collect(os.DirFS(cfg.dir), "themes")
	collect(embedFS, "embed/themes")
	sort.Strings(names)
	return names
}

func filterPrefix(items []string, prefix string) []string {
	var out []string
	for _, s := range items {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}
