package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/zyedidia/gotcl"
)

// Editor is the top-level editor state, managing the screen, tabs, and
// key dispatch.
type Editor struct {
	screen tcell.Screen
	config *Config
	theme  *Theme

	ks     *KeyState
	regs   *RegisterSet
	interp *gotcl.Interp

	tabs   []*Tab
	curtab int

	infobar    *InfoBar
	search     SearchState
	lspManager *LspManager
	completion EditorCompletion
	palette    Palette
	// completionGen invalidates in-flight async completion requests: a
	// callback only opens the menu if its generation is still current.
	completionGen int

	// Bracketed paste collection (see paste.go): between the EventPaste
	// markers, key events accumulate in pasteBuf instead of dispatching.
	pasting  bool
	pasteBuf strings.Builder

	// comments maps filetype → line-comment prefix (from comments.toml).
	comments map[string]string

	// buffers is the buffer list: every buffer opened this session, in
	// creation order, including hidden ones (not shown in any pane).
	buffers []*Buffer
	bufnum  int     // last assigned buffer number
	altBuf  *Buffer // alternate buffer (:b #): previously shown buffer

	// jumps is the jump list (<C-o>/<C-i>).
	jumps JumpList

	// mainq holds actions posted from background goroutines (LSP receive
	// loop, file watchers) to run on the main event-loop goroutine.
	mainq chan func()

	running bool
	w, h    int
}

// postToMain schedules fn to run on the main event-loop goroutine and wakes
// the event loop. Editor state must only be mutated there.
func (e *Editor) postToMain(fn func()) {
	if e.mainq == nil {
		return
	}
	select {
	case e.mainq <- fn:
	default:
		// Queue full: drop. Watcher reloads re-fire on the next tick, but
		// a dropped diagnostics push is lost until the server republishes,
		// so the queue is sized generously.
	}
	if e.screen != nil {
		e.screen.PostEvent(tcell.NewEventInterrupt(nil))
	}
}

// drainMain runs all posted actions. Called from the event loop.
func (e *Editor) drainMain() {
	for {
		select {
		case fn := <-e.mainq:
			fn()
		default:
			return
		}
	}
}

// NewEditor creates a new editor with the given screen, config, and theme.
func NewEditor(screen tcell.Screen, cfg *Config, th *Theme) *Editor {
	w, h := screen.Size()
	regs := NewRegisterSet()
	ks := NewKeyState(nil, regs)
	SetupBindings(ks)

	ed := &Editor{
		screen:  screen,
		config:  cfg,
		theme:   th,
		ks:      ks,
		regs:    regs,
		infobar: NewInfoBar(),
		mainq:   make(chan func(), 1024),
		w:       w,
		h:       h,
	}

	ks.activeView = func() *View {
		return ed.ActiveView()
	}
	ks.dispatch = ed.dispatchKey
	ks.recordJump = ed.pushJump

	// Comment prefix lookup for gc (comment toggle) and gq (formatting).
	// A missing entry is not an error: gq formats plain text in any
	// filetype, and gc simply does nothing.
	ed.comments = cfg.LoadComments()
	ks.commentPrefix = func(b *Buffer) string {
		return ed.comments[b.Filetype]
	}

	ks.onModeChange = func(mode ModeID) {
		if ed.screen == nil {
			return
		}
		switch mode {
		case ModeInsert, ModeReplace:
			ed.screen.SetCursorStyle(tcell.CursorStyleSteadyBar)
		case ModeOperatorPending:
			ed.screen.SetCursorStyle(tcell.CursorStyleSteadyUnderline)
		case ModeVisual, ModeVisualLine, ModeVisualBlock:
			ed.screen.SetCursorStyle(tcell.CursorStyleSteadyUnderline)
		default:
			ed.screen.SetCursorStyle(tcell.CursorStyleSteadyBlock)
		}
	}
	ks.onCursorStyle = func(waiting bool) {
		if ed.screen == nil {
			return
		}
		if waiting {
			ed.screen.SetCursorStyle(tcell.CursorStyleSteadyUnderline)
		} else {
			ed.screen.SetCursorStyle(tcell.CursorStyleSteadyBlock)
		}
	}

	if err := ed.initClipboard(); err != nil {
		log.Print(err)
	}
	ed.initLsp()
	ed.initTCL()
	ed.registerEditorBindings()
	ed.registerSearchBindings()
	ed.registerLspBindings()
	ed.registerCompletionBindings()

	// Load command history from disk.
	if b, ok := GetOptBool(cfg.opts.top, "savehistory"); !ok || b {
		ed.infobar.LoadHistory()
	}

	return ed
}

