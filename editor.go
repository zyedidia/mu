package main

import (
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"
)

// Editor is the top-level editor state, managing the screen, views, and
// key dispatch.
type Editor struct {
	screen tcell.Screen
	config *Config
	theme  *Theme

	ks   *KeyState
	regs *RegisterSet

	views  []*View
	active int

	running bool
	w, h    int

	message string
	msgErr  bool
}

// NewEditor creates a new editor with the given screen, config, and theme.
func NewEditor(screen tcell.Screen, cfg *Config, th *Theme) *Editor {
	w, h := screen.Size()
	regs := NewRegisterSet()
	ks := NewKeyState(nil, regs)
	SetupBindings(ks)

	ed := &Editor{
		screen: screen,
		config: cfg,
		theme:  th,
		ks:     ks,
		regs:   regs,
		w:      w,
		h:      h,
	}

	// Editor-level bindings (quit, etc.)
	ed.registerEditorBindings()

	return ed
}

func (e *Editor) registerEditorBindings() {
	// ZQ: force quit
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.running = false
	}, "Z", "Q")

	// ZZ: save (TODO) and quit
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		e.running = false
	}, "Z", "Z")
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

	v.Resize(e.w, e.h-2) // leave room for status + info bars
	e.views = append(e.views, v)
	e.active = len(e.views) - 1
	e.ks.SetBuffer(buf)
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

// Message displays a message in the info bar.
func (e *Editor) Message(msg string) {
	e.message = msg
	e.msgErr = false
}

// Error displays an error in the info bar.
func (e *Editor) Error(msg string) {
	e.message = msg
	e.msgErr = true
}

// Run starts the main event loop.
func (e *Editor) Run() {
	e.running = true
	e.Display()

	for e.running {
		ev := e.screen.PollEvent()
		if ev == nil {
			break
		}

		// Clear transient messages on any keypress.
		e.message = ""
		e.msgErr = false

		switch ev := ev.(type) {
		case *tcell.EventKey:
			key := keyEventToString(ev)
			if key != "" {
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

	// Draw buffer contents.
	v.Display(func(x, y int, mainc rune, combc []rune, style Style) {
		e.screen.SetContent(x, y, mainc, combc, style.TCellStyle())
	}, func(x, y int, main bool) {
		if main {
			e.screen.ShowCursor(x, y)
		}
	}, e.theme)

	// Status bar.
	e.drawStatusBar(e.h - 2)

	// Info bar.
	e.drawInfoBar(e.h - 1)

	e.screen.Show()
}

// --- Status bar ---

func (e *Editor) drawStatusBar(y int) {
	if y < 0 {
		return
	}
	style := e.theme.Style("statusline")
	ts := style.TCellStyle()

	v := e.ActiveView()
	b := v.buf

	// Left: mode | filename [+]
	mode := e.ks.Mode().Name
	name := b.Path
	if name == "" {
		name = "[No Name]"
	}
	mod := ""
	if b.Modified() {
		mod = " [+]"
	}
	left := fmt.Sprintf(" %s | %s%s ", mode, name, mod)

	// Right: line:col
	line, col := b.LineColAt(b.Cursor().Pos)
	right := fmt.Sprintf(" %d:%d ", line+1, col+1)

	x := 0
	for _, r := range left {
		if x >= e.w {
			break
		}
		e.screen.SetContent(x, y, r, nil, ts)
		x++
	}
	for x < e.w-len(right) {
		e.screen.SetContent(x, y, ' ', nil, ts)
		x++
	}
	for _, r := range right {
		if x >= e.w {
			break
		}
		e.screen.SetContent(x, y, r, nil, ts)
		x++
	}
}

// --- Info bar ---

func (e *Editor) drawInfoBar(y int) {
	if y < 0 {
		return
	}
	style := e.theme.Default()
	if e.msgErr {
		style = e.theme.Style("error")
	}
	ts := style.TCellStyle()

	x := 0
	for _, r := range e.message {
		if x >= e.w {
			break
		}
		e.screen.SetContent(x, y, r, nil, ts)
		x++
	}
}

// --- Key event conversion ---

// keyEventToString converts a tcell key event to our key string format.
func keyEventToString(ev *tcell.EventKey) string {
	mod := ev.Modifiers()

	// Regular rune with no special modifiers (or just shift).
	if ev.Key() == tcell.KeyRune {
		r := ev.Rune()
		if mod&tcell.ModAlt != 0 {
			return fmt.Sprintf("<A-%c>", r)
		}
		return string(r)
	}

	// Named special keys.
	if name, ok := specialKeyMap[ev.Key()]; ok {
		if mod&tcell.ModAlt != 0 {
			return fmt.Sprintf("<A-%s>", name)
		}
		return name
	}

	// Ctrl combinations. Handle after special keys to avoid
	// Ctrl-I/Tab, Ctrl-M/Enter conflicts.
	if ev.Key() >= tcell.KeyCtrlA && ev.Key() <= tcell.KeyCtrlZ {
		ch := 'a' + rune(ev.Key()-tcell.KeyCtrlA)
		return fmt.Sprintf("<C-%c>", ch)
	}

	return ""
}

var specialKeyMap = map[tcell.Key]string{
	tcell.KeyEscape:    KeyEscape,
	tcell.KeyEnter:     KeyEnter,
	tcell.KeyBackspace: KeyBacksp,
	tcell.KeyBackspace2: KeyBacksp,
	tcell.KeyTab:       KeyTab,
	tcell.KeyDelete:    KeyDelete,
	tcell.KeyUp:        KeyUp,
	tcell.KeyDown:      KeyDown,
	tcell.KeyLeft:      KeyLeft,
	tcell.KeyRight:     KeyRight,
	tcell.KeyHome:      KeyHome,
	tcell.KeyEnd:       KeyEnd,
	tcell.KeyPgUp:      KeyPgUp,
	tcell.KeyPgDn:      KeyPgDn,
}
