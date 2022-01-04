package ned

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/micro-editor/tcell/v2"
	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/kbd"
	"github.com/zyedidia/ned/buffer"
	"github.com/zyedidia/ned/pane"
	"github.com/zyedidia/ned/pane/buf"
	"github.com/zyedidia/ned/pkg/input"
	"github.com/zyedidia/ned/pkg/output"
	"github.com/zyedidia/ned/pkg/tclutil"
	"github.com/zyedidia/ned/pkg/theme"
)

type Store[K, V any] interface {
	Get(k K) (V, bool)
	Put(k K, v V)
}

type Editor struct {
	panes  []pane.Pane
	cur    int
	interp *tcl.Interp

	modes Store[string, kbd.Config]
	mode  *kbd.Config
}

func newEditor() *Editor {
	interp := tcl.NewInterp()
	_, err := interp.EvalString(tclcore)
	if err != nil {
		log.Println(err)
	}
	e := &Editor{
		interp: interp,
	}
	e.Register()
	return e
}

func NewEditor() *Editor {
	e := newEditor()
	e.MakePane()
	e.open(input.NewReader(strings.NewReader(""), "no name"), &output.Discard{})
	return e
}

func NewEditorFromPath(path string) *Editor {
	e := newEditor()
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

func (e *Editor) SetModes(modes Store[string, kbd.Config]) {
	e.modes = modes
}

func (e *Editor) SetMode(m string) error {
	mode, ok := e.modes.Get(m)
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
	return e.cur >= 0 && e.cur < len(e.panes)
}

func (e *Editor) MakePane() {
	if e.valid() {
		e.panes[e.cur].Unregister(e.interp)
	}
	e.panes = append(e.panes, nil)
	e.cur = len(e.panes) - 1
}

func (e *Editor) open(in buffer.Input, out buffer.Output) error {
	b, err := buffer.NewBuffer(in, out, buffer.Options{})
	if err != nil {
		return err
	}
	e.panes[e.cur] = buf.NewBufPane(b, &Options{
		ed: e,
		opts: defaults,
	})
	e.panes[e.cur].Register(e.interp)
	return nil
}

func (e *Editor) Resize(w, h int) {
	e.panes[e.cur].Resize(w, h)
}

func (e *Editor) Display(draw func(x, y int, mainc rune, combc []rune, style theme.Style), cursor func(x, y int)) {
	e.panes[e.cur].Display(draw, cursor)
}
