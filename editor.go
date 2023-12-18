package mu

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"sync"

	goerrors "github.com/go-errors/errors"
	"go.lsp.dev/protocol"

	"github.com/micro-editor/tcell/v2"
	"github.com/zyedidia/clipper"
	"github.com/zyedidia/generic/stack"
	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/kbd"
	"github.com/zyedidia/mu/buffer"
	"github.com/zyedidia/mu/config"
	"github.com/zyedidia/mu/lsp"
	"github.com/zyedidia/mu/pane"
	"github.com/zyedidia/mu/pkg/tclutil"
	"github.com/zyedidia/mu/pkg/theme"
	"github.com/zyedidia/mu/plugin"
)

type TermClip interface {
	SetClipboard(reg string, text []byte) error
	GetClipboard(reg string) ([]byte, error)
}

const errmsg = `Please report this issue at https://github.com/zyedidia/micro/issues.`

type PanicErr struct {
	trace string
}

func (e PanicErr) Error() string {
	return fmt.Sprintf("%s\n%v\n%s\n", "panic! (recoverable)", e.trace, errmsg)
}

type Editor struct {
	tabs        []*Tab
	curtab      int
	active      pane.Pane
	interp      *tcl.Interp
	plugins     *plugin.Manager
	displayLock sync.Mutex

	buffers []*buffer.Buffer

	modes    map[string]kbd.Config
	mode     stack.Stack[*kbd.Config]
	modeLock sync.Mutex

	theme  *theme.Theme
	config *config.ConfigFS

	clipboard clipper.Clipboard
	termclip  TermClip

	lsp *lsp.Manager

	log *buffer.Buffer

	w, h    int
	infobar *InfoBar

	Redraw  chan struct{}
	Errors  chan error
	Suspend chan func()
	Resume  chan struct{}
}

type FillFn func(r rune, style theme.Style)
type DrawFn func(x, y int, mainc rune, combc []rune, style theme.Style)
type CursorFn func(x, y int, main bool)

func loadBindings(cfg *config.ConfigFS, modes ...string) (map[string]kbd.Config, error) {
	modemap := make(map[string]kbd.Config)
	for _, m := range modes {
		b, err := cfg.LoadBindings(m)
		if err != nil {
			return nil, fmt.Errorf("while loading %s.kbd: %w", m, err)
		}
		modemap[m] = b
	}
	return modemap, nil
}

func newEditor(w, h int, clip TermClip) (*Editor, error) {
	cfg := config.NewConfigFS(config.DefaultConfigDir(), "")

	interp := tcl.NewInterp()
	_, err := interp.EvalString(tclcore)
	if err != nil {
		log.Println(err)
	}
	thname := cfg.GlobalStrOpt("theme")
	th, err := cfg.LoadTheme(thname)
	if err != nil {
		return nil, err
	}
	redraw := make(chan struct{}, 1)
	modes, err := loadBindings(cfg, "micro", "cmd", "charcmd", "complete", "term", "vim-normal", "vim-insert", "vim-visual")
	if err != nil {
		return nil, err
	}
	pm, err := plugin.NewManager(filepath.Join(cfg.ConfigDir()))
	if err != nil {
		return nil, err
	}
	e := &Editor{
		interp:   interp,
		plugins:  pm,
		modes:    modes,
		config:   cfg,
		theme:    th,
		termclip: clip,
		Redraw:   redraw,
		w:        w,
		h:        h,
		log:      buffer.NewNamedEmptyBuffer("log", cfg, nil, redraw),
		Errors:   make(chan error, 16),
		Suspend:  make(chan func(), 16),
		Resume:   make(chan struct{}, 1),
	}
	e.infobar = NewInfoBar(buffer.NewNamedEmptyBuffer("command", cfg, nil, redraw), e)

	langs, err := cfg.LoadLspLanguages()
	if err != nil {
		return nil, err
	}

	e.lsp = lsp.NewManager(func(msg protocol.ShowMessageParams) {
		e.displayLock.Lock()
		defer e.SendRedraw()
		defer e.displayLock.Unlock()

		e.infobar.Message("lsp: " + msg.Message)
	}, func(msg protocol.PublishDiagnosticsParams) {
		e.displayLock.Lock()
		defer e.SendRedraw()
		defer e.displayLock.Unlock()

		for _, b := range e.buffers {
			if b.FullName() == msg.URI.Filename() {
				b.ClearDiagnostics()
				for _, d := range msg.Diagnostics {
					b.AddLspDiagnostic(d.Range, d.Severity, d.Message)
				}
			}
		}
	}, langs)

	e.Register()
	e.initClipboard()
	e.plugins.Load()
	return e, nil
}