func (e *Editor) registerEditorBindings() {
	// ZQ: force quit
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.running = false
	}, "Z", "Q")

	// ZZ: save and quit (refusing if another buffer has unsaved changes,
	// as vim does on the last window)
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := e.ActiveView(); v != nil && v.buf.Path != "" {
			if err := v.buf.Save(); err != nil {
				e.infobar.Error(err.Error())
				return
			}
		}
		if mb := e.anyModifiedBuffer(); mb != nil {
			e.infobar.Error(fmt.Sprintf("No write since last change for %s", bufDisplayName(mb)))
			return
		}
		e.running = false
	}, "Z", "Z")

	// :: enter command mode with tab completion
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.infobar.StartPrompt(":", func(input string) {
			e.RunCommand(input)
		})
		e.infobar.SetCompleter(cmdCompleter(e))
	}, ":")

	// Ctrl-P: searchable palette (files, text, buffers, and commands).
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.startPalette("")
	}, "<C-p>")

	// bindCW binds an action to <C-w> followed by key, and also
	// <C-w> followed by <C-key> (for when Ctrl is held across both).
	bindCW := func(key string, fn KeyAction) {
		e.ks.modes[ModeNormal].Bindings.Bind(fn, "<C-w>", key)
		if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
			e.ks.modes[ModeNormal].Bindings.Bind(fn, "<C-w>", fmt.Sprintf("<C-%s>", key))
		}
	}

	// Ctrl-W w: next pane
	bindCW("w", func(ks *KeyState) {
		e.ActiveTab().NextPane()
		e.syncActiveBuffer()
		ks.ResetAction()
	})

	// Ctrl-W h: focus left (also <BS> since <C-h> == Backspace in terminals)
	focusLeft := func(ks *KeyState) {
		e.ActiveTab().FocusLeft()
		e.syncActiveBuffer()
		ks.ResetAction()
	}
	bindCW("h", focusLeft)
	e.ks.modes[ModeNormal].Bindings.Bind(focusLeft, "<C-w>", KeyLeft)
	e.ks.modes[ModeNormal].Bindings.Bind(focusLeft, "<C-w>", KeyBacksp)

	// Ctrl-W l: focus right
	bindCW("l", func(ks *KeyState) {
		e.ActiveTab().FocusRight()
		e.syncActiveBuffer()
		ks.ResetAction()
	})
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.ActiveTab().FocusRight()
		e.syncActiveBuffer()
		ks.ResetAction()
	}, "<C-w>", KeyRight)

	// Ctrl-W k: focus up
	bindCW("k", func(ks *KeyState) {
		e.ActiveTab().FocusUp()
		e.syncActiveBuffer()
		ks.ResetAction()
	})
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.ActiveTab().FocusUp()
		e.syncActiveBuffer()
		ks.ResetAction()
	}, "<C-w>", KeyUp)

	// Ctrl-W j: focus down
	bindCW("j", func(ks *KeyState) {
		e.ActiveTab().FocusDown()
		e.syncActiveBuffer()
		ks.ResetAction()
	})
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.ActiveTab().FocusDown()
		e.syncActiveBuffer()
		ks.ResetAction()
	}, "<C-w>", KeyDown)

	// Ctrl-W v: vertical split
	bindCW("v", func(ks *KeyState) {
		e.VSplit(nil)
		ks.ResetAction()
	})

	// Ctrl-W s: horizontal split
	bindCW("s", func(ks *KeyState) {
		e.HSplit(nil)
		ks.ResetAction()
	})

	// Ctrl-W q: close pane
	bindCW("q", func(ks *KeyState) {
		e.ClosePane()
		ks.ResetAction()
	})

	// Ctrl-O / Ctrl-I (Tab): jump list navigation. Terminals deliver
	// Ctrl-I as Tab, so Tab is the forward binding, as in vim.
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		for i := 0; i < ks.Count(); i++ {
			e.jumpBack()
		}
		ks.ResetAction()
	}, "<C-o>")
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		for i := 0; i < ks.Count(); i++ {
			e.jumpForward()
		}
		ks.ResetAction()
	}, KeyTab)

	// gt: next tab
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.NextTab()
		ks.ResetAction()
	}, "g", "t")

	// gT: previous tab
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.PrevTab()
		ks.ResetAction()
	}, "g", "T")
}

// --- Tab management ---

// ActiveTab returns the current tab.
func (e *Editor) ActiveTab() *Tab {
	if len(e.tabs) == 0 {
		return nil
	}
	return e.tabs[e.curtab]
}

// ActiveView returns the focused view in the current tab.
func (e *Editor) ActiveView() *View {
	t := e.ActiveTab()
	if t == nil {
		return nil
	}
	return t.ActiveView()
}

// syncActiveBuffer updates the KeyState to point at the active view's buffer.
func (e *Editor) syncActiveBuffer() {
	if v := e.ActiveView(); v != nil {
		e.ks.SetBuffer(v.buf)
	}
}

// --- Buffer list ---

// registerBuffer adds a buffer to the buffer list, assigning it a stable
// number. Idempotent.
func (e *Editor) registerBuffer(b *Buffer) {
	for _, ob := range e.buffers {
		if ob == b {
			return
		}
	}
	e.bufnum++
	b.bufnum = e.bufnum
	e.buffers = append(e.buffers, b)
}

