package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const paletteLimit = 200

type paletteItem struct {
	label  string
	action func()
}

// Palette is the small amount of state shared by all picker modes.
type Palette struct {
	active   bool
	items    []paletteItem
	selected int
	filter   func(string) []paletteItem
}

func init() {
	editorCommands = append(editorCommands, CommandDef{
		"palette", cmdPalette,
		"palette [files|text|buffers|commands]: open the searchable palette",
	})
}

func cmdPalette(e *Editor, args []string) error {
	mode := ""
	if len(args) > 0 {
		mode = args[0]
	}
	return e.startPalette(mode)
}

func (e *Editor) startPalette(mode string) error {
	var prompt string
	var filter func(string) []paletteItem

	switch mode {
	case "":
		prompt = "Palette> "
		filter = e.paletteModes
	case "files", "file":
		prompt = "Files> "
		filter = filterPaletteItems(e.fileItems("."))
	case "buffers", "buffer":
		prompt = "Buffers> "
		filter = filterPaletteItems(e.bufferItems())
	case "commands", "command":
		prompt = "Commands> "
		filter = filterPaletteItems(e.commandItems())
	case "text", "grep":
		prompt = "Text> "
		files := paletteFiles(".")
		filter = func(query string) []paletteItem { return e.textItems(".", files, query) }
	default:
		return fmt.Errorf("palette: unknown mode %q", mode)
	}

	e.palette = Palette{active: true, filter: filter}
	e.infobar.StartPromptIncremental(prompt, e.updatePalette, nil, func() {
		e.palette = Palette{}
	})
	e.updatePalette("")
	return nil
}

func (e *Editor) paletteModes(query string) []paletteItem {
	modes := []struct{ name, mode string }{
		{"Files — search file names", "files"},
		{"Text — search file contents", "text"},
		{"Buffers — search open buffers", "buffers"},
		{"Commands — run an editor command", "commands"},
	}
	items := make([]paletteItem, 0, len(modes))
	for _, m := range modes {
		mode := m.mode
		items = append(items, paletteItem{m.name, func() { _ = e.startPalette(mode) }})
	}
	return filterPaletteItems(items)(query)
}

func (e *Editor) updatePalette(query string) {
	if !e.palette.active || e.palette.filter == nil {
		return
	}
	e.palette.items = e.palette.filter(query)
	if e.palette.selected >= len(e.palette.items) {
		e.palette.selected = len(e.palette.items) - 1
	}
	if e.palette.selected < 0 {
		e.palette.selected = 0
	}
}

func (e *Editor) handlePaletteKey(key string) {
	switch key {
	case KeyUp, "<C-p>":
		if n := len(e.palette.items); n > 0 {
			e.palette.selected = (e.palette.selected + n - 1) % n
		}
	case KeyDown, "<C-n>", KeyTab:
		if n := len(e.palette.items); n > 0 {
			e.palette.selected = (e.palette.selected + 1) % n
		}
	case KeyEnter:
		if len(e.palette.items) == 0 {
			return
		}
		action := e.palette.items[e.palette.selected].action
		e.infobar.Cancel()
		if action != nil {
			action()
		}
	default:
		e.infobar.HandleKey(key)
	}
}

func filterPaletteItems(items []paletteItem) func(string) []paletteItem {
	return func(query string) []paletteItem {
		type match struct {
			item  paletteItem
			score int
		}
		matches := make([]match, 0, len(items))
		for _, item := range items {
			if score, ok := fuzzyScore(query, item.label); ok {
				matches = append(matches, match{item, score})
			}
		}
		sort.SliceStable(matches, func(i, j int) bool {
			return matches[i].score < matches[j].score
		})
		if len(matches) > paletteLimit {
			matches = matches[:paletteLimit]
		}
		out := make([]paletteItem, len(matches))
		for i := range matches {
			out[i] = matches[i].item
		}
		return out
	}
}

