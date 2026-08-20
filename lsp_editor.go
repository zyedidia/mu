package main

import (
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	lsp "go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// formatOnSaveTimeout bounds the synchronous format request a save makes:
// long enough for a real formatter, short enough that a hung server delays
// a save noticeably less than the general request timeout would.
const formatOnSaveTimeout = 2 * time.Second

// applyEditTimeout bounds how long a server's workspace/applyEdit waits for
// the editor goroutine to run it. A variable so tests can shorten it.
var applyEditTimeout = 5 * time.Second

// initLsp sets up the LSP manager with diagnostic and message callbacks.
func (e *Editor) initLsp() {
	langs, err := e.config.LoadLspLanguages()
	if err != nil {
		log.Printf("[lsp] load languages: %v", err)
		langs = make(map[string]LspLanguage)
	}
	e.lspManager = NewLspManager(langs, e.lspCallbacks())
}

// lspCallbacks builds the server → editor callbacks. They run on the LSP
// receive goroutine, so each one marshals its state changes onto the main
// event loop.
func (e *Editor) lspCallbacks() lspCallbacks {
	return lspCallbacks{
		onShow: func(msg lsp.ShowMessageParams) {
			e.postToMain(func() {
				e.infobar.Message(msg.Message)
			})
		},
		onDiag: func(diag lsp.PublishDiagnosticsParams) {
			e.postToMain(func() {
				e.handleDiagnostics(diag)
			})
		},
		onProgress: func(status string) {
			e.postToMain(func() {
				// Server progress ("Indexing: 3/5 (60%)") is transient
				// status; never stomp an active prompt.
				if !e.infobar.IsActive() {
					e.infobar.Message(status)
				}
			})
		},
		onApplyEdit: func(edit lspWorkspaceEdit) lsp.ApplyWorkspaceEditResponse {
			// workspace/applyEdit is a request, so the server needs a reply.
			// Marshal the mutation to the editor goroutine and wait for that
			// bounded piece of local work to finish. The wait needs a
			// deadline: postToMain drops when the queue is full, and an
			// undeliverable edit would otherwise hang this goroutine (and
			// the server's request) forever. state hands the edit to
			// exactly one side, so a late-arriving apply can't contradict
			// an "editor busy" answer already sent.
			var state atomic.Int32 // 0 queued, 1 editor applying, 2 abandoned
			done := make(chan lsp.ApplyWorkspaceEditResponse, 1)
			e.postToMain(func() {
				if !state.CompareAndSwap(0, 1) {
					return
				}
				if err := e.applyWorkspaceEdit(edit); err != nil {
					done <- lsp.ApplyWorkspaceEditResponse{Applied: false, FailureReason: err.Error()}
				} else {
					done <- lsp.ApplyWorkspaceEditResponse{Applied: true}
				}
			})
			select {
			case r := <-done:
				return r
			case <-time.After(applyEditTimeout):
				if state.CompareAndSwap(0, 2) {
					return lsp.ApplyWorkspaceEditResponse{Applied: false, FailureReason: "editor busy"}
				}
				// The editor claimed the edit just as the deadline
				// passed; it is mid-apply and answers shortly.
				return <-done
			}
		},
	}
}

// lspAsync runs a blocking LSP call off the event loop and hands the result
// back on it, so requests never freeze keystroke handling. Callbacks must
// re-validate editor state: the buffer may have changed while waiting.
func lspAsync[T any](e *Editor, call func() (T, error), done func(T, error)) {
	go func() {
		v, err := call()
		e.postToMain(func() { done(v, err) })
	}()
}

// hasBuffer reports whether b is still in the editor's buffer list (an
// async result may arrive after its buffer was deleted).
func (e *Editor) hasBuffer(b *Buffer) bool {
	for _, eb := range e.buffers {
		if eb == b {
			return true
		}
	}
	return false
}

// lspFilename returns the local filesystem path for a file:// URI. Servers
// can return other schemes (jdt://contents, deno:/asset) or malformed URIs,
// for which uri.Filename panics; those report ok=false instead of crashing
// the editor.
func lspFilename(u uri.URI) (name string, ok bool) {
	defer func() {
		if recover() != nil {
			name, ok = "", false
		}
	}()
	if !strings.HasPrefix(string(u), "file://") {
		return "", false
	}
	return u.Filename(), true
}

// lspFormattingOptions builds formatting options from the buffer's resolved
// tabsize/tabstospaces settings, so servers that honor them (unlike gopls)
// format according to the buffer's configuration rather than a hardcoded
// 4-space indent.
func (e *Editor) lspFormattingOptions(b *Buffer) lsp.FormattingOptions {
	opts := e.config.BufferOptions(b.Path, b.Filetype)
	tabsize, ok := GetOptInt(opts, "tabsize")
	if !ok || tabsize <= 0 {
		tabsize = 4
	}
	spaces, ok := GetOptBool(opts, "tabstospaces")
	if !ok {
		spaces = true
	}
	return lsp.FormattingOptions{TabSize: uint32(tabsize), InsertSpaces: spaces}
}

// handleDiagnostics receives pushed diagnostics and applies them to the
// matching buffer.
func (e *Editor) handleDiagnostics(params lsp.PublishDiagnosticsParams) {
	path, ok := lspFilename(params.URI)
	if !ok {
		return
	}
	// Deliver to any listed buffer, hidden ones included: the diagnostics
	// palette surfaces every buffer's diagnostics, so hidden buffers must
	// not be left with stale positions.
	for _, b := range e.buffers {
		absPath, _ := filepath.Abs(b.Path)
		if absPath == path {
			b.ClearDiagnostics()
			b.lspDiagnostics = append(b.lspDiagnostics, params.Diagnostics...)
			for _, d := range params.Diagnostics {
				_, col8 := b.Utf8Loc(int(d.Range.Start.Line), int(d.Range.Start.Character))
				dtype := DiagWarning
				if d.Severity == lsp.DiagnosticSeverityError {
					dtype = DiagError
				}
				b.AddDiagnostic(int(d.Range.Start.Line), col8, d.Message, dtype)
			}
			return
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

// registerLspBindings adds gd, K, gr, ]d, [d, = keybindings and LSP commands.
func (e *Editor) registerLspBindings() {
	// =: format operator (=motion, ==, visual mode), backed by
	// textDocument/rangeFormatting. Named opLspFormat to avoid colliding
	// with format.go's opFormat (the gq reflow operator).
	registerOperator(e.ks, "=", func(ks *KeyState, b *Buffer, start, end int) {
		e.lspFormatRange(b, start, end)
	})

	// gd: go to definition
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.lspGotoDefinition()
		ks.ResetAction()
	}, "g", "d")

	// K: hover
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.lspHover()
		ks.ResetAction()
	}, "K")

	// gr: find references
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.lspFindReferences()
		ks.ResetAction()
	}, "g", "r")

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

	// <C-k> in insert mode: signature help
	e.ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		e.lspSignatureHelpAt()
	}, "<C-k>")
}

