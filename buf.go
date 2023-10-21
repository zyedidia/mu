package mu

import (
	"strings"

	"github.com/zyedidia/mu/buffer"
	"github.com/zyedidia/mu/pane/buf"
	"github.com/zyedidia/mu/pkg/input"
	"github.com/zyedidia/mu/pkg/output"
)

func (e *Editor) NewEmptyBufPane() *buf.BufPane {
	b, err := e.NewBufPane(input.NewReader(strings.NewReader(""), "no name"), &output.Discard{})
	if err != nil {
		panic(err)
	}
	return b
}

func (e *Editor) NewBufPane(in buffer.Input, out buffer.Output) (*buf.BufPane, error) {
	b, err := buffer.NewBuffer(in, out, e.config, e.Redraw, func(name string) (*buffer.BufferData, buffer.Cursor) {
		for _, b := range e.buffers {
			if b.FullName() == name {
				return b.BufferData, *b.Cursor()
			}
		}
		return nil, buffer.Cursor{}
	})
	if err != nil {
		return nil, err
	}
	e.buffers = append(e.buffers, b) // TODO: remove when closing a buffer
	return buf.NewBufPane(b, e.infobar, e, e.config, e), nil
}

func (e *Editor) NewBufPaneFromPath(path string) (*buf.BufPane, error) {
	in := &input.File{
		Path: path,
	}
	out := &output.File{
		Path: path,
	}
	return e.NewBufPane(in, out)
}
