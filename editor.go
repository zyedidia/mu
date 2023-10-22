package mu

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"

	goerrors "github.com/go-errors/errors"

	"github.com/micro-editor/tcell/v2"
	"github.com/zyedidia/clipper"
	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/kbd"
	"github.com/zyedidia/mu/buffer"
	"github.com/zyedidia/mu/config"
	"github.com/zyedidia/mu/pane"
	"github.com/zyedidia/mu/pkg/tclutil"
	"github.com/zyedidia/mu/pkg/theme"
)

type TermClip interface {
	SetClipboard(reg string, text []byte) error
	GetClipboard(reg string) ([]byte, error)
}

const errmsg = `Please report this issue online on GitHub.`

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
	displayLock sync.Mutex

	buffers []*buffer.Buffer

	modes    map[string]kbd.Config
	mode     *kbd.Config
	modeLock sync.Mutex

	theme  *theme.Theme
	config *config.ConfigFS

	clipboard clipper.Clipboard
	termclip  TermClip

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
type CursorFn func(x, y int)

func newEditor(w, h int, clip TermClip) *Editor {
	cfg := config.NewConfigFS(config.DefaultConfigDir(), "")

	interp := tcl.NewInterp()
	_, err := interp.EvalString(tclcore)
	if err != nil {
		log.Println(err)
	}
	thname := cfg.MustGlobalStrOpt("theme")
	th, err := cfg.LoadTheme(thname)
	if err != nil {
		log.Printf("error loading theme %s: %v\n", thname, err)
	}
	redraw := make(chan struct{}, 16)
	e := &Editor{
		interp: interp,
		modes: map[string]kbd.Config{
			"micro":   cfg.MustLoadBindings("micro"),
			"cmd":     cfg.MustLoadBindings("cmd"),
			"charcmd": cfg.MustLoadBindings("charcmd"),
		},
		config:   cfg,
		theme:    th,
		termclip: clip,
		Redraw:   redraw,
		w:        w,
		h:        h,
		log:      buffer.NewNamedEmptyBuffer("log", cfg, redraw),
		Errors:   make(chan error, 16),
		Suspend:  make(chan func(), 16),
		Resume:   make(chan struct{}),
	}
	e.infobar = NewInfoBar(buffer.NewNamedEmptyBuffer("command", cfg, redraw), e)
	e.MustSetMode("micro")
	e.Register()
	e.initClipboard()
	return e
}

func NewEditor(w, h int, clip TermClip) *Editor {
	e := newEditor(w, h, clip)
	e.OpenTabPane(e.NewEmptyBufPane())
	return e
}

func NewEditorFromPath(path string, w, h int, clip TermClip) (*Editor, error) {
	e := newEditor(w, h, clip)
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
	switch e.config.MustGlobalStrOpt("clipboard") {
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
	if e.config.MustGlobalStrOpt("clipboard") == "terminal" {
		return e.termclip.GetClipboard(reg)
	} else if e.clipboard != nil {
		return e.clipboard.ReadAll(reg)
	}
	return nil, errors.New("clipboard is unavailable")
}

func (e *Editor) SetClipboard(reg string, text []byte) error {
	if e.config.MustGlobalStrOpt("clipboard") == "terminal" {
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
	e.mode = &mode
	return nil
}

func (e *Editor) GetMode() string {
	e.modeLock.Lock()
	defer e.modeLock.Unlock()
	return e.mode.Core
}

func (e *Editor) HasMode() bool {
	e.modeLock.Lock()
	defer e.modeLock.Unlock()
	return e.mode != nil
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

	action, ok, more := e.mode.VM.Exec(ev)
	if !more {
		e.mode.VM.Reset()
	}
	if ok {
		go func() {
			e.displayLock.Lock()
			defer e.SendRedraw()
			defer e.displayLock.Unlock()

			defer func() {
				if err := recover(); err != nil {
					e.Errors <- PanicErr{goerrors.Wrap(err, 2).ErrorStack()}
				}
			}()

			err := e.Eval(action.Cmd, action.Vars)
			if err != nil {
				e.Errors <- err
				e.Error(err.Error())
			}
		}()
	}
}

func (e *Editor) Register() {
	for _, c := range commands {
		tclutil.Register(e.interp, c.Name, c.Fn, e)
	}
}

func (e *Editor) SendRedraw() {
	e.Redraw <- struct{}{}
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
	cursor(-1, -1)

	if e.curtab >= 0 && e.curtab < len(e.tabs) {
		e.tabs[e.curtab].Display(draw, cursor, e.theme)
	}
	e.infobar.Display(func(x, y int, mainc rune, combc []rune, style theme.Style) {
		draw(x, e.h+y-1, mainc, combc, style)
	}, func(x, y int) {
		cursor(x, e.h+y-1)
	})
}

func (e *Editor) ActivatePane(pane pane.Pane) {
	if e.infobar.active {
		e.infobar.cmd.Unregister(e.interp)
	} else if e.active != nil {
		e.active.Unregister(e.interp)
	}
	if pane != nil {
		pane.Register(e.interp)
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