// lspGotoDefinition requests definition at cursor and jumps there when the
// (asynchronous) answer arrives.
func (e *Editor) lspGotoDefinition() {
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

	lspAsync(e, func() ([]lsp.Location, error) {
		return s.Definition(absPath, pos)
	}, func(locs []lsp.Location, err error) {
		// Only jump if the user is still in the buffer that asked.
		if av := e.ActiveView(); av == nil || av.buf != b {
			return
		}
		if err != nil {
			e.infobar.Error(fmt.Sprintf("definition: %v", err))
			return
		}
		if len(locs) == 0 {
			e.infobar.Message("No definition found")
			return
		}

		// gd is a jump: <C-o> returns here, across files too.
		e.pushJump()
		e.jumpToLspLocation(b, locs[0])
	})
}

// jumpToLspLocation moves the cursor to loc, which is relative to origin:
// same file moves the cursor in place, a different file is opened first.
func (e *Editor) jumpToLspLocation(origin *Buffer, loc lsp.Location) {
	targetPath, ok := lspFilename(loc.URI)
	if !ok {
		e.infobar.Error(fmt.Sprintf("cannot open non-file location %s", string(loc.URI)))
		return
	}
	originAbsPath, _ := filepath.Abs(origin.Path)

	// Moving the origin's cursor in place is only correct while the origin
	// is what the user is looking at; a palette action can fire long after
	// the user switched or deleted buffers, and moving a hidden buffer's
	// cursor would silently do nothing. OpenFile dedups against the buffer
	// list, so the fallthrough shows the existing buffer.
	if av := e.ActiveView(); av != nil && av.buf == origin && targetPath == originAbsPath {
		target := origin.FromLspPosition(loc.Range.Start)
		*origin.Cursor() = origin.Cursor().MoveTo(target)
		return
	}
	if err := e.OpenFile(targetPath); err != nil {
		e.infobar.Error(fmt.Sprintf("open: %v", err))
		return
	}
	nb := e.ActiveView().buf
	target := nb.FromLspPosition(loc.Range.Start)
	*nb.Cursor() = nb.Cursor().MoveTo(target)
}

