package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	lsp "go.lsp.dev/protocol"
)

// initLsp sets up the LSP manager with diagnostic and message callbacks.
func (e *Editor) initLsp() {
	langs, err := e.config.LoadLspLanguages()
	if err != nil {
		log.Printf("[lsp] load languages: %v", err)
		langs = make(map[string]LspLanguage)
	}

	e.lspManager = NewLspManager(langs,
		func(msg lsp.ShowMessageParams) {
			e.infobar.Message(msg.Message)
			if e.screen != nil {
				e.screen.PostEvent(tcell.NewEventInterrupt(nil))
			}
		},
		func(diag lsp.PublishDiagnosticsParams) {
			e.handleDiagnostics(diag)
			if e.screen != nil {
				e.screen.PostEvent(tcell.NewEventInterrupt(nil))
			}
		},
	)
}

// handleDiagnostics receives pushed diagnostics and applies them to the
// matching buffer.
func (e *Editor) handleDiagnostics(params lsp.PublishDiagnosticsParams) {
	path := params.URI.Filename()
	for _, v := range e.views {
		absPath, _ := filepath.Abs(v.buf.Path)
		if absPath == path {
			v.buf.ClearDiagnostics()
			for _, d := range params.Diagnostics {
				_, col8 := v.buf.Utf8Loc(int(d.Range.Start.Line), int(d.Range.Start.Character))
				dtype := DiagWarning
				if d.Severity == lsp.DiagnosticSeverityError {
					dtype = DiagError
				}
				v.buf.AddDiagnostic(int(d.Range.Start.Line), col8, d.Message, dtype)
			}
			break
		}
	}
}

// initBufferLsp starts LSP for a buffer if configured for its filetype.
func (e *Editor) initBufferLsp(buf *Buffer, ft string) {
	if ft == "" || e.lspManager == nil {
		return
	}
	absPath, _ := filepath.Abs(buf.Path)
	contents := string(buf.Slice(0, buf.Len()))
	s, err := e.lspManager.Open(ft, absPath, contents, 0)
	if err != nil {
		log.Printf("[lsp] open %s: %v", ft, err)
		return
	}
	buf.lspServer = s
	buf.lspFt = ft
}

// registerLspBindings adds gd, K, ]d, [d keybindings and LSP commands.
func (e *Editor) registerLspBindings() {
	// gd: go to definition
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.lspGotoDefinition()
	}, "g", "d")

	// K: hover
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.lspHover()
	}, "K")

	// ]d: next diagnostic
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.lspNextDiagnostic(1)
		ks.ResetAction()
	}, "]", "d")

	// [d: previous diagnostic
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.lspNextDiagnostic(-1)
		ks.ResetAction()
	}, "[", "d")
}

// lspGotoDefinition requests definition at cursor and jumps there.
func (e *Editor) lspGotoDefinition() {
	v := e.ActiveView()
	if v == nil || v.buf.lspServer == nil {
		e.infobar.Error("No LSP server")
		return
	}
	b := v.buf
	line, col := b.LineColAt(b.Cursor().Pos)
	pos := b.LspPosition(line, col)

	absPath, _ := filepath.Abs(b.Path)
	locs, err := b.lspServer.Definition(absPath, pos)
	if err != nil {
		e.infobar.Error(fmt.Sprintf("definition: %v", err))
		return
	}
	if len(locs) == 0 {
		e.infobar.Message("No definition found")
		return
	}

	loc := locs[0]
	targetPath := loc.URI.Filename()
	targetAbsPath, _ := filepath.Abs(b.Path)

	if targetPath == targetAbsPath {
		// Same file: jump to position.
		target := b.FromLspPosition(loc.Range.Start)
		*b.Cursor() = b.Cursor().MoveTo(target)
	} else {
		// Different file: open it and jump.
		if err := e.OpenFile(targetPath); err != nil {
			e.infobar.Error(fmt.Sprintf("open: %v", err))
			return
		}
		nb := e.ActiveView().buf
		target := nb.FromLspPosition(loc.Range.Start)
		*nb.Cursor() = nb.Cursor().MoveTo(target)
	}
}

// lspHover requests hover info at cursor and displays in infobar.
func (e *Editor) lspHover() {
	v := e.ActiveView()
	if v == nil || v.buf.lspServer == nil {
		e.infobar.Error("No LSP server")
		return
	}
	b := v.buf
	line, col := b.LineColAt(b.Cursor().Pos)
	pos := b.LspPosition(line, col)

	absPath, _ := filepath.Abs(b.Path)
	info, err := b.lspServer.Hover(absPath, pos)
	if err != nil {
		if err == ErrLspNotSupported {
			e.infobar.Error("Hover not supported")
		} else {
			e.infobar.Error(fmt.Sprintf("hover: %v", err))
		}
		return
	}
	if info == "" {
		e.infobar.Message("No hover info")
		return
	}
	// Collapse to single line for infobar display.
	info = strings.ReplaceAll(info, "\n", " ")
	info = strings.Join(strings.Fields(info), " ")
	e.infobar.Message(info)
}

// lspNextDiagnostic jumps to the next (dir=1) or previous (dir=-1) diagnostic.
func (e *Editor) lspNextDiagnostic(dir int) {
	v := e.ActiveView()
	if v == nil {
		return
	}
	b := v.buf
	diags := b.GetDiagnostics()
	if len(diags) == 0 {
		e.infobar.Message("No diagnostics")
		return
	}
	curLine, _ := b.LineColAt(b.Cursor().Pos)

	var best *Diagnostic
	for i := range diags {
		d := &diags[i]
		if dir > 0 && d.Line > curLine {
			if best == nil || d.Line < best.Line {
				best = d
			}
		} else if dir < 0 && d.Line < curLine {
			if best == nil || d.Line > best.Line {
				best = d
			}
		}
	}
	// Wrap around.
	if best == nil {
		if dir > 0 {
			best = &diags[0]
		} else {
			best = &diags[len(diags)-1]
		}
	}

	pos := b.OffsetAt(best.Line, best.Col)
	*b.Cursor() = b.Cursor().MoveTo(pos)
	e.infobar.Message(fmt.Sprintf("[%s] %s", best.Type.String(), best.Text))
}

// --- LSP TCL commands ---

func init() {
	editorCommands = append(editorCommands,
		CommandDef{"lsp-hover", cmdLspHover, "lsp-hover: show hover info"},
		CommandDef{"lsp-def", cmdLspDef, "lsp-def: go to definition"},
		CommandDef{"lsp-format", cmdLspFormat, "lsp-format: format document"},
	)
}

func cmdLspHover(e *Editor, args []string) error {
	e.lspHover()
	return nil
}

func cmdLspDef(e *Editor, args []string) error {
	e.lspGotoDefinition()
	return nil
}

func cmdLspFormat(e *Editor, args []string) error {
	v := e.ActiveView()
	if v == nil || v.buf.lspServer == nil {
		return fmt.Errorf("no LSP server")
	}
	b := v.buf
	absPath, _ := filepath.Abs(b.Path)
	edits, err := b.lspServer.Format(absPath)
	if err != nil {
		return err
	}
	b.UndoBarrier()
	// Apply edits in reverse order so positions stay valid.
	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		start := b.FromLspPosition(edit.Range.Start)
		end := b.FromLspPosition(edit.Range.End)
		b.Remove(start, end)
		if len(edit.NewText) > 0 {
			b.Insert(start, []byte(edit.NewText))
		}
	}
	e.infobar.Message("Formatted")
	return nil
}
