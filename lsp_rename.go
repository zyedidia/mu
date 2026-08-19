package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	lsp "go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// Rename requests a workspace-wide rename of the symbol at pos to newName.
func (s *LspServer) Rename(filename string, pos lsp.Position, newName string) (lspWorkspaceEdit, error) {
	if s == nil || s.caps().RenameProvider == nil {
		return lspWorkspaceEdit{}, ErrLspNotSupported
	}
	params := lsp.RenameParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
			Position:     pos,
		},
		NewName: newName,
	}
	resp, err := s.request(lsp.MethodTextDocumentRename, params, lspRequestTimeout)
	if err != nil {
		return lspWorkspaceEdit{}, err
	}
	var edit struct {
		Result lspWorkspaceEdit `json:"result"`
	}
	if err := json.Unmarshal(resp, &edit); err != nil {
		return lspWorkspaceEdit{}, err
	}
	return edit.Result, nil
}

// lspRename prompts for a new name (prefilled with the word under the
// cursor) and applies the resulting workspace edit.
func (e *Editor) lspRename() {
	v := e.ActiveView()
	if v == nil || v.buf.lspServer == nil {
		e.infobar.Error("No LSP server")
		return
	}
	b := v.buf
	s := b.lspServer
	version := b.lspVersion
	line, col := b.LineColAt(b.Cursor().Pos)
	pos := b.LspPosition(line, col)
	absPath, _ := filepath.Abs(b.Path)

	e.infobar.StartPrompt("New name> ", func(newName string) {
		if newName == "" {
			return
		}
		lspAsync(e, func() (lspWorkspaceEdit, error) {
			return s.Rename(absPath, pos, newName)
		}, func(edit lspWorkspaceEdit, err error) {
			if !e.hasBuffer(b) || b.lspVersion != version {
				e.infobar.Error("rename: buffer changed, not applied")
				return
			}
			if err != nil {
				if err == ErrLspNotSupported {
					e.infobar.Error("Rename not supported")
				} else {
					e.infobar.Error(fmt.Sprintf("rename: %v", err))
				}
				return
			}
			if err := e.applyWorkspaceEdit(edit); err != nil {
				e.infobar.Error(fmt.Sprintf("rename: %v", err))
				return
			}
			e.infobar.Message(fmt.Sprintf("Renamed to %s", newName))
		})
	})
	e.infobar.input = []rune(e.wordUnderCursor())
	e.infobar.cursorPos = len(e.infobar.input)
}

func init() {
	editorCommands = append(editorCommands,
		CommandDef{"lsp-rename", cmdLspRename, "lsp-rename: rename symbol under cursor"},
	)
}

func cmdLspRename(e *Editor, args []string) error {
	e.lspRename()
	return nil
}
