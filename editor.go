package ned

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/micro-editor/tcell/v2"
	"github.com/zyedidia/clipper"
	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/kbd"
	"github.com/zyedidia/ned/buffer"
	"github.com/zyedidia/ned/config"
	"github.com/zyedidia/ned/pane"
	"github.com/zyedidia/ned/pane/buf"
	"github.com/zyedidia/ned/pkg/input"
	"github.com/zyedidia/ned/pkg/output"
	"github.com/zyedidia/ned/pkg/tclutil"
	"github.com/zyedidia/ned/pkg/theme"
)

type TermClip interface {
	SetClipboard(reg string, text []byte) error
	GetClipboard(reg string) ([]byte, error)
}

type Editor struct {
	panes  []pane.Pane
	cur    int
	interp *tcl.Interp

	modes map[string]kbd.Config
	mode  *kbd.Config

	theme  *theme.Theme
	config *config.ConfigFS

	clipboard clipper.Clipboard
	termclip  TermClip

	w, h    int
	infobar *InfoBar

	redraw chan struct{}
}

func newEditor(clip TermClip) *Editor {
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
	redraw := make(chan struct{})
	e := &Editor{
		interp: interp,
		modes: map[string]kbd.Config{
			"micro": cfg.MustLoadBindings("micro"),
			"cmd":   cfg.MustLoadBindings("cmd"),
		},
		config:   cfg,
		theme:    th,
		redraw:   redraw,
		termclip: clip,
	}
	e.infobar = NewInfoBar(interp, buffer.NewEmptyBuffer(cfg, redraw), e)
	e.SetMode("micro")
	e.Register()
	return e
}

func NewEditor(clip TermClip) *Editor {
	e := newEditor(clip)
	e.MakePane()
	e.open(input.NewReader(strings.NewReader(""), "no name"), &output.Discard{})
	return e
}

func NewEditorFromPath(path string, clip TermClip) *Editor {
	e := newEditor(clip)
	e.MakePane()
	e.Open(path)
	return e
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

func (e *Editor) SetMode(m string) error {
	mode, ok := e.modes[m]
	if !ok {
		return fmt.Errorf("mode %s does not exist", m)
	}
	e.mode = &mode
	return nil
}

func (e *Editor) HandleEvent(ev tcell.Event) error {
	if e.mode == nil {
		return errors.New("no mode selected")
	}

	if rev, ok := ev.(*tcell.EventResize); ok {
		w, h := rev.Size()
		e.Resize(w, h)
		return nil
	}

	action, ok, more := e.mode.VM.Exec(ev)
	if !more {
		e.mode.VM.Reset()
	}
	if ok {
		return e.EvalWithVars(action.Cmd, action.Vars)
	}
	return nil
}

func (e *Editor) Register() {
	for _, c := range commands {
		tclutil.Register(e.interp, c.Name, c.Fn, e)
	}
}

func (e *Editor) Active() pane.Pane {
	return e.panes[e.cur]
}

func (e *Editor) valid() bool {
	return e.cur >= 0 && e.cur < len(e.panes) && e.panes[e.cur] != nil
}

func (e *Editor) MakePane() {
	e.panes = append(e.panes, nil)
	e.SetPane(len(e.panes) - 1)
}

func (e *Editor) open(in buffer.Input, out buffer.Output) error {
	b, err := buffer.NewBuffer(in, out, e.config, e.redraw, func(name string) (*buffer.BufferData, buffer.Cursor) {
		return nil, buffer.Cursor{}
	})
	if err != nil {
		return err
	}
	e.panes[e.cur] = buf.NewBufPane(b, e.infobar, e.termclip, e.config, e)
	e.panes[e.cur].Register(e.interp)
	return nil
}

func (e *Editor) Resize(w, h int) {
	e.w, e.h = w, h
	for _, p := range e.panes {
		p.Resize(w, h-1)
	}
	e.infobar.Resize(w, 1)
}

func (e *Editor) Display(draw func(x, y int, mainc rune, combc []rune, style theme.Style), cursor func(x, y int)) {
	e.panes[e.cur].Display(draw, cursor, e.theme)
	e.infobar.Display(func(x, y int, mainc rune, combc []rune, style theme.Style) {
		draw(x, e.h+y-1, mainc, combc, style)
	}, func(x, y int) {
		cursor(x, e.h+y-1)
	})
}

func (e *Editor) Clear(fill func(x rune, style theme.Style)) {
	fill(' ', e.theme.Default())
}

func (e *Editor) Redraw() chan struct{} {
	return e.redraw
}

func (e *Editor) SetPane(i int) {
	if e.infobar.active {
		e.infobar.cmd.Unregister(e.interp)
	} else if e.valid() {
		e.panes[e.cur].Unregister(e.interp)
	}
	if e.panes[i] != nil {
		e.panes[i].Register(e.interp)
	}
	e.cur = i
}

func (e *Editor) Error(msg string) {
	e.infobar.Error(msg)
}

func (e *Editor) Message(msg string) {
	e.infobar.Message(msg)
}
