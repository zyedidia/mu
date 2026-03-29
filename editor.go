package main

import (
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/zyedidia/gotcl"
)

// Editor is the top-level editor state, managing the screen, views, and
// key dispatch.
type Editor struct {
	screen tcell.Screen
	config *Config
	theme  *Theme

	ks     *KeyState
	regs   *RegisterSet
	interp *gotcl.Interp

	views  []*View
	active int

	infobar    *InfoBar
	search     SearchState
	lspManager *LspManager

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

	ks.halfPageSize = func() int {
		if v := ed.ActiveView(); v != nil {
			return v.height / 2
		}
		return 10
	}

	ed.initLsp()
	ed.initTCL()
	ed.registerEditorBindings()
	ed.registerSearchBindings()
	ed.registerLspBindings()

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

	// :: enter command mode
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.infobar.StartPrompt(":", func(input string) {
			e.RunCommand(input)
		})
	}, ":")
}

// OpenFile opens a file in a new view.
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
	e.addView(buf, path)
	return nil
}

// OpenEmpty opens an empty buffer.
func (e *Editor) OpenEmpty() {
	buf := NewEmptyBuffer()
	e.addView(buf, "")
}

func (e *Editor) addView(buf *Buffer, path string) {
	opts := e.config.BufferOptions(path, "")
	tabsize, _ := GetOptInt(opts, "tabsize")
	if tabsize == 0 {
		tabsize = 4
	}
	v := NewView(buf, tabsize)

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

	v.Resize(e.w, e.h-2)
	e.views = append(e.views, v)
	e.active = len(e.views) - 1
	e.ks.SetBuffer(buf)
	buf.updateModTime()
	buf.onReload = func(_ *Buffer) {
		// Post a redraw event so the screen updates.
		if e.screen != nil {
			e.screen.PostEvent(tcell.NewEventInterrupt(nil))
		}
	}
	buf.StartWatcher()

	// Detect filetype, initialize syntax highlighting and LSP.
	ft := DetectFiletype(e.config, path, buf.GetLine(0))
	if ft != "" {
		buf.InitSyntax(e.config, ft)
		e.initBufferLsp(buf, ft)
	}

	if path != "" && isReadonly(path) {
		buf.readonly = true
	}
}

// ActiveView returns the currently focused view.
func (e *Editor) ActiveView() *View {
	if len(e.views) == 0 {
		return nil
	}
	return e.views[e.active]
}

// Resize handles terminal resize events.
func (e *Editor) Resize(w, h int) {
	e.w, e.h = w, h
	for _, v := range e.views {
		v.Resize(w, h-2)
	}
	e.screen.Sync()
}

// checkExternalModified prompts the user if the file changed on disk.
// If the buffer is unmodified, it reloads silently.
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
		// Buffer is clean: auto-reload.
		if err := b.Reload(); err != nil {
			e.infobar.Error(fmt.Sprintf("reload: %v", err))
		} else {
			e.infobar.Message(fmt.Sprintf("\"%s\" reloaded", b.Path))
		}
		return
	}
	// Buffer has unsaved changes: ask.
	e.infobar.Prompt(fmt.Sprintf("\"%s\" changed on disk. Reload? (y/n)", b.Path), func(key string) {
		if key == "y" {
			if err := b.Reload(); err != nil {
				e.infobar.Error(fmt.Sprintf("reload: %v", err))
			} else {
				e.infobar.Message(fmt.Sprintf("\"%s\" reloaded", b.Path))
			}
		} else {
			// User declined: update mtime so we don't ask again.
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

// Run starts the main event loop. Shuts down LSP servers on exit.
func (e *Editor) Run() {
	defer e.lspManager.ShutdownAll()
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
			// Check for external modifications before processing keys.
			e.checkExternalModified()

			// If the infobar prompt is active, send keys there.
			if e.infobar.IsActive() {
				e.infobar.HandleKey(key)
			} else {
				// Clear transient messages on normal keypress.
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

	v := e.ActiveView()
	if v == nil {
		e.screen.Show()
		return
	}

	v.Relocate()

	// Check if syntax window needs re-centering.
	v.buf.SyntaxCheckWindow(v.buf.Cursor().Pos)

	// Draw buffer contents.
	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		e.screen.SetContent(x, y, mainc, combc, style.TCellStyle())
	}, func(x, y int, main bool) {
		if main && !e.infobar.IsActive() && !e.infobar.showCursor {
			e.screen.ShowCursor(x, y)
		}
	}, e.theme)

	// Show diagnostic for cursor line in infobar (if no other message).
	if !e.infobar.IsActive() && e.infobar.message == "" {
		line, _ := v.buf.LineColAt(v.buf.Cursor().Pos)
		if d, ok := v.buf.GetDiagnosticAt(line); ok {
			e.infobar.Message(fmt.Sprintf("[%s] %s", d.Type.String(), d.Text))
		}
	}

	// Status bar.
	e.drawStatusBar(e.h - 2)

	// Info bar.
	e.infobar.Draw(e.screen, e.h-1, e.w, e.theme)

	e.screen.Show()
}

// --- Key event conversion ---

// keyEventToString converts a tcell key event to our key string format.
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
}