// bufDisplayName returns a buffer's name for messages and :ls.
func bufDisplayName(b *Buffer) string {
	if b.Path == "" {
		return "[No Name]"
	}
	return b.Path
}

// anyModifiedBuffer returns a buffer with unsaved changes, or nil. Hidden
// buffers count: quitting must not silently drop their edits.
func (e *Editor) anyModifiedBuffer() *Buffer {
	for _, b := range e.buffers {
		if b.Modified() {
			return b
		}
	}
	return nil
}

// lastPane reports whether only one pane remains (closing it exits).
func (e *Editor) lastPane() bool {
	return len(e.tabs) == 1 && e.tabs[0].NumPanes() == 1
}

// showBuffer displays b in the active pane. The pane's previous buffer
// stays in the buffer list and becomes the alternate buffer.
func (e *Editor) showBuffer(b *Buffer) {
	if cur := e.ActiveView(); cur != nil && cur.buf == b {
		return
	}
	v := e.newViewWithOptions(b)
	v.savedCursor = *b.Cursor()
	e.showView(v)
}

// deleteBuffer removes b from the buffer list, replaces it in any pane
// showing it (with the alternate buffer, another listed buffer, or a new
// empty one), and releases its background resources.
func (e *Editor) deleteBuffer(b *Buffer) {
	var repl *Buffer
	if e.altBuf != nil && e.altBuf != b {
		repl = e.altBuf
	} else {
		for _, ob := range e.buffers {
			if ob != b {
				repl = ob
				break
			}
		}
	}
	for i, ob := range e.buffers {
		if ob == b {
			e.buffers = append(e.buffers[:i], e.buffers[i+1:]...)
			break
		}
	}
	if e.altBuf == b {
		e.altBuf = nil
	}
	e.jumps.prune(e.bufferListed)

	for _, t := range e.tabs {
		for id, v := range t.panes {
			if v.buf != b {
				continue
			}
			e.saveViewState(v)
			if repl == nil {
				nb := NewEmptyBuffer()
				repl = e.configureView(nb, "").buf
				e.registerBuffer(repl)
			}
			nv := e.newViewWithOptions(repl)
			nv.savedCursor = *repl.Cursor()
			t.panes[id] = nv
		}
		t.Resize(t.w, t.h)
	}
	e.syncActiveBuffer()

	b.StopWatcher()
	b.LspClose()
}

// NewTabWithView creates a new tab containing the given view.
func (e *Editor) NewTabWithView(v *View) {
	e.registerBuffer(v.buf)
	t := NewTab(v, e.w, e.h-1-e.tabBarHeight())
	e.tabs = append(e.tabs, t)
	e.curtab = len(e.tabs) - 1
	e.resizeTabs()
	e.syncActiveBuffer()
}

// resizeTabs resizes all tabs to account for the current tab bar height.
func (e *Editor) resizeTabs() {
	th := e.tabBarHeight()
	for _, t := range e.tabs {
		t.Resize(e.w, e.h-1-th)
	}
}

// NextTab switches to the next tab.
func (e *Editor) NextTab() {
	if len(e.tabs) > 1 {
		e.curtab = (e.curtab + 1) % len(e.tabs)
		e.syncActiveBuffer()
	}
}

// PrevTab switches to the previous tab.
func (e *Editor) PrevTab() {
	if len(e.tabs) > 1 {
		e.curtab = (e.curtab - 1 + len(e.tabs)) % len(e.tabs)
		e.syncActiveBuffer()
	}
}

// CloseTab closes the current tab.
func (e *Editor) CloseTab() {
	if len(e.tabs) <= 1 {
		e.running = false
		return
	}
	closed := e.tabs[e.curtab]
	// Sync the focused pane's cursor into its view before discarding, so
	// the state saved by releaseViewBuffer is current. Inactive panes
	// already keep theirs in savedCursor.
	if av := closed.ActiveView(); av != nil {
		av.Deactivate()
	}
	e.tabs = append(e.tabs[:e.curtab], e.tabs[e.curtab+1:]...)
	if e.curtab >= len(e.tabs) {
		e.curtab = len(e.tabs) - 1
	}
	e.resizeTabs()
	e.syncActiveBuffer()
	for _, v := range closed.panes {
		e.releaseViewBuffer(v)
	}
}

// --- Split management ---

// VSplit creates a vertical split. If args is non-nil, opens that file;
// otherwise duplicates the current buffer.
func (e *Editor) VSplit(args []string) {
	v := e.makeNewView(args)
	if v == nil {
		return
	}
	e.registerBuffer(v.buf)
	e.ActiveTab().VSplit(v)
	e.syncActiveBuffer()
}

// HSplit creates a horizontal split.
func (e *Editor) HSplit(args []string) {
	v := e.makeNewView(args)
	if v == nil {
		return
	}
	e.registerBuffer(v.buf)
	e.ActiveTab().HSplit(v)
	e.syncActiveBuffer()
}