func NewEditor(w, h int, clip TermClip) (*Editor, error) {
	e, err := newEditor(w, h, clip)
	if err != nil {
		return nil, err
	}
	e.OpenTabPane(e.NewEmptyBufPane())
	return e, nil
}

func NewEditorFromPath(path string, w, h int, clip TermClip) (*Editor, error) {
	e, err := newEditor(w, h, clip)
	if err != nil {
		return nil, err
	}
	bp, err := e.NewBufPaneFromPath(path)
	if err != nil {
		return nil, err
	}
	e.OpenTabPane(bp)
	return e, nil
}

func init() {
	commands = append(commands, tclutil.Command{
		Name: "help",
		Fn:   (*Editor).Help,
		Doc:  "help: show help",
	})
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})
}

func (e *Editor) initClipboard() {
	switch e.config.GlobalStrOpt("clipboard") {
	case "external":
		c, err := clipper.GetClipboard(clipper.Clipboards...)
		if err == nil {
			e.clipboard = c
			return
		}
		e.config.SetGlobalOpt("clipboard", "internal")
		log.Printf("error loading external clipboard: %v\n", err)
	case "terminal":
		if e.termclip == nil {
			e.config.SetGlobalOpt("clipboard", "internal")
			log.Printf("terminal clipboard is unavailable")
		}
	}
	c := &clipper.Internal{}
	c.Init()
	e.clipboard = c
}

func (e *Editor) GetClipboard(reg string) ([]byte, error) {
	if e.config.GlobalStrOpt("clipboard") == "terminal" {
		return e.termclip.GetClipboard(reg)
	} else if e.clipboard != nil {
		return e.clipboard.ReadAll(reg)
	}
	return nil, errors.New("clipboard is unavailable")
}

func (e *Editor) SetClipboard(reg string, text []byte) error {
	if e.config.GlobalStrOpt("clipboard") == "terminal" {
		return e.termclip.SetClipboard(reg, text)
	} else if e.clipboard != nil {
		return e.clipboard.WriteAll(reg, text)
	}
	return errors.New("clipboard is unavailable")
}

func (e *Editor) MustSetMode(m string) {
	err := e.SetMode(m)
	if err != nil {
		panic(err)
	}
}

func (e *Editor) SetMode(m string) error {
	e.modeLock.Lock()
	defer e.modeLock.Unlock()
	mode, ok := e.modes[m]
	if !ok {
		return fmt.Errorf("mode %s does not exist", m)
	}
	e.mode.Pop()
	e.mode.Push(&mode)
	p := e.ActivePane()
	if e.infobar.active {
		e.infobar.cmd.SetMode(m)
	} else if p != nil {
		p.SetMode(m)
	}
	return nil
}

func (e *Editor) PushMode(m string) error {
	e.modeLock.Lock()
	defer e.modeLock.Unlock()
	mode, ok := e.modes[m]
	if !ok {
		return fmt.Errorf("mode %s does not exist", m)
	}
	e.mode.Push(&mode)
	p := e.ActivePane()
	if e.infobar.active {
		e.infobar.cmd.SetMode(m)
	} else if p != nil {
		p.SetMode(m)
	}
	return nil
}

func (e *Editor) PopMode() {
	e.modeLock.Lock()
	defer e.modeLock.Unlock()
	e.mode.Pop()
	m := e.mode.Peek()
	p := e.ActivePane()
	if e.infobar.active {
		e.infobar.cmd.SetMode(m.Core)
	} else if p != nil {
		p.SetMode(m.Core)
	}
}

func (e *Editor) GetMode() string {
	e.modeLock.Lock()
	defer e.modeLock.Unlock()
	return e.mode.Peek().Core
}

func (e *Editor) HasMode() bool {
	e.modeLock.Lock()
	defer e.modeLock.Unlock()
	return e.mode.Peek() != nil
}

type EventConsumer interface {
	ConsumeEvent(ev tcell.Event) error
}

