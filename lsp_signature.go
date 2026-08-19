package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf16"

	lsp "go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// The vendored protocol package types ParameterInformation.Label as a plain
// string, but the spec also allows a [start, end] UTF-16 offset pair into
// the signature label; decoding straight into lsp.SignatureHelp would fail
// for servers that use the offset form. These wire-only types capture the
// label as raw JSON so both forms decode without error.
type lspParameterInformation struct {
	Label json.RawMessage `json:"label"`
}

type lspSignatureInformation struct {
	Label      string                    `json:"label"`
	Parameters []lspParameterInformation `json:"parameters,omitempty"`
}

type lspSignatureHelp struct {
	Signatures      []lspSignatureInformation `json:"signatures"`
	ActiveParameter *uint32                   `json:"activeParameter,omitempty"`
	ActiveSignature uint32                    `json:"activeSignature,omitempty"`
}

// SignatureHelp requests parameter info for the call at pos.
func (s *LspServer) SignatureHelp(filename string, pos lsp.Position) (*lspSignatureHelp, error) {
	if s == nil || s.caps().SignatureHelpProvider == nil {
		return nil, ErrLspNotSupported
	}
	params := lsp.SignatureHelpParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
			Position:     pos,
		},
	}
	resp, err := s.request(lsp.MethodTextDocumentSignatureHelp, params, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	var help struct {
		Result *lspSignatureHelp `json:"result"`
	}
	if err := json.Unmarshal(resp, &help); err != nil {
		return nil, err
	}
	return help.Result, nil
}

// signatureHelpText formats a SignatureHelp response as a single line for
// the infobar, bracketing the active parameter's label so it stands out in
// plain text.
func signatureHelpText(help *lspSignatureHelp) string {
	if help == nil || len(help.Signatures) == 0 {
		return ""
	}
	sigIdx := int(help.ActiveSignature)
	if sigIdx < 0 || sigIdx >= len(help.Signatures) {
		sigIdx = 0
	}
	sig := help.Signatures[sigIdx]

	// A parameter offset pair (rather than a label substring) needs the
	// label sliced out in UTF-16 units, matching the rest of the LSP
	// position model.
	if help.ActiveParameter == nil {
		return sig.Label
	}
	paramIdx := int(*help.ActiveParameter)
	if paramIdx < 0 || paramIdx >= len(sig.Parameters) {
		return sig.Label
	}
	label := sig.Parameters[paramIdx].Label

	var name string
	if json.Unmarshal(label, &name) == nil {
		if name == "" || !strings.Contains(sig.Label, name) {
			return sig.Label
		}
		return strings.Replace(sig.Label, name, "["+name+"]", 1)
	}
	var offsets [2]int
	if json.Unmarshal(label, &offsets) == nil {
		u16 := utf16.Encode([]rune(sig.Label))
		start, end := offsets[0], offsets[1]
		if start < 0 || end > len(u16) || start >= end {
			return sig.Label
		}
		before := string(utf16.Decode(u16[:start]))
		mid := string(utf16.Decode(u16[start:end]))
		after := string(utf16.Decode(u16[end:]))
		return before + "[" + mid + "]" + after
	}
	return sig.Label
}

// lspSignatureHelpAt requests signature help at cursor and shows it in the
// infobar when the (asynchronous) answer arrives.
func (e *Editor) lspSignatureHelpAt() {
	v := e.ActiveView()
	if v == nil || v.buf.lspServer == nil {
		return
	}
	b := v.buf
	s := b.lspServer
	line, col := b.LineColAt(b.Cursor().Pos)
	pos := b.LspPosition(line, col)
	absPath, _ := filepath.Abs(b.Path)

	lspAsync(e, func() (*lspSignatureHelp, error) {
		return s.SignatureHelp(absPath, pos)
	}, func(help *lspSignatureHelp, err error) {
		if av := e.ActiveView(); av == nil || av.buf != b || e.infobar.IsActive() {
			return
		}
		if err != nil {
			if err == ErrLspNotSupported {
				e.infobar.Error("Signature help not supported")
			} else {
				e.infobar.Error(fmt.Sprintf("signature help: %v", err))
			}
			return
		}
		if text := signatureHelpText(help); text != "" {
			e.infobar.Message(text)
		} else {
			e.infobar.Message("No signature help")
		}
	})
}

func init() {
	editorCommands = append(editorCommands,
		CommandDef{"lsp-signature", cmdLspSignature, "lsp-signature: show signature help"},
	)
}

func cmdLspSignature(e *Editor, args []string) error {
	e.lspSignatureHelpAt()
	return nil
}
