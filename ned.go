package ned

import (
	"sort"
	"strings"

	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/ned/buffer"
	"github.com/zyedidia/ned/pkg/input"
	"github.com/zyedidia/ned/pkg/output"
)

type command struct {
	name string
	fn   interface{}
	doc  string
}

type Editor struct {
	bufs   []*Buffer
	cur    int
	interp *tcl.Interp
	tclerr error
}

func newEditor() *Editor {
	interp := tcl.NewInterp()
	e := &Editor{
		interp: interp,
	}
	e.RegisterCommands()
	return e
}

func NewEditor() *Editor {
	e := newEditor()
	e.mkbuf()
	e.open(input.NewReader(strings.NewReader(""), "no name"), &output.Discard{})
	return e
}

func NewEditorFromPath(path string) *Editor {
	e := newEditor()
	e.mkbuf()
	e.Open(path)
	return e
}

func init() {
	commands = append(commands, command{
		name: "help",
		fn:   (*Editor).Help,
		doc:  "help: show help",
	})
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].name < commands[j].name
	})
}

func (e *Editor) RegisterCommands() {
	for _, c := range commands {
		e.RegisterCommand(c.name, c.fn)
	}
}

func (e *Editor) active() *Buffer {
	return e.bufs[e.cur]
}

func (e *Editor) valid() bool {
	return e.cur >= 0 && e.cur < len(e.bufs)
}

func (e *Editor) mkbuf() {
	e.bufs = append(e.bufs, nil)
	e.cur = len(e.bufs) - 1
}

func (e *Editor) open(in buffer.Input, out buffer.Output) error {
	b, err := buffer.NewBuffer(in, out, buffer.Options{})
	if err != nil {
		return err
	}
	e.bufs[e.cur] = NewBuffer(b)
	return nil
}