// ClosePane closes the active pane. If it's the last pane in the tab,
// closes the tab.
func (e *Editor) ClosePane() {
	t := e.ActiveTab()
	if t == nil {
		return
	}
	removed := t.ActiveView()
	if removed != nil {
		// Sync the cursor into the view before Unsplit hands the buffer's
		// cursor to the surviving pane, so releaseViewBuffer saves the
		// closed pane's own position.
		removed.Deactivate()
	}
	if !t.Unsplit() {
		e.CloseTab()
		return
	}
	e.syncActiveBuffer()
	e.releaseViewBuffer(removed)
}

// makeNewView creates a view for a split. Opens the file from args[0] (or
// shares its buffer if already open); with no args, views the current
// buffer.
func (e *Editor) makeNewView(args []string) *View {
	if len(args) > 0 && args[0] != "" {
		v, err := e.viewForFile(args[0])
		if err != nil {
			e.infobar.Error(fmt.Sprintf("open: %v", err))
			return nil
		}
		return v
	}
	// Duplicate current buffer in a new view (no re-init of syntax/LSP/watcher).
	cur := e.ActiveView()
	if cur == nil {
		return nil
	}
	v := e.newViewWithOptions(cur.buf)
	// Initialize the new view's cursor from the current cursor position.
	v.savedCursor = *cur.buf.Cursor()
	return v
}

// --- Opening files ---

// expandTilde replaces a leading "~" or "~/" with the user's home directory,
// the way a shell would, so a path typed in the command bar (":e ~/notes")
// reaches the same file as one typed in a shell. Only the bare form is
// expanded: "~user" and a tilde anywhere but the front are left alone, and
// so is everything else when the home directory can't be determined.
func expandTilde(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	rest := path[1:]
	if rest != "" && rest[0] != '/' && rest[0] != filepath.Separator {
		return path // ~user
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rest == "" {
		return home
	}
	return filepath.Join(home, rest[1:])
}

// samePath reports whether two paths refer to the same file, compared
// absolutely.
func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && aa == bb
}

// findBuffer returns the listed buffer for path (hidden ones included), or
// nil. Keeping one buffer per file means shared content, a single LSP
// document/version counter, and a single file watcher, however many panes
// show it — and reopening a hidden file's path resurfaces its buffer.
func (e *Editor) findBuffer(path string) *Buffer {
	for _, b := range e.buffers {
		if b.Path != "" && samePath(b.Path, path) {
			return b
		}
	}
	return nil
}

// viewForFile returns a view for path: a new view of the already-open
// buffer when the file is open somewhere, otherwise a freshly configured
// buffer read from disk.
func (e *Editor) viewForFile(path string) (*View, error) {
	path = expandTilde(path)
	if b := e.findBuffer(path); b != nil {
		v := e.newViewWithOptions(b)
		v.savedCursor = *b.Cursor()
		return v, nil
	}
	// Refuse FIFOs: reading one blocks until a writer appears, freezing
	// the editor (the write-probe in isReadonly has the same hazard).
	if fi, err := os.Stat(path); err == nil && fi.Mode()&os.ModeNamedPipe != 0 {
		return nil, fmt.Errorf("%s is a named pipe", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			data = []byte{}
		} else {
			return nil, err
		}
	}
	buf, err := NewBuffer(data, path)
	if err != nil {
		return nil, err
	}
	return e.configureView(buf, path), nil
}

// OpenFile opens a file in a new view. If no tabs exist, creates one.
// Otherwise adds to the current tab.
func (e *Editor) OpenFile(path string) error {
	v, err := e.viewForFile(path)
	if err != nil {
		return err
	}
	e.showView(v)
	return nil
}

