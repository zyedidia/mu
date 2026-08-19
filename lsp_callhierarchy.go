package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	lsp "go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// PrepareCallHierarchy resolves the call-hierarchy item(s) at pos, the
// required first step before requesting incoming or outgoing calls.
func (s *LspServer) PrepareCallHierarchy(filename string, pos lsp.Position) ([]lsp.CallHierarchyItem, error) {
	if s == nil || s.caps().CallHierarchyProvider == nil {
		return nil, ErrLspNotSupported
	}
	params := lsp.CallHierarchyPrepareParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
			Position:     pos,
		},
	}
	resp, err := s.request(lsp.MethodTextDocumentPrepareCallHierarchy, params, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	var items struct {
		Result []lsp.CallHierarchyItem `json:"result"`
	}
	if err := json.Unmarshal(resp, &items); err != nil {
		return nil, err
	}
	return items.Result, nil
}

// IncomingCalls requests the functions that call item.
func (s *LspServer) IncomingCalls(item lsp.CallHierarchyItem) ([]lsp.CallHierarchyIncomingCall, error) {
	if s == nil {
		return nil, ErrLspNotSupported
	}
	resp, err := s.request(lsp.MethodCallHierarchyIncomingCalls, lsp.CallHierarchyIncomingCallsParams{Item: item}, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	var calls struct {
		Result []lsp.CallHierarchyIncomingCall `json:"result"`
	}
	if err := json.Unmarshal(resp, &calls); err != nil {
		return nil, err
	}
	return calls.Result, nil
}

// OutgoingCalls requests the functions that item calls.
func (s *LspServer) OutgoingCalls(item lsp.CallHierarchyItem) ([]lsp.CallHierarchyOutgoingCall, error) {
	if s == nil {
		return nil, ErrLspNotSupported
	}
	resp, err := s.request(lsp.MethodCallHierarchyOutgoingCalls, lsp.CallHierarchyOutgoingCallsParams{Item: item}, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	var calls struct {
		Result []lsp.CallHierarchyOutgoingCall `json:"result"`
	}
	if err := json.Unmarshal(resp, &calls); err != nil {
		return nil, err
	}
	return calls.Result, nil
}

// lspCallHierarchy resolves the call-hierarchy item under the cursor (the
// first, if a server returns several candidates), then requests either its
// incoming or outgoing calls and opens a searchable palette of the results;
// selecting one jumps to that function's definition.
func (e *Editor) lspCallHierarchy(incoming bool) {
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

	prompt, notSupported := "Incoming Calls> ", "Incoming calls not supported"
	if !incoming {
		prompt, notSupported = "Outgoing Calls> ", "Outgoing calls not supported"
	}
	e.infobar.Message("Loading call hierarchy…")

	lspAsync(e, func() ([]lsp.CallHierarchyItem, error) {
		return s.PrepareCallHierarchy(absPath, pos)
	}, func(items []lsp.CallHierarchyItem, err error) {
		if av := e.ActiveView(); av == nil || av.buf != b {
			return
		}
		if err != nil {
			if err == ErrLspNotSupported {
				e.infobar.Error(notSupported)
			} else {
				e.infobar.Error(fmt.Sprintf("call hierarchy: %v", err))
			}
			return
		}
		if len(items) == 0 {
			e.infobar.Message("No call hierarchy item here")
			return
		}
		item := items[0]

		if incoming {
			lspAsync(e, func() ([]lsp.CallHierarchyIncomingCall, error) {
				return s.IncomingCalls(item)
			}, func(calls []lsp.CallHierarchyIncomingCall, err error) {
				if av := e.ActiveView(); av == nil || av.buf != b {
					return
				}
				if err != nil {
					e.infobar.Error(fmt.Sprintf("incoming calls: %v", err))
					return
				}
				items := make([]paletteItem, len(calls))
				for i, c := range calls {
					c := c
					items[i] = paletteItem{
						label: fmt.Sprintf("%s  %s", c.From.Name, locationLabel(lsp.Location{URI: c.From.URI, Range: c.From.SelectionRange})),
						action: func() {
							e.pushJump()
							e.jumpToLspLocation(b, lsp.Location{URI: c.From.URI, Range: c.From.SelectionRange})
						},
					}
				}
				showCallHierarchyResults(e, prompt, items)
			})
			return
		}
		lspAsync(e, func() ([]lsp.CallHierarchyOutgoingCall, error) {
			return s.OutgoingCalls(item)
		}, func(calls []lsp.CallHierarchyOutgoingCall, err error) {
			if av := e.ActiveView(); av == nil || av.buf != b {
				return
			}
			if err != nil {
				e.infobar.Error(fmt.Sprintf("outgoing calls: %v", err))
				return
			}
			items := make([]paletteItem, len(calls))
			for i, c := range calls {
				c := c
				items[i] = paletteItem{
					label: fmt.Sprintf("%s  %s", c.To.Name, locationLabel(lsp.Location{URI: c.To.URI, Range: c.To.SelectionRange})),
					action: func() {
						e.pushJump()
						e.jumpToLspLocation(b, lsp.Location{URI: c.To.URI, Range: c.To.SelectionRange})
					},
				}
			}
			showCallHierarchyResults(e, prompt, items)
		})
	})
}

func showCallHierarchyResults(e *Editor, prompt string, items []paletteItem) {
	if len(items) == 0 {
		e.infobar.Message("No calls found")
		return
	}
	e.startPaletteItems(prompt, items)
}

func init() {
	editorCommands = append(editorCommands,
		CommandDef{"lsp-incoming-calls", cmdLspIncomingCalls, "lsp-incoming-calls: list functions that call the symbol under cursor"},
		CommandDef{"lsp-outgoing-calls", cmdLspOutgoingCalls, "lsp-outgoing-calls: list functions called by the symbol under cursor"},
	)
}

func cmdLspIncomingCalls(e *Editor, args []string) error {
	e.lspCallHierarchy(true)
	return nil
}

func cmdLspOutgoingCalls(e *Editor, args []string) error {
	e.lspCallHierarchy(false)
	return nil
}
