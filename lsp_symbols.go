package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lsp "go.lsp.dev/protocol"
)

// lspFindReferences requests all references to the symbol at cursor and
// opens a searchable palette of results once the asynchronous response
// arrives.
func (e *Editor) lspFindReferences() {
	v := e.ActiveView()
	if v == nil || v.buf.lspServer == nil {
		e.infobar.Error("No LSP server")
		return
	}
	b := v.buf
	s := b.lspServer
	line, col := b.LineColAt(b.Cursor().Pos)
	pos := b.LspPosition(line, col)
	absPath, _ := filepath.Abs(b.Path)
	e.infobar.Message("Loading references…")

	lspAsync(e, func() ([]lsp.Location, error) {
		return s.References(absPath, pos)
	}, func(locs []lsp.Location, err error) {
		// A late answer must not replace an active prompt with a palette.
		if av := e.ActiveView(); av == nil || av.buf != b || e.infobar.IsActive() {
			return
		}
		if err != nil {
			if err == ErrLspNotSupported {
				e.infobar.Error("Find references not supported")
			} else {
				e.infobar.Error(fmt.Sprintf("references: %v", err))
			}
			return
		}
		if len(locs) == 0 {
			e.infobar.Message("No references found")
			return
		}
		files := make(map[string][]string)
		items := make([]paletteItem, len(locs))
		for i, loc := range locs {
			loc := loc
			items[i] = paletteItem{label: e.locationLabel(files, loc), action: func() {
				e.pushJump()
				e.jumpToLspLocation(b, loc)
			}}
		}
		e.startPaletteItems("References> ", items)
	})
}

// locationLabel renders a Location as "path:line: text" for a palette.
// Line text comes from the open buffer when the file is loaded — LSP
// positions refer to the in-memory (didChange-synced) document, which can
// differ from disk — falling back to files, a per-palette cache, so each
// file is read at most once per batch instead of once per item. Non-file
// URIs (jdt://, deno:/) get a bare label instead of a crash.
func (e *Editor) locationLabel(files map[string][]string, loc lsp.Location) string {
	lineNum := int(loc.Range.Start.Line)
	path, ok := lspFilename(loc.URI)
	if !ok {
		return fmt.Sprintf("%s:%d:", string(loc.URI), lineNum+1)
	}
	display := path
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(rel, "..") {
			display = rel
		}
	}
	text := ""
	if b := e.findBuffer(path); b != nil {
		if lineNum >= 0 && lineNum <= b.NumLines() {
			text = strings.TrimSpace(string(b.GetLine(lineNum)))
		}
	} else {
		lines, cached := files[path]
		if !cached {
			if data, err := os.ReadFile(path); err == nil {
				lines = strings.Split(string(data), "\n")
			}
			files[path] = lines // nil on read failure: don't retry per item
		}
		if lineNum >= 0 && lineNum < len(lines) {
			text = strings.TrimSpace(lines[lineNum])
		}
	}
	return fmt.Sprintf("%s:%d: %s", display, lineNum+1, text)
}

// lspDocumentSymbols requests the symbol outline of the current buffer and
// opens a searchable palette of results (flattened, indented by nesting
// depth) once the asynchronous response arrives.
func (e *Editor) lspDocumentSymbols() {
	v := e.ActiveView()
	if v == nil || v.buf.lspServer == nil {
		e.infobar.Error("No LSP server")
		return
	}
	b := v.buf
	s := b.lspServer
	absPath, _ := filepath.Abs(b.Path)
	e.infobar.Message("Loading symbols…")

	lspAsync(e, func() ([]lsp.DocumentSymbol, error) {
		return s.DocumentSymbols(absPath)
	}, func(symbols []lsp.DocumentSymbol, err error) {
		if av := e.ActiveView(); av == nil || av.buf != b || e.infobar.IsActive() {
			return
		}
		if err != nil {
			if err == ErrLspNotSupported {
				e.infobar.Error("Document symbols not supported")
			} else {
				e.infobar.Error(fmt.Sprintf("symbols: %v", err))
			}
			return
		}
		var items []paletteItem
		flattenSymbols(symbols, 0, func(sym lsp.DocumentSymbol, depth int) {
			pos := sym.SelectionRange.Start
			label := fmt.Sprintf("%s%s  %s", strings.Repeat("  ", depth), sym.Name, sym.Kind.String())
			items = append(items, paletteItem{label: label, action: func() {
				// The action can fire after the user switched buffers;
				// bring b back rather than moving a hidden cursor.
				if !e.hasBuffer(b) {
					return
				}
				e.pushJump()
				e.showBuffer(b)
				target := b.FromLspPosition(pos)
				*b.Cursor() = b.Cursor().MoveTo(target)
			}})
		})
		if len(items) == 0 {
			e.infobar.Message("No symbols found")
			return
		}
		e.startPaletteItems("Symbols> ", items)
	})
}

// flattenSymbols walks a DocumentSymbol tree depth-first, calling visit for
// every node in document order with its nesting depth.
func flattenSymbols(symbols []lsp.DocumentSymbol, depth int, visit func(lsp.DocumentSymbol, int)) {
	for _, sym := range symbols {
		visit(sym, depth)
		flattenSymbols(sym.Children, depth+1, visit)
	}
}

func init() {
	editorCommands = append(editorCommands,
		CommandDef{"lsp-references", cmdLspReferences, "lsp-references: find all references to symbol under cursor"},
		CommandDef{"lsp-symbols", cmdLspSymbols, "lsp-symbols: list symbols in the current document"},
	)
}

func cmdLspReferences(e *Editor, args []string) error {
	e.lspFindReferences()
	return nil
}

func cmdLspSymbols(e *Editor, args []string) error {
	e.lspDocumentSymbols()
	return nil
}
