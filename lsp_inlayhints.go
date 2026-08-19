package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	lsp "go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// The vendored protocol package predates inlay hints (LSP 3.17), so these
// wire-only types cover just the fields mu needs. Label may be a plain
// string or an array of label parts; both decode from raw JSON.
type lspInlayHintParams struct {
	TextDocument lsp.TextDocumentIdentifier `json:"textDocument"`
	Range        lsp.Range                  `json:"range"`
}

type lspInlayHintLabelPart struct {
	Value string `json:"value"`
}

type lspInlayHint struct {
	Position lsp.Position    `json:"position"`
	Label    json.RawMessage `json:"label"`
}

// inlayHintLabelText normalizes an InlayHint.label into plain text.
func inlayHintLabelText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []lspInlayHintLabelPart
	if json.Unmarshal(raw, &parts) == nil {
		var sb strings.Builder
		for _, p := range parts {
			sb.WriteString(p.Value)
		}
		return sb.String()
	}
	return ""
}

// InlayHints requests inlay hints for the whole document (start of file to
// end). See the LspServer.inlayHintProvider field comment for why support is
// tracked outside caps().
func (s *LspServer) InlayHints(filename string, end lsp.Position) ([]lspInlayHint, error) {
	if s == nil || !s.hasInlayHintProvider() {
		return nil, ErrLspNotSupported
	}
	params := lspInlayHintParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
		Range:        lsp.Range{End: end},
	}
	resp, err := s.request("textDocument/inlayHint", params, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	var hints struct {
		Result []lspInlayHint `json:"result"`
	}
	if err := json.Unmarshal(resp, &hints); err != nil {
		return nil, err
	}
	return hints.Result, nil
}

// InlayHintMark is a display-ready projection of an LSP inlay hint: the
// buffer line to mark in the gutter, and the hint text to show in the
// infobar when the cursor is on that line. Hints are not rendered inline in
// the buffer text.
type InlayHintMark struct {
	Line int
	Text string
}

// SetInlayHints replaces the buffer's inlay hints (see lspRefreshInlayHints).
func (b *Buffer) SetInlayHints(hints []InlayHintMark) { b.inlayHints = hints }

// ClearInlayHints removes all inlay hints from the buffer.
func (b *Buffer) ClearInlayHints() { b.inlayHints = nil }

// GetInlayHintAt returns the first inlay hint on the given line.
func (b *Buffer) GetInlayHintAt(line int) (InlayHintMark, bool) {
	for _, h := range b.inlayHints {
		if h.Line == line {
			return h, true
		}
	}
	return InlayHintMark{}, false
}

// lspRefreshInlayHints requests inlay hints for the whole buffer and stores
// them for gutter display (an 'i' marker) and infobar lookup when the
// cursor is on the hinted line.
func (e *Editor) lspRefreshInlayHints() {
	v := e.ActiveView()
	if v == nil || v.buf.lspServer == nil {
		e.infobar.Error("No LSP server")
		return
	}
	b := v.buf
	s := b.lspServer
	absPath, _ := filepath.Abs(b.Path)
	lastLine, lastCol := b.LineColAt(b.Len())
	end := b.LspPosition(lastLine, lastCol)

	lspAsync(e, func() ([]lspInlayHint, error) {
		return s.InlayHints(absPath, end)
	}, func(hints []lspInlayHint, err error) {
		if !e.hasBuffer(b) {
			return
		}
		if err != nil {
			if err == ErrLspNotSupported {
				e.infobar.Error("Inlay hints not supported")
			} else {
				e.infobar.Error(fmt.Sprintf("inlay hints: %v", err))
			}
			return
		}
		marks := make([]InlayHintMark, 0, len(hints))
		for _, h := range hints {
			text := inlayHintLabelText(h.Label)
			if text == "" {
				continue
			}
			line, _ := b.LineColAt(b.FromLspPosition(h.Position))
			marks = append(marks, InlayHintMark{Line: line, Text: text})
		}
		b.SetInlayHints(marks)
		if len(marks) == 0 {
			e.infobar.Message("No inlay hints")
		} else {
			e.infobar.Message(fmt.Sprintf("%d inlay hints", len(marks)))
		}
	})
}

func init() {
	editorCommands = append(editorCommands,
		CommandDef{"lsp-inlay-hints", cmdLspInlayHints, "lsp-inlay-hints: show inlay hints in the gutter"},
	)
}

func cmdLspInlayHints(e *Editor, args []string) error {
	e.lspRefreshInlayHints()
	return nil
}
