package main

import (
	"fmt"

	lsp "go.lsp.dev/protocol"
)

// lspWorkspaceSymbols prompts for a query, requests matching symbols across
// every file in the workspace, and opens a searchable palette of results
// once the asynchronous response arrives; selecting one jumps to it
// (opening the file first if needed).
func (e *Editor) lspWorkspaceSymbols() {
	v := e.ActiveView()
	if v == nil || v.buf.lspServer == nil {
		e.infobar.Error("No LSP server")
		return
	}
	b := v.buf
	s := b.lspServer

	e.infobar.StartPrompt("Workspace Symbol> ", func(query string) {
		e.infobar.Message("Loading symbols…")
		lspAsync(e, func() ([]lsp.SymbolInformation, error) {
			return s.WorkspaceSymbols(query)
		}, func(syms []lsp.SymbolInformation, err error) {
			if err != nil {
				if err == ErrLspNotSupported {
					e.infobar.Error("Workspace symbols not supported")
				} else {
					e.infobar.Error(fmt.Sprintf("workspace symbols: %v", err))
				}
				return
			}
			if len(syms) == 0 {
				e.infobar.Message("No symbols found")
				return
			}
			items := make([]paletteItem, len(syms))
			for i, sym := range syms {
				sym := sym
				items[i] = paletteItem{
					label: fmt.Sprintf("%s  %s  %s", sym.Name, sym.Kind.String(), locationLabel(sym.Location)),
					action: func() {
						e.pushJump()
						e.jumpToLspLocation(b, sym.Location)
					},
				}
			}
			e.startPaletteItems("Workspace Symbols> ", items)
		})
	})
}

func init() {
	editorCommands = append(editorCommands,
		CommandDef{"lsp-workspace-symbols", cmdLspWorkspaceSymbols, "lsp-workspace-symbols: fuzzy search symbols across the workspace"},
	)
}

func cmdLspWorkspaceSymbols(e *Editor, args []string) error {
	e.lspWorkspaceSymbols()
	return nil
}