// lspHover requests hover info at cursor and displays it in the infobar
// when the (asynchronous) answer arrives.
func (e *Editor) lspHover() {
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

	lspAsync(e, func() (string, error) {
		return s.Hover(absPath, pos)
	}, func(info string, err error) {
		// A late answer for a buffer the user left, or while a prompt is
		// open, is dropped rather than displayed.
		if av := e.ActiveView(); av == nil || av.buf != b || e.infobar.IsActive() {
			return
		}
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
	})
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
		CommandDef{"lsp-actions", cmdLspActions, "lsp-actions: show code actions"},
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

func cmdLspActions(e *Editor, args []string) error {
	e.lspCodeActions()
	return nil
}

func cmdLspFormat(e *Editor, args []string) error {
	v := e.ActiveView()
	if v == nil || v.buf.lspServer == nil {
		return fmt.Errorf("no LSP server")
	}
	b := v.buf
	s := b.lspServer
	absPath, _ := filepath.Abs(b.Path)
	version := b.lspVersion

	fopts := e.lspFormattingOptions(b)
	lspAsync(e, func() ([]lsp.TextEdit, error) {
		return s.Format(absPath, fopts, lspRequestTimeout)
	}, func(edits []lsp.TextEdit, err error) {
		if err != nil {
			e.infobar.Error(fmt.Sprintf("format: %v", err))
			return
		}
		// The edits are positions into the buffer as it was at request
		// time: applying them after an edit (or to a deleted buffer)
		// would corrupt it.
		if !e.hasBuffer(b) || b.lspVersion != version {
			e.infobar.Error("format: buffer changed, not applied")
			return
		}
		b.UndoBarrier()
		applyTextEdits(b, edits)
		e.infobar.Message("Formatted")
	})
	return nil
}

// applyFormatOnSave runs textDocument/formatting synchronously and applies
// the resulting edits before b is written to disk, when the buffer's
// resolved "autoformat" option is on (off by default, see
// embed/options.toml) and its LSP server supports formatting. This is
// called from Buffer.beforeSave (set in configureView), which every save
// path (:w, :wa, ZZ, sudo save) already runs through.
//
// Formatting here blocks the caller until the request finishes (bounded by
// lspRequestTimeout), unlike :lsp-format's async version — deliberately, so
// the file that lands on disk always reflects the formatted content and a
// compound command like :wq still saves before quitting. A slow or hung
// formatter delays that one save; it never corrupts it, and any failure
// (other than the server simply not supporting formatting) is reported but
// does not block the save itself.
func (e *Editor) applyFormatOnSave(b *Buffer) {
	if b.lspServer == nil {
		return
	}
	opts := e.config.BufferOptions(b.Path, b.Filetype)
	if on, _ := GetOptBool(opts, "autoformat"); !on {
		return
	}
	absPath, _ := filepath.Abs(b.Path)
	edits, err := b.lspServer.Format(absPath, e.lspFormattingOptions(b), formatOnSaveTimeout)
	if err != nil {
		if err != ErrLspNotSupported {
			// Post the error to the main queue rather than the infobar
			// directly: the save that invoked this hook posts its own
			// '"path" written' message afterwards in the same turn, which
			// would overwrite (and hide) an error set here. The queued
			// error lands after it.
			msg := fmt.Sprintf("format on save: %v", err)
			e.postToMain(func() { e.infobar.Error(msg) })
		}
		return
	}
	b.UndoBarrier()
	applyTextEdits(b, edits)
}

// lspFormatRange formats the byte-offset range [start, end) via
// textDocument/rangeFormatting; the = operator's function (see
// registerLspBindings). Multiple concurrent calls (multi-cursor =, or ==
// fired several times in quick succession) each snapshot lspVersion
// independently, so only the first response to land applies cleanly — later
// ones see a bumped version and are rejected, same as a plain edit racing
// :lsp-format.
func (e *Editor) lspFormatRange(b *Buffer, start, end int) {
	if b.lspServer == nil {
		e.infobar.Error("No LSP server")
		return
	}
	s := b.lspServer
	absPath, _ := filepath.Abs(b.Path)
	version := b.lspVersion
	sl, sc := b.LineColAt(start)
	el, ec := b.LineColAt(end)
	rng := lsp.Range{Start: b.LspPosition(sl, sc), End: b.LspPosition(el, ec)}

	fopts := e.lspFormattingOptions(b)
	lspAsync(e, func() ([]lsp.TextEdit, error) {
		return s.RangeFormatting(absPath, rng, fopts)
	}, func(edits []lsp.TextEdit, err error) {
		if err != nil {
			if err == ErrLspNotSupported {
				e.infobar.Error("Range formatting not supported")
			} else {
				e.infobar.Error(fmt.Sprintf("format: %v", err))
			}
			return
		}
		if !e.hasBuffer(b) || b.lspVersion != version {
			e.infobar.Error("format: buffer changed, not applied")
			return
		}
		b.UndoBarrier()
		applyTextEdits(b, edits)
		e.infobar.Message("Formatted")
	})
}

// applyTextEdits applies LSP text edits to a buffer. Edits are applied
// bottom-up so earlier positions stay valid; the spec does not guarantee
// the server returns them sorted, but it does require that multiple
// inserts at the same position apply in array order — hence the stable
// ascending sort walked backwards.
func applyTextEdits(b *Buffer, edits []lsp.TextEdit) {
	sorted := make([]lsp.TextEdit, len(edits))
	copy(sorted, edits)
	sort.SliceStable(sorted, func(i, j int) bool {
		si, sj := sorted[i].Range.Start, sorted[j].Range.Start
		if si.Line != sj.Line {
			return si.Line < sj.Line
		}
		return si.Character < sj.Character
	})
	for i := len(sorted) - 1; i >= 0; i-- {
		edit := sorted[i]
		start := b.FromLspPosition(edit.Range.Start)
		end := b.FromLspPosition(edit.Range.End)
		if end < start {
			start, end = end, start
		}
		if end > start {
			b.Remove(start, end)
		}
		if len(edit.NewText) > 0 {
			b.Insert(start, []byte(edit.NewText))
		}
	}
}