// OpenFiles opens several files: the first in the current pane, the rest
// each in their own tab, focusing the first.
func (e *Editor) OpenFiles(paths []string) error {
	var firstErr error
	for i, path := range paths {
		var err error
		if i == 0 {
			err = e.OpenFile(path)
		} else {
			err = e.OpenFileInTab(path)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if len(paths) > 1 && len(e.tabs) > 0 {
		e.curtab = 0
		e.syncActiveBuffer()
	}
	return firstErr
}

// OpenFileInTab opens a file in a new tab.
func (e *Editor) OpenFileInTab(path string) error {
	v, err := e.viewForFile(path)
	if err != nil {
		return err
	}
	e.NewTabWithView(v)
	return nil
}

// OpenEmpty opens an empty buffer.
func (e *Editor) OpenEmpty() {
	buf := NewEmptyBuffer()
	v := e.configureView(buf, "")
	e.showView(v)
}

// showView places a fully configured view on screen: in a new tab if none
// exists, otherwise replacing the active pane. The replaced view's buffer
// is released if no other pane shows it.
func (e *Editor) showView(v *View) {
	e.registerBuffer(v.buf)
	if len(e.tabs) == 0 {
		e.NewTabWithView(v)
		return
	}
	t := e.ActiveTab()
	old := t.panes[t.cur]
	if old != nil {
		// The replaced view was the focused pane: sync its cursor so
		// releaseViewBuffer saves its state.
		old.Deactivate()
		if old.buf != v.buf {
			e.altBuf = old.buf
		}
	}
	t.panes[t.cur] = v
	t.Resize(t.w, t.h)
	e.syncActiveBuffer()
	e.releaseViewBuffer(old)
}

// saveViewState persists a discarded view's state (undo history and
// cursor/viewport), as persistState does at exit, so closing a pane or tab
// mid-session doesn't lose its position. The view's own cursor is read from
// savedCursor; close sites Deactivate a focused view before discarding it.
func (e *Editor) saveViewState(v *View) {
	if v == nil || v.buf.Path == "" {
		return
	}
	if saveUndo, _ := GetOptBool(e.config.opts.top, "saveundo"); saveUndo {
		v.buf.SaveUndoHistory()
	}
	if saveCursor, _ := GetOptBool(e.config.opts.top, "savecursor"); saveCursor && !v.buf.Modified() {
		v.SaveInactiveCursorPos()
	}
}

// releaseViewBuffer persists a discarded view's state. The buffer itself
// stays alive in the buffer list (hidden), keeping its watcher and LSP
// document; deleteBuffer releases those.
func (e *Editor) releaseViewBuffer(v *View) {
	if v == nil {
		return
	}
	e.saveViewState(v)
}

// newViewWithOptions creates a View for a buffer and applies display options
// from the config. Does not initialize buffer-level state (syntax, LSP, etc.).
func (e *Editor) newViewWithOptions(buf *Buffer) *View {
	v := NewView(buf, 4)
	e.applyViewOptions(v)
	return v
}

// applyViewOptions resolves the buffer's options (using its path and
// filetype) and applies them to the view's display settings.
func (e *Editor) applyViewOptions(v *View) {
	opts := e.config.BufferOptions(v.buf.Path, v.buf.Filetype)
	v.Opts = opts

	if n, ok := GetOptInt(opts, "tabsize"); ok && n > 0 {
		v.vis.TabSize = n
	}
	if b, ok := GetOptBool(opts, "linenums"); ok {
		v.LineNums = b
	}
	if b, ok := GetOptBool(opts, "softwrap"); ok {
		v.SoftWrap = b
	}
	if b, ok := GetOptBool(opts, "cursorline"); ok {
		v.CursorLine = b
	}
	if n, ok := GetOptInt(opts, "scrollmargin"); ok {
		v.ScrollMargin = n
	}
	if n, ok := GetOptInt(opts, "hscrollmargin"); ok {
		v.HScrollMargin = n
	}
}

// refreshViewOptions re-resolves and applies options for every open view
// (after :set changes an option).
func (e *Editor) refreshViewOptions() {
	for _, t := range e.tabs {
		for _, v := range t.panes {
			e.applyViewOptions(v)
		}
	}
}

// syntaxEnabled reports whether the syntax option is on in a resolved
// option map (missing means on).
func syntaxEnabled(opts map[string]any) bool {
	on, ok := GetOptBool(opts, "syntax")
	return !ok || on
}

// applySyntaxOption enables or disables highlighting for every listed
// buffer according to its resolved syntax option (the option is
// per-filetype/glob resolvable, so buffers can differ).
func (e *Editor) applySyntaxOption() {
	for _, b := range e.buffers {
		opts := e.config.BufferOptions(b.Path, b.Filetype)
		if !syntaxEnabled(opts) {
			b.DisableSyntax()
		} else if b.syntax == nil && b.Filetype != "" {
			b.InitSyntax(e.config, b.Filetype)
		}
	}
}

// lspEnabled reports whether the lsp option is on in a resolved option map
// (missing means on).
func lspEnabled(opts map[string]any) bool {
	on, ok := GetOptBool(opts, "lsp")
	return !ok || on
}

// applyLspOption attaches or detaches language servers for every listed
// buffer according to its resolved lsp option (per-filetype/glob
// resolvable, so buffers can differ). Detached buffers close their
// document and drop stale diagnostics; servers left serving no buffer are
// shut down, and a later re-enable starts a fresh one.
func (e *Editor) applyLspOption() {
	used := make(map[*LspServer]bool)
	for _, b := range e.buffers {
		opts := e.config.BufferOptions(b.Path, b.Filetype)
		if !lspEnabled(opts) {
			if b.lspServer != nil {
				b.LspClose()
				b.ClearDiagnostics()
				b.ClearInlayHints()
			}
		} else if b.lspServer == nil && b.Filetype != "" {
			e.initBufferLsp(b, b.Filetype)
		}
		if b.lspServer != nil {
			used[b.lspServer] = true
		}
	}
	e.lspManager.ShutdownUnused(used)
}

// configureView creates a View and fully initializes the buffer (syntax,
// LSP, file watcher, readonly detection). Use for newly opened files.
func (e *Editor) configureView(buf *Buffer, path string) *View {
	// Detect the filetype first: option resolution and the view's display
	// settings depend on it ([filetype] sections in options.toml).
	ft := DetectFiletype(e.config, path, buf.GetLine(0))
	buf.Filetype = ft

	v := e.newViewWithOptions(buf)

	buf.updateModTime()
	// The watcher only detects external changes; the reload itself runs on
	// the main goroutine so the buffer is never mutated concurrently.
	buf.onReload = func(b *Buffer) {
		e.postToMain(func() {
			if !b.Modified() && b.ExternallyModified() {
				b.Reload()
			}
		})
	}
	buf.onHighlight = func() {
		if e.screen != nil {
			e.screen.PostEvent(tcell.NewEventInterrupt(nil))
		}
	}
	buf.beforeSave = func(b *Buffer) {
		e.applyFormatOnSave(b)
	}
	buf.StartWatcher()

	if ft != "" {
		if syntaxEnabled(v.Opts) {
			buf.InitSyntax(e.config, ft)
		}
		if lspEnabled(v.Opts) {
			e.initBufferLsp(buf, ft)
		}
	}

	if path != "" && isReadonly(path) {
		buf.readonly = true
	}

	// Restore persisted state.
	if path != "" {
		if b, ok := GetOptBool(e.config.opts.top, "saveundo"); !ok || b {
			buf.LoadUndoHistory()
		}
		if b, ok := GetOptBool(e.config.opts.top, "savecursor"); !ok || b {
			v.LoadCursorPos()
		}
	}

	return v
}

// tabBarHeight returns 1 if there are multiple tabs, 0 otherwise.
func (e *Editor) tabBarHeight() int {
	if len(e.tabs) > 1 {
		return 1
	}
	return 0
}

// Resize handles terminal resize events.
func (e *Editor) Resize(w, h int) {
	e.w, e.h = w, h
	e.resizeTabs()
	e.screen.Sync()
}

// checkExternalModified reloads or prompts if the file changed on disk.
// Returns true if a prompt was opened, in which case the triggering
// keystroke must not be dispatched (it would answer the prompt).
func (e *Editor) checkExternalModified() bool {
	v := e.ActiveView()
	if v == nil || e.infobar.IsActive() {
		return false
	}
	b := v.buf
	if !b.ExternallyModified() {
		return false
	}
	if !b.Modified() {
		if err := b.Reload(); err != nil {
			e.infobar.Error(fmt.Sprintf("reload: %v", err))
		} else {
			e.infobar.Message(fmt.Sprintf("\"%s\" reloaded", b.Path))
		}
		return false
	}
	e.infobar.Prompt(fmt.Sprintf("\"%s\" changed on disk. Reload? (y/n)", b.Path), func(key string) {
		if key == "y" {
			if err := b.Reload(); err != nil {
				e.infobar.Error(fmt.Sprintf("reload: %v", err))
			} else {
				e.infobar.Message(fmt.Sprintf("\"%s\" reloaded", b.Path))
			}
		} else {
			b.updateModTime()
		}
	})
	return true
}

// Message displays a message in the info bar.
func (e *Editor) Message(msg string) {
	e.infobar.Message(msg)
}

// Error displays an error in the info bar.
func (e *Editor) Error(msg string) {
	e.infobar.Error(msg)
}

// persistState saves undo history, cursor positions, and command history
// to disk.
func (e *Editor) persistState() {
	saveUndo, _ := GetOptBool(e.config.opts.top, "saveundo")
	saveCursor, _ := GetOptBool(e.config.opts.top, "savecursor")
	saveHistory, _ := GetOptBool(e.config.opts.top, "savehistory")

	// Save per-buffer state.
	seen := make(map[*Buffer]bool)
	for _, t := range e.tabs {
		for _, v := range t.panes {
			b := v.buf
			if seen[b] {
				continue
			}
			seen[b] = true
			if saveUndo {
				b.SaveUndoHistory()
			}
			if saveCursor && !b.Modified() {
				// A tab's focused pane owns the buffer's live cursor;
				// other panes keep theirs in savedCursor.
				if v == t.ActiveView() {
					v.SaveCursorPos()
				} else {
					v.SaveInactiveCursorPos()
				}
			}
		}
	}

	// Save command history.
	if saveHistory {
		e.infobar.SaveHistory()
	}
}

// Run starts the main event loop. Shuts down LSP servers on exit.
func (e *Editor) Run() {
	defer e.persistState()
	defer e.lspManager.ShutdownAll()
	defer e.screen.SetCursorStyle(tcell.CursorStyleDefault)

	e.screen.SetCursorStyle(tcell.CursorStyleSteadyBlock)
	e.running = true
	e.Display()

	for e.running {
		ev := e.screen.PollEvent()
		if ev == nil {
			break
		}

		// Coalesce queued input: handle every pending event, then render
		// one frame. Without this the editor renders a frame per key
		// event, so a key-repeat rate above the frame rate makes input
		// queue up behind redraws and scrolling keeps running after the
		// key is released — worst on slow machines, where scrolling
		// frames are the most expensive. The batch is capped so a
		// continuous event stream can never starve rendering.
		e.handleEvent(ev)
		for i := 0; e.running && i < 128 && e.screen.HasPendingEvent(); i++ {
			ev = e.screen.PollEvent()
			if ev == nil {
				break
			}
			e.handleEvent(ev)
		}

		if e.pasting {
			// Mid-paste: keep collecting without rendering.
			continue
		}
		e.Display()
	}
}

// handleEvent processes a single event (no rendering).
func (e *Editor) handleEvent(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		if e.pasting {
			// Paste content in flight: collect it without key dispatch.
			e.collectPasteKey(ev)
			return
		}
		key := keyEventToString(ev)
		if key == "" {
			return
		}
		if e.checkExternalModified() {
			// A reload prompt was just opened: show it and let the NEXT
			// keystroke answer it, not this one.
			return
		}

		e.dispatchKey(key)
	case *tcell.EventPaste:
		if ev.Start() {
			e.pasting = true
			e.pasteBuf.Reset()
			return
		}
		e.pasting = false
		text := e.pasteBuf.String()
		e.pasteBuf.Reset()
		e.pasteText(text)
	case *tcell.EventResize:
		w, h := ev.Size()
		e.Resize(w, h)
	case *tcell.EventClipboard:
		// A terminal answered an OSC 52 clipboard read: refresh the
		// '+' register with the received content.
		e.regs.storeClipboard(ev.Data())
	case *tcell.EventInterrupt:
		e.drainMain()
	}
}

// dispatchKey routes one key: to the infobar prompt or completion menu when
// one is active, otherwise into the vim state machine. Macro replay routes
// its keys through here too, so recorded ':' and '/' interactions work; the
// infobar and completion branches record their keys explicitly (HandleKey
// records its own).
func (e *Editor) dispatchKey(key string) {
	if e.palette.active {
		e.ks.RecordMacroKey(key)
		e.handlePaletteKey(key)
	} else if e.infobar.IsActive() {
		e.ks.RecordMacroKey(key)
		e.infobar.HandleKey(key)
	} else if e.hasCompletion() {
		e.ks.RecordMacroKey(key)
		e.handleCompletionKey(key)
	} else {
		e.infobar.Clear()
		e.ks.HandleKey(key)
	}
}

// Display renders the entire screen. There is no whole-screen clear: the
// components tile the screen completely each frame (panes blank past their
// buffer's end, and the bars pad their rows), so a clear would only add
// per-cell work.
func (e *Editor) Display() {

	// Recalculate Vx for all cursors unless the last action was a purely
	// vertical motion (j/k/gj/gk/Ctrl-D/Ctrl-U) or a key sequence is still
	// pending (the g of gj: the cursor hasn't moved, and recalculating
	// would drop a display-column chain).
	if !e.ks.vertical && len(e.ks.keys) == 0 {
		b := e.ks.Buf()
		for i := 0; i < b.NumCursors(); i++ {
			b.cursors[i].Vx = b.VisualCol(b.cursors[i].Pos)
		}
		e.ks.displayVx = false
	}
	e.ks.vertical = false

	t := e.ActiveTab()
	if t == nil {
		e.screen.Fill(' ', e.theme.Default().TCellStyle())
		e.screen.Show()
		return
	}

	// Draw tab bar at the top if there are multiple tabs.
	th := e.tabBarHeight()
	if th > 0 {
		e.drawTabBar(0)
	}

	// Draw all panes in the tab (offset by tab bar height). With multiple
	// cursors the view draws every cursor (primary included) as a fake
	// block cursor, so the hardware cursor is hidden rather than doubling
	// up on the primary.
	multi := e.ks.Buf().NumCursors() > 1
	if multi {
		e.screen.HideCursor()
	}
	// Cells arrive in long runs of one style; memoizing the last tcell
	// conversion skips it for nearly every cell.
	var lastStyle Style
	var lastTStyle tcell.Style
	haveStyle := false
	t.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		if !haveStyle || style != lastStyle {
			lastStyle, lastTStyle = style, style.TCellStyle()
			haveStyle = true
		}
		e.screen.SetContent(x, y+th, mainc, combc, lastTStyle)
	}, func(x, y int, main bool) {
		if main && !multi && !e.infobar.IsActive() && !e.infobar.showCursor {
			e.screen.ShowCursor(x, y+th)
		}
	}, e.theme, e.ks.Mode().Name)

	// Show the selected completion candidate's detail (type signature and
	// doc comment) in the message bar while the menu is open.
	if e.hasCompletion() && !e.infobar.IsActive() && e.infobar.message == "" {
		if d := e.completionDetail(); d != "" {
			e.infobar.Message(d)
		}
	}

	// Show diagnostic for cursor line in infobar.
	v := e.ActiveView()
	if v != nil && !e.infobar.IsActive() && e.infobar.message == "" {
		line, _ := v.buf.LineColAt(v.buf.Cursor().Pos)
		if d, ok := v.buf.GetDiagnosticAt(line); ok {
			e.infobar.Message(fmt.Sprintf("[%s] %s", d.Type.String(), d.Text))
		} else if h, ok := v.buf.GetInlayHintAt(line); ok {
			e.infobar.Message(h.Text)
		}
	}

	// Macro recording indicator (vim: "recording @q").
	if e.ks.macroReg != 0 && !e.infobar.IsActive() && e.infobar.message == "" {
		e.infobar.Message(fmt.Sprintf("recording @%c", e.ks.macroReg))
	}

	// Multi-cursor indicator.
	if n := e.ks.Buf().NumCursors(); n > 1 && !e.infobar.IsActive() && e.infobar.message == "" {
		e.infobar.Message(fmt.Sprintf("%d cursors", n))
	}

	// Completion bar (above the infobar).
	if e.palette.active {
		e.drawPalette()
	} else if e.infobar.HasCompletions() {
		e.infobar.DrawCompletions(e.screen, e.h-2, e.w, e.theme)
	} else if e.hasCompletion() {
		e.drawEditorCompletions(e.h - 2)
	}

	// Info bar (always the bottom row).
	e.infobar.Draw(e.screen, e.h-1, e.w, e.theme)

	e.screen.Show()
}

