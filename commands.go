package ned

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/zyedidia/ned/pkg/input"
	"github.com/zyedidia/ned/pkg/output"
)

func (e *Editor) Help() {
	for _, cmd := range commands {
		fmt.Println(cmd.doc)
	}
}

func (e *Editor) Open(path string) error {
	in := &input.File{
		Path: path,
	}
	out := &output.File{
		Path: path,
	}
	return e.open(in, out)
}

func (e *Editor) Save() error {
	return e.active().Save()
}

func (e *Editor) InsertAt(pos int, val string) {
	e.active().Insert(pos, []byte(val))
}

func (e *Editor) Remove(from, to int) {
	e.active().Remove(from, to)
}

func (e *Editor) Read(from, to int) string {
	b := make([]byte, to-from)
	n, _ := e.active().ReadAt(b, int64(from))
	return string(b[:n])
}

func (e *Editor) ReadLine(l int) string {
	return string(e.active().GetLine(l))
}

func (e *Editor) ReadAll() string {
	return string(e.active().Bytes())
}

func (e *Editor) FindDown(off int, regex string) ([]int, error) {
	r, err := regexp.Compile(regex)
	if err != nil {
		return nil, err
	}
	match := e.active().FindDown(r, off)
	if len(match) < 1 {
		return nil, fmt.Errorf("no match found")
	}
	return match, nil
}

func (e *Editor) FindUp(off int, regex string) ([]int, error) {
	r, err := regexp.Compile(regex)
	if err != nil {
		return nil, err
	}
	match := e.active().FindUp(r, off)
	if len(match) < 1 {
		return nil, fmt.Errorf("no match found")
	}
	return match, nil
}

func (e *Editor) Filetype() string {
	return e.active().Filetype()
}

func (e *Editor) Name() string {
	return e.active().Name()
}

func (e *Editor) Quit() {
	i := e.cur
	copy(e.bufs[i:], e.bufs[i+1:])
	e.bufs[len(e.bufs)-1] = nil
	e.bufs = e.bufs[:len(e.bufs)-1]
	if !e.valid() {
		e.cur = len(e.bufs) - 1
	}
}

func (e *Editor) QuitAll() {
	e.bufs = nil
	e.cur = 0
}

func (e *Editor) ShowBuffers() {
	for i, b := range e.bufs {
		if e.cur == i {
			fmt.Printf("[%d: %v]\n", i, b.Name())
		} else {
			fmt.Printf("%d: %v\n", i, b.Name())
		}
	}
}

func (e *Editor) SetBuffer(name string) error {
	for i, b := range e.bufs {
		if b.Name() == name {
			e.cur = i
			return nil
		}
	}
	return fmt.Errorf("buffer '%s' not found", name)
}

func (e *Editor) SetBufferIdx(idx int) error {
	if idx < 0 || idx >= len(e.bufs) {
		return fmt.Errorf("invalid buffer index: %d", idx)
	}
	e.cur = idx
	return nil
}

func (e *Editor) NewBuffer() {
	e.mkbuf()
	e.open(input.NewReader(strings.NewReader(""), "no name"), &output.Discard{})
}

func (e *Editor) LineCol(pos int) []int {
	line, col := e.active().LineColAt(pos)
	return []int{line, col}
}

func (e *Editor) Offset(line, col int) int {
	return e.active().OffsetAt(line, col)
}

var commands = []command{
	{
		"open",
		(*Editor).Open,
		"open <file>: open <file> in the current buffer",
	},
	{
		"save",
		(*Editor).Save,
		"save: save the current buffer",
	},
	{
		"insert-at",
		(*Editor).InsertAt,
		"insert-at <pos> <text>: insert <text> at <pos>",
	},
	{
		"read",
		(*Editor).Read,
		"read <from> <to>: return the buffer contents in the range [<from>:<to>)",
	},
	{
		"read-line",
		(*Editor).ReadLine,
		"read-line <line>: return the contents of <line>",
	},
	{
		"read-all",
		(*Editor).ReadAll,
		"read-all: return the contents of the current buffer",
	},
	{
		"find-down",
		(*Editor).FindDown,
		"find-down <pos> <regex>: search down from <pos> for <regex> and return match as a pair of positions",
	},
	{
		"find-up",
		(*Editor).FindUp,
		"find-up <pos> <regex>: search up from <pos> for <regex> and return match as a pair of positions",
	},
	{
		"filetype",
		(*Editor).Filetype,
		"filetype: return the filetype of the current buffer",
	},
	{
		"name",
		(*Editor).Name,
		"name: return the name of the current buffer",
	},
	{
		"line-col",
		(*Editor).LineCol,
		"line-col <pos>: return the line/col pair corresponding to a byte offset",
	},
	{
		"offset",
		(*Editor).Offset,
		"offset <line> <col>: return the offset corresponding to a line/col pair",
	},
	{
		"quit",
		(*Editor).Quit,
		"quit: close the current buffer",
	},
	{
		"quit-all",
		(*Editor).QuitAll,
		"quit-all: close all buffers",
	},
	{
		"show-buffers",
		(*Editor).ShowBuffers,
		"show-buffers: display all open buffers",
	},
	{
		"set-buffer-idx",
		(*Editor).SetBufferIdx,
		"set-buffer-idx <idx>: set the currently active buffer to the <idx>-th buffer",
	},
	{
		"set-buffer",
		(*Editor).SetBuffer,
		"set-buffer <name>: set the currently active buffer to the buffer with <name>",
	},
	{
		"new-buffer",
		(*Editor).NewBuffer,
		"new-buffer: open a new empty buffer",
	},
}
