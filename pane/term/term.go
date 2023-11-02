//go:build linux || darwin || dragonfly || openbsd_amd64 || freebsd

package term

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/james4k/terminal"
	"github.com/micro-editor/tcell/v2"
	"github.com/zyedidia/gotcl"
	"github.com/zyedidia/mu/pkg/theme"
)

type TermPane struct {
	width, height int

	pty    *os.File
	cmd    *exec.Cmd
	term   *terminal.VT
	state  terminal.State
	name   string
	exited bool
	lock   sync.Mutex
}

func NewTermPaneShell(redraw chan struct{}) (*TermPane, error) {
	return NewTermPane(redraw, os.Getenv("SHELL"), "-i")
}

func NewTermPane(redraw chan struct{}, name string, args ...string) (*TermPane, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=", "MICROTERM=1")

	t := &TermPane{
		cmd:  cmd,
		name: name,
	}

	var err error
	t.term, t.pty, err = terminal.Start(&t.state, cmd)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			err := t.term.Parse()
			if err != nil {
				break
			}
			redraw <- struct{}{}
		}
		fmt.Fprintf(t.term, "command exited: press any key to close")
		redraw <- struct{}{}
		t.closeterm()
	}()

	return t, nil
}

func (t *TermPane) Close() error {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.exited {
		return nil
	}

	// TODO: force close this terminal
	return nil
}

func (t *TermPane) Closed() bool {
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.exited
}

func (t *TermPane) closeterm() {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.term.File().Close()
	t.term.Close()
	t.exited = true
}

func (t *TermPane) ConsumeEvent(ev tcell.Event) error {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		t.Resize(ev.Size())
	case *tcell.EventKey:
		t.lock.Lock()
		defer t.lock.Unlock()
		var arg string
		if ev.Modifiers()&tcell.ModAlt == tcell.ModAlt ||
			ev.Modifiers()&tcell.ModMeta == tcell.ModMeta {
			arg += "\x1b"
		}
		if ev.Key() == tcell.KeyRune {
			arg += string(ev.Rune())
		} else if c, ok := codes[ev.Key()]; ok {
			arg += c
		}
		_, err := t.term.File().WriteString(arg)
		return err
	}
	return nil
}

func (t *TermPane) Display(draw func(x, y int, mainc rune, combc []rune, style theme.Style), showCursor func(x, y int, main bool), th *theme.Theme) {
	t.state.Lock()
	defer t.state.Unlock()

	for y := 0; y < t.height; y++ {
		for x := 0; x < t.width; x++ {
			c, f, b := t.state.Cell(x, y)

			style := theme.Style{}
			if f != terminal.DefaultFG {
				style.Fg = theme.NewPaletteColor(int(f))
			}
			if b != terminal.DefaultBG {
				style.Bg = theme.NewPaletteColor(int(b))
			}
			draw(x, y, c, nil, style)
		}
	}

	if t.state.CursorVisible() {
		x, y := t.state.Cursor()
		showCursor(x, y, true)
	}
}

func (t *TermPane) Resize(w, h int) {
	t.width, t.height = w, h
	t.lock.Lock()
	t.term.Resize(w, h)
	t.lock.Unlock()
}

func (t *TermPane) Name() string {
	return t.name
}

func (t *TermPane) SetMode(m string) {}
func (t *TermPane) Register(*gotcl.Interp) string {
	return "term"
}
func (t *TermPane) Unregister(*gotcl.Interp) {}
func (t *TermPane) Help(w io.Writer)         {}

func (t *TermPane) SetOpt(name string, val interface{}) error { return nil }
func (t *TermPane) GetOpt(name string) (interface{}, bool) {
	return nil, false
}

func (t *TermPane) Status() (string, string) {
	return fmt.Sprintf("%s:%d", t.name, t.cmd.Process.Pid), ""
}
