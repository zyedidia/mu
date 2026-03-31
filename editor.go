package main

import (
	"fmt"
	"os"

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

	running bool
	w, h    int
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
		w:       w,
		h:       h,
	}

	ks.activeView = func() *View {
		return ed.ActiveView()
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
		case ModeVisual, ModeVisualLine:
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

	// ZZ: save and quit
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if v := e.ActiveView(); v != nil && v.buf.Path != "" {
			if err := v.buf.Save(); err != nil {
				e.infobar.Error(err.Error())
				return
			}
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

// NewTabWithView creates a new tab containing the given view.
func (e *Editor) NewTabWithView(v *View) {
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
	e.tabs = append(e.tabs[:e.curtab], e.tabs[e.curtab+1:]...)
	if e.curtab >= len(e.tabs) {
		e.curtab = len(e.tabs) - 1
	}
	e.resizeTabs()
	e.syncActiveBuffer()
}

// --- Split management ---

// VSplit creates a vertical split. If args is non-nil, opens that file;
// otherwise duplicates the current buffer.
func (e *Editor) VSplit(args []string) {
	v := e.makeNewView(args)
	if v == nil {
		return
	}
	e.ActiveTab().VSplit(v)
	e.syncActiveBuffer()
}

// HSplit creates a horizontal split.
func (e *Editor) HSplit(args []string) {
	v := e.makeNewView(args)
	if v == nil {
		return
	}
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
	if !t.Unsplit() {
		e.CloseTab()
		return
	}
	e.syncActiveBuffer()
}

// makeNewView creates a view for a split. Opens the file from args[0] or
// creates a view of the current buffer.
func (e *Editor) makeNewView(args []string) *View {
	if len(args) > 0 && args[0] != "" {
		data, err := os.ReadFile(args[0])
		if err != nil {
			if os.IsNotExist(err) {
				data = []byte{}
			} else {
				e.infobar.Error(fmt.Sprintf("open: %v", err))
				return nil
			}
		}
		buf, err := NewBuffer(data, args[0])
		if err != nil {
			e.infobar.Error(fmt.Sprintf("open: %v", err))
			return nil
		}
		return e.configureView(buf, args[0])
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

// OpenFile opens a file in a new view. If no tabs exist, creates one.
// Otherwise adds to the current tab.
func (e *Editor) OpenFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			data = []byte{}
		} else {
			return err
		}
	}
	buf, err := NewBuffer(data, path)
	if err != nil {
		return err
	}
	v := e.configureView(buf, path)

	if len(e.tabs) == 0 {
		e.NewTabWithView(v)
	} else {
		// Replace the active pane's view with the new one.
		t := e.ActiveTab()
		t.panes[t.cur] = v
		t.Resize(t.w, t.h)
		e.syncActiveBuffer()
	}
	return nil
}

// OpenEmpty opens an empty buffer.
func (e *Editor) OpenEmpty() {
	buf := NewEmptyBuffer()
	v := e.configureView(buf, "")
	if len(e.tabs) == 0 {
		e.NewTabWithView(v)
	} else {
		t := e.ActiveTab()
		t.panes[t.cur] = v
		t.Resize(t.w, t.h)
		e.syncActiveBuffer()
	}
}

// newViewWithOptions creates a View for a buffer and applies display options
// from the config. Does not initialize buffer-level state (syntax, LSP, etc.).
func (e *Editor) newViewWithOptions(buf *Buffer) *View {
	opts := e.config.BufferOptions(buf.Path, "")
	tabsize, _ := GetOptInt(opts, "tabsize")
	if tabsize == 0 {
		tabsize = 4
	}
	v := NewView(buf, tabsize)
	v.Opts = opts

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

	return v
}

// configureView creates a View and fully initializes the buffer (syntax,
// LSP, file watcher, readonly detection). Use for newly opened files.
func (e *Editor) configureView(buf *Buffer, path string) *View {
	v := e.newViewWithOptions(buf)

	buf.updateModTime()
	buf.onReload = func(_ *Buffer) {
		if e.screen != nil {
			e.screen.PostEvent(tcell.NewEventInterrupt(nil))
		}
	}
	buf.onHighlight = func() {
		if e.screen != nil {
			e.screen.PostEvent(tcell.NewEventInterrupt(nil))
		}
	}
	buf.StartWatcher()

	ft := DetectFiletype(e.config, path, buf.GetLine(0))
	buf.Filetype = ft
	if ft != "" {
		buf.InitSyntax(e.config, ft)
		e.initBufferLsp(buf, ft)
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
			buf.LoadCursorPos()
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

// checkExternalModified prompts the user if the file changed on disk.
func (e *Editor) checkExternalModified() {
	v := e.ActiveView()
	if v == nil || e.infobar.IsActive() {
		return
	}
	b := v.buf
	if !b.ExternallyModified() {
		return
	}
	if !b.Modified() {
		if err := b.Reload(); err != nil {
			e.infobar.Error(fmt.Sprintf("reload: %v", err))
		} else {
			e.infobar.Message(fmt.Sprintf("\"%s\" reloaded", b.Path))
		}
		return
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
				b.SaveCursorPos()
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

		switch ev := ev.(type) {
		case *tcell.EventKey:
			key := keyEventToString(ev)
			if key == "" {
				continue
			}
			e.checkExternalModified()

			if e.infobar.IsActive() {
				e.infobar.HandleKey(key)
			} else if e.hasCompletion() {
				e.handleCompletionKey(key)
			} else {
				e.infobar.Clear()
				e.ks.HandleKey(key)
			}
		case *tcell.EventResize:
			w, h := ev.Size()
			e.Resize(w, h)
		}

		e.Display()
	}
}

// Display renders the entire screen.
func (e *Editor) Display() {
	defStyle := e.theme.Default().TCellStyle()
	e.screen.Fill(' ', defStyle)

	// Recalculate Vx for all cursors unless the last action was a
	// purely vertical motion (j/k/Ctrl-D/Ctrl-U).
	if !e.ks.vertical {
		b := e.ks.Buf()
		for i := 0; i < b.NumCursors(); i++ {
			b.cursors[i].Vx = b.VisualCol(b.cursors[i].Pos)
		}
	}
	e.ks.vertical = false

	t := e.ActiveTab()
	if t == nil {
		e.screen.Show()
		return
	}

	// Draw tab bar at the top if there are multiple tabs.
	th := e.tabBarHeight()
	if th > 0 {
		e.drawTabBar(0)
	}

	// Draw all panes in the tab (offset by tab bar height).
	t.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		e.screen.SetContent(x, y+th, mainc, combc, style.TCellStyle())
	}, func(x, y int, main bool) {
		if main && !e.infobar.IsActive() && !e.infobar.showCursor {
			e.screen.ShowCursor(x, y+th)
		}
	}, e.theme, e.ks.Mode().Name)

	// Show diagnostic for cursor line in infobar.
	v := e.ActiveView()
	if v != nil && !e.infobar.IsActive() && e.infobar.message == "" {
		line, _ := v.buf.LineColAt(v.buf.Cursor().Pos)
		if d, ok := v.buf.GetDiagnosticAt(line); ok {
			e.infobar.Message(fmt.Sprintf("[%s] %s", d.Type.String(), d.Text))
		}
	}

	// Completion bar (above the infobar).
	if e.infobar.HasCompletions() {
		e.infobar.DrawCompletions(e.screen, e.h-2, e.w, e.theme)
	} else if e.hasCompletion() {
		e.drawEditorCompletions(e.h - 2)
	}

	// Info bar (always the bottom row).
	e.infobar.Draw(e.screen, e.h-1, e.w, e.theme)

	e.screen.Show()
}

// drawTabBar renders a tab bar showing all tab names.
func (e *Editor) drawTabBar(y int) {
	style := e.theme.Style("tabbar")
	if !e.theme.HasStyle("tabbar") {
		style = e.theme.Style("statusline")
	}
	activeStyle := style.Add(AttrReverse)
	ts := style.TCellStyle()
	ats := activeStyle.TCellStyle()

	x := 0
	for i, t := range e.tabs {
		name := "[No Name]"
		if v := t.ActiveView(); v != nil && v.buf.Path != "" {
			name = v.buf.Path
		}
		label := fmt.Sprintf(" %s ", name)

		s := ts
		if i == e.curtab {
			s = ats
		}
		for _, r := range label {
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