// tabBarScroll returns the index of the first tab to draw so that tab cur
// is fully visible within width w (or at least starts at the left edge).
func tabBarScroll(widths []int, cur, w int) int {
	start := 0
	sum := 0
	for i := start; i <= cur; i++ {
		sum += widths[i]
	}
	for sum > w && start < cur {
		sum -= widths[start]
		start++
	}
	return start
}

// drawTabBar renders a tab bar showing all tab names. Scrolls so the
// active tab is always visible.
func (e *Editor) drawTabBar(y int) {
	style := e.theme.Style("tabbar")
	if !e.theme.HasStyle("tabbar") {
		style = e.theme.Style("statusline")
	}
	activeStyle := style.Add(AttrReverse)
	ts := style.TCellStyle()
	ats := activeStyle.TCellStyle()

	labels := make([]string, len(e.tabs))
	widths := make([]int, len(e.tabs))
	for i, t := range e.tabs {
		name := "[No Name]"
		if v := t.ActiveView(); v != nil && v.buf.Path != "" {
			name = filepath.Base(v.buf.Path)
		}
		labels[i] = fmt.Sprintf(" %s ", name)
		widths[i] = len([]rune(labels[i]))
	}

	x := 0
	for i := tabBarScroll(widths, e.curtab, e.w); i < len(e.tabs); i++ {
		s := ts
		if i == e.curtab {
			s = ats
		}
		for _, r := range labels[i] {
			if x >= e.w {
				break
			}
			e.screen.SetContent(x, y, r, nil, s)
			x++
		}
	}
	for x < e.w {
		e.screen.SetContent(x, y, ' ', nil, ts)
		x++
	}
}

