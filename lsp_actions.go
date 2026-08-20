package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	lsp "go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// These small wire types cover both forms allowed by the LSP code-action
// response: a legacy Command and a CodeAction carrying an edit and/or command.
type lspCommand struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

type lspCodeAction struct {
	Title     string            `json:"title"`
	Kind      string            `json:"kind,omitempty"`
	Disabled  json.RawMessage   `json:"disabled,omitempty"`
	Edit      *lspWorkspaceEdit `json:"edit,omitempty"`
	Command   json.RawMessage   `json:"command,omitempty"`
	Arguments []any             `json:"arguments,omitempty"` // legacy Command
}

type lspWorkspaceEdit struct {
	Changes         map[string][]lsp.TextEdit `json:"changes,omitempty"`
	DocumentChanges []json.RawMessage         `json:"documentChanges,omitempty"`
}

type lspDocumentEdit struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int32 `json:"version,omitempty"`
	} `json:"textDocument"`
	Edits []lsp.TextEdit `json:"edits"`
}

func (s *LspServer) CodeActions(filename string, r lsp.Range, diagnostics []lsp.Diagnostic) ([]lspCodeAction, error) {
	if s == nil || !capEnabled(s.caps().CodeActionProvider) {
		return nil, ErrLspNotSupported
	}
	params := struct {
		TextDocument lsp.TextDocumentIdentifier `json:"textDocument"`
		Range        lsp.Range                  `json:"range"`
		Context      struct {
			Diagnostics []lsp.Diagnostic `json:"diagnostics"`
		} `json:"context"`
	}{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
		Range:        r,
	}
	params.Context.Diagnostics = diagnostics

	resp, err := s.request("textDocument/codeAction", params, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	var result struct {
		Result []lspCodeAction `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return result.Result, nil
}

func (s *LspServer) ExecuteCommand(command lspCommand) error {
	if s == nil || command.Command == "" {
		return nil
	}
	_, err := s.request("workspace/executeCommand", struct {
		Command   string `json:"command"`
		Arguments []any  `json:"arguments,omitempty"`
	}{command.Command, command.Arguments}, lspRequestTimeout)
	return err
}

func actionCommand(action lspCodeAction) (lspCommand, error) {
	if len(action.Command) == 0 || string(action.Command) == "null" {
		return lspCommand{}, nil
	}
	// A legacy Command has its command identifier directly in the field;
	// CodeAction.Command is an embedded object.
	var name string
	if json.Unmarshal(action.Command, &name) == nil {
		return lspCommand{Title: action.Title, Command: name, Arguments: action.Arguments}, nil
	}
	var command lspCommand
	if err := json.Unmarshal(action.Command, &command); err != nil {
		return lspCommand{}, err
	}
	return command, nil
}

// lspCodeActions requests actions at the cursor and opens a searchable
// palette once the asynchronous response arrives.
func (e *Editor) lspCodeActions() {
	v := e.ActiveView()
	if v == nil || v.buf.lspServer == nil {
		e.infobar.Error("No LSP server")
		return
	}
	b := v.buf
	s := b.lspServer
	version := b.lspVersion
	requestPos := b.Cursor().Pos
	line, col := b.LineColAt(b.Cursor().Pos)
	pos := b.LspPosition(line, col)
	r := lsp.Range{Start: pos, End: pos}
	diagnostics := diagnosticsAt(b.lspDiagnostics, pos)
	absPath, _ := filepath.Abs(b.Path)
	e.infobar.Message("Loading code actions…")

	lspAsync(e, func() ([]lspCodeAction, error) {
		return s.CodeActions(absPath, r, diagnostics)
	}, func(actions []lspCodeAction, err error) {
		// The last clause keeps a late answer from replacing an active
		// prompt (or another palette) with the code-action palette.
		if !e.hasBuffer(b) || b.lspVersion != version || b.Cursor().Pos != requestPos || e.ActiveView() == nil || e.ActiveView().buf != b || e.infobar.IsActive() {
			return
		}
		if err != nil {
			if err == ErrLspNotSupported {
				e.infobar.Error("Code actions not supported")
			} else {
				e.infobar.Error(fmt.Sprintf("code actions: %v", err))
			}
			return
		}
		items := make([]paletteItem, 0, len(actions))
		for _, candidate := range actions {
			if candidate.Title == "" || (len(candidate.Disabled) != 0 && string(candidate.Disabled) != "null") {
				continue
			}
			action := candidate
			items = append(items, paletteItem{label: action.Title, action: func() {
				e.applyCodeAction(s, b, version, action)
			}})
		}
		if len(items) == 0 {
			e.infobar.Message("No code actions")
			return
		}
		e.startPaletteItems("Code Actions> ", items)
	})
}

func diagnosticsAt(diagnostics []lsp.Diagnostic, pos lsp.Position) []lsp.Diagnostic {
	var out []lsp.Diagnostic
	for _, diagnostic := range diagnostics {
		start, end := diagnostic.Range.Start, diagnostic.Range.End
		afterStart := pos.Line > start.Line || (pos.Line == start.Line && pos.Character >= start.Character)
		beforeEnd := pos.Line < end.Line || (pos.Line == end.Line && pos.Character <= end.Character)
		if afterStart && beforeEnd {
			out = append(out, diagnostic)
		}
	}
	return out
}

func (e *Editor) applyCodeAction(s *LspServer, origin *Buffer, version int32, action lspCodeAction) {
	if !e.hasBuffer(origin) || origin.lspVersion != version {
		e.infobar.Error("code action: buffer changed, not applied")
		return
	}
	if action.Edit != nil {
		if err := e.applyWorkspaceEdit(*action.Edit); err != nil {
			e.infobar.Error(fmt.Sprintf("code action: %v", err))
			return
		}
	}
	command, err := actionCommand(action)
	if err != nil {
		e.infobar.Error(fmt.Sprintf("code action: %v", err))
		return
	}
	if command.Command == "" {
		e.infobar.Message(action.Title)
		return
	}
	lspAsync(e, func() (struct{}, error) {
		return struct{}{}, s.ExecuteCommand(command)
	}, func(_ struct{}, err error) {
		if err != nil {
			e.infobar.Error(fmt.Sprintf("code action command: %v", err))
		} else {
			e.infobar.Message(action.Title)
		}
	})
}

func (e *Editor) applyWorkspaceEdit(edit lspWorkspaceEdit) error {
	type target struct {
		path    string
		version *int32
		edits   []lsp.TextEdit
	}
	var targets []target
	for rawURI, edits := range edit.Changes {
		path, ok := lspFilename(uri.URI(rawURI))
		if !ok {
			return fmt.Errorf("unsupported workspace edit URI %s", rawURI)
		}
		targets = append(targets, target{path: path, edits: edits})
	}
	for _, raw := range edit.DocumentChanges {
		var doc lspDocumentEdit
		if err := json.Unmarshal(raw, &doc); err != nil || doc.TextDocument.URI == "" {
			return fmt.Errorf("unsupported workspace resource operation")
		}
		path, ok := lspFilename(uri.URI(doc.TextDocument.URI))
		if !ok {
			return fmt.Errorf("unsupported workspace edit URI %s", doc.TextDocument.URI)
		}
		targets = append(targets, target{path: path, version: doc.TextDocument.Version, edits: doc.Edits})
	}

	// Resolve and validate every target before changing any text.
	buffers := make([]*Buffer, len(targets))
	original := e.ActiveView()
	defer func() {
		if original != nil && e.hasBuffer(original.buf) {
			e.showBuffer(original.buf)
		}
	}()
	for i, target := range targets {
		if target.path == "" || strings.Contains(target.path, "://") {
			return fmt.Errorf("unsupported workspace edit URI")
		}
		b := e.findBuffer(target.path)
		if b == nil {
			if err := e.OpenFile(target.path); err != nil {
				return err
			}
			b = e.ActiveView().buf
		}
		if target.version != nil && *target.version != b.lspVersion {
			return fmt.Errorf("buffer changed, not applied")
		}
		buffers[i] = b
	}
	for i, target := range targets {
		buffers[i].UndoBarrier()
		applyTextEdits(buffers[i], target.edits)
		buffers[i].UndoBarrier()
	}
	return nil
}