func (e *Editor) HandleEvent(ev tcell.Event) {
	if !e.HasMode() {
		return
	}

	if rev, ok := ev.(*tcell.EventResize); ok {
		e.displayLock.Lock()
		defer e.SendRedraw()
		defer e.displayLock.Unlock()
		w, h := rev.Size()
		e.Resize(w, h)
		return
	}

	if mev, ok := ev.(*tcell.EventMouse); ok {
		if mev.Buttons() != tcell.ButtonNone && !e.infobar.active {
			e.displayLock.Lock()
			x, y := mev.Position()
			mev.SetPosition(e.ActiveTab().ActivateXY(e, x, y))
			e.displayLock.Unlock()
			e.SendRedraw()
		}
	}

process:
	action, ok, more := e.mode.Peek().VM.Exec(ev)
	if !more {
		e.mode.Peek().VM.Reset()
		if !ok && e.mode.Size() > 1 {
			e.PopMode()
			goto process
		}
	}
	if ok {
		e.displayLock.Lock()
		go func() {
			defer e.SendRedraw()
			defer e.displayLock.Unlock()

			defer func() {
				if err := recover(); err != nil {
					e.Errors <- PanicErr{goerrors.Wrap(err, 2).ErrorStack()}
				}
			}()

			err := e.Eval(action.Cmd, action.Vars)
			if len(e.tabs) == 0 {
				e.exit()
			} else if err != nil {
				e.Errors <- err
				e.Error(err.Error())
			}

			if e.infobar.active {
				e.infobar.cmd.PostEvent()
			}
		}()
	} else if ec, ok := e.ActivePane().(EventConsumer); ok {
		// active pane wants the raw event directly
		e.displayLock.Lock()
		defer e.SendRedraw()
		defer e.displayLock.Unlock()

		err := ec.ConsumeEvent(ev)
		if e.ActivePane().Closed() {
			e.Quit()
			if len(e.tabs) == 0 {
				e.exit()
			}
		} else if err != nil {
			e.Errors <- err
			e.Error(err.Error())
		}
	}
}

func (e *Editor) Register() {
	for _, c := range commands {
		tclutil.Register(e.interp, c.Name, c.Fn, e, c.Pre, nil)
	}
}

func (e *Editor) SendRedraw() {
	// only send if not already full
	select {
	case e.Redraw <- struct{}{}:
	default:
	}
}

func (e *Editor) ActiveTab() *Tab {
	return e.tabs[e.curtab]
}

func (e *Editor) ActivePane() pane.Pane {
	return e.active
}

func (e *Editor) Resize(w, h int) {
	e.w, e.h = w, h
	for _, t := range e.tabs {
		t.Resize(w, h-1) // -1 for infobar
	}
	e.infobar.Resize(w, 1)
}

func (e *Editor) Display(fill FillFn, draw DrawFn, cursor CursorFn) {
	e.displayLock.Lock()
	defer e.displayLock.Unlock()

	fill(' ', e.theme.Default())
	cursor(-1, -1, true)

	if e.curtab >= 0 && e.curtab < len(e.tabs) {
		e.tabs[e.curtab].Display(draw, cursor, e.theme)
		e.tabs[e.curtab].ActivePane().DisplayStatus(func(x, y int, mainc rune, combc []rune, style theme.Style) {
			draw(x, e.h+y-2, mainc, combc, style)
		}, e.w, e.theme)
	}
	e.infobar.Display(func(x, y int, mainc rune, combc []rune, style theme.Style) {
		draw(x, e.h+y-1, mainc, combc, style)
	}, func(x, y int, main bool) {
		cursor(x, e.h+y-1, main)
	})
	e.infobar.cmd.DisplayStatus(func(x, y int, mainc rune, combc []rune, style theme.Style) {
		draw(x, e.h+y-2, mainc, combc, style)
	}, e.w, e.theme)
}

func (e *Editor) ActivatePane(pane pane.Pane) {
	if e.infobar.active {
		e.infobar.cmd.Unregister(e.interp)
	} else if e.active != nil {
		e.active.Unregister(e.interp)
	}
	if pane != nil {
		mode := pane.Register(e.interp)
		err := e.SetMode(mode)
		if err != nil {
			log.Println("error setting mode:", err)
		}
	}
	e.active = pane

	e.Resize(e.w, e.h)
}

func (e *Editor) Error(msg string) {
	e.infobar.Error(msg)
}

func (e *Editor) Message(msg string) {
	e.infobar.Message(msg)
}

func (e *Editor) SuspendResume() (chan func(), chan struct{}) {
	return e.Suspend, e.Resume
}

var ErrQuit = errors.New("quit")

func (e *Editor) exit() {
	e.infobar.cmd.SerializeHistory(e.config)
	e.Errors <- ErrQuit
}