// --- Key event conversion ---

func keyEventToString(ev *tcell.EventKey) string {
	mod := ev.Modifiers()

	if ev.Key() == tcell.KeyRune {
		r := ev.Rune()
		if mod&tcell.ModAlt != 0 {
			return fmt.Sprintf("<A-%c>", r)
		}
		return string(r)
	}

	if name, ok := specialKeyMap[ev.Key()]; ok {
		if mod&tcell.ModAlt != 0 {
			return fmt.Sprintf("<A-%s>", name)
		}
		return name
	}

	if ev.Key() >= tcell.KeyCtrlA && ev.Key() <= tcell.KeyCtrlZ {
		ch := 'a' + rune(ev.Key()-tcell.KeyCtrlA)
		return fmt.Sprintf("<C-%c>", ch)
	}

	return ""
}

var specialKeyMap = map[tcell.Key]string{
	tcell.KeyEscape:     KeyEscape,
	tcell.KeyEnter:      KeyEnter,
	tcell.KeyBackspace:  KeyBacksp,
	tcell.KeyBackspace2: KeyBacksp,
	tcell.KeyTab:        KeyTab,
	tcell.KeyDelete:     KeyDelete,
	tcell.KeyUp:         KeyUp,
	tcell.KeyDown:       KeyDown,
	tcell.KeyLeft:       KeyLeft,
	tcell.KeyRight:      KeyRight,
	tcell.KeyHome:       KeyHome,
	tcell.KeyEnd:        KeyEnd,
	tcell.KeyPgUp:       KeyPgUp,
	tcell.KeyPgDn:       KeyPgDn,
	tcell.KeyBacktab:    "<S-Tab>",
	tcell.KeyCtrlSpace:  "<C-space>",
}