// fuzzyScore matches query as a case-insensitive subsequence. Lower is better;
// adjacent and word-start matches naturally rank first.
func fuzzyScore(query, candidate string) (int, bool) {
	queryLower := strings.ToLower(query)
	candidateLower := strings.ToLower(candidate)
	q := []rune(queryLower)
	c := []rune(candidateLower)
	if len(q) == 0 {
		return 0, true
	}
	qi, last, score := 0, -2, 0
	for i, r := range c {
		if qi == len(q) {
			break
		}
		if r != q[qi] {
			continue
		}
		score += i
		if i == last+1 {
			score -= 3
		}
		if i == 0 || strings.ContainsRune("/._- ", c[i-1]) {
			score -= 2
		}
		last = i
		qi++
	}
	if qi != len(q) {
		return score, false
	}
	// Buffer labels are absolute paths. An exact filename query should beat
	// an accidental subsequence in a platform-specific temporary directory
	// prefix (notably macOS's long /var/folders paths).
	if candidateLower == queryLower {
		score -= 20000
	} else if strings.ToLower(filepath.Base(candidate)) == queryLower {
		score -= 10000
	}
	return score, true
}

func paletteFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if rel, err := filepath.Rel(root, path); err == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func (e *Editor) fileItems(root string) []paletteItem {
	files := paletteFiles(root)
	items := make([]paletteItem, 0, len(files))
	for _, name := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		items = append(items, paletteItem{name, func() {
			e.pushJump()
			if err := e.OpenFile(path); err != nil {
				e.infobar.Error(err.Error())
			}
		}})
	}
	return items
}

func (e *Editor) bufferItems() []paletteItem {
	items := make([]paletteItem, 0, len(e.buffers))
	for _, b := range e.buffers {
		buf := b
		items = append(items, paletteItem{bufDisplayName(buf), func() {
			e.pushJump()
			e.showBuffer(buf)
		}})
	}
	return items
}

func (e *Editor) commandItems() []paletteItem {
	items := make([]paletteItem, 0, len(editorCommands))
	for _, cmd := range editorCommands {
		name, label := cmd.Name, cmd.Doc
		items = append(items, paletteItem{label, func() {
			e.infobar.StartPrompt(":", func(input string) { e.RunCommand(input) })
			e.infobar.SetCompleter(cmdCompleter(e))
			e.infobar.input = []rune(name + " ")
			e.infobar.cursorPos = len(e.infobar.input)
		}})
	}
	return items
}

func (e *Editor) textItems(root string, files []string, query string) []paletteItem {
	if query == "" {
		return nil
	}
	needle := strings.ToLower(query)
	var items []paletteItem
	for _, name := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		data, err := os.ReadFile(path)
		if err != nil || !utf8.Valid(data) {
			continue
		}
		content := string(data)
		if strings.IndexByte(content, 0) >= 0 {
			continue
		}
		for line, text := range strings.Split(content, "\n") {
			if !strings.Contains(strings.ToLower(text), needle) {
				continue
			}
			lineNum := line
			label := fmt.Sprintf("%s:%d: %s", name, line+1, strings.TrimSpace(text))
			items = append(items, paletteItem{label, func() {
				e.pushJump()
				if err := e.OpenFile(path); err != nil {
					e.infobar.Error(err.Error())
					return
				}
				b := e.ActiveView().buf
				*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(lineNum, 0))
			}})
			if len(items) == paletteLimit {
				return items
			}
		}
	}
	return items
}

func (e *Editor) drawPalette() {
	n := len(e.palette.items)
	if n > 10 {
		n = 10
	}
	if n > e.h-2 {
		n = e.h - 2
	}
	if n <= 0 {
		return
	}
	start := e.palette.selected - n + 1
	if start < 0 {
		start = 0
	}
	y := e.h - 1 - n
	for i := 0; i < n; i++ {
		idx := start + i
		style := e.theme.Default()
		if idx == e.palette.selected {
			style = style.Add(AttrReverse)
		}
		runes := []rune(e.palette.items[idx].label)
		for x := 0; x < e.w; x++ {
			r := ' '
			if x < len(runes) {
				r = runes[x]
			}
			e.screen.SetContent(x, y+i, r, nil, style.TCellStyle())
		}
	}
}
