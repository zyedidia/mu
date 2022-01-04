package buf

import (
	"io"

	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/ned/buffer"
	"github.com/zyedidia/ned/pkg/tclutil"
	"github.com/zyedidia/ned/pkg/theme"
)

type Options interface {
	Set(name string, v interface{}) error
	Get(name string) interface{}
}

type BufPane struct {
	*buffer.Buffer
	vis buffer.RuneVisualizer

	cursors []Cursor
	cur     int

	stpos, stcol  int
	width, height int

	softwrap, wordwrap bool
	scrollmargin       int
	hscrollmargin      int

	theme *theme.Theme

	opts Options
}

func NewBufPane(b *buffer.Buffer, opts Options) *BufPane {
	return &BufPane{
		Buffer: b,
		vis: &buffer.Visualizer{
			TabSize: 4,
			CharMap: map[rune]string{
				'\t': "|",
				'\n': "\n",
				' ':  " ",
			},
		},
		scrollmargin:  3,
		hscrollmargin: 1,
		cursors:       []Cursor{SpawnCursorAt(0)},
		theme:         theme.Monokai,
		opts:          opts,
	}
}

func (bp *BufPane) Set(opt string, val interface{}) error {
	return bp.opts.Set(opt, val)
}
func (bp *BufPane) Get(opt string) interface{} {
	return bp.opts.Get(opt)
}

func (bp *BufPane) Register(interp *tcl.Interp) {
	for _, c := range commands {
		tclutil.Register(interp, c.Name, c.Fn, bp)
	}
}

func (bp *BufPane) Unregister(interp *tcl.Interp) {
	for _, c := range commands {
		tclutil.Unregister(interp, c.Name)
	}
}

func (bp *BufPane) Help(w io.Writer) {
	for _, cmd := range commands {
		w.Write([]byte(cmd.Doc))
		w.Write([]byte{'\n'})
	}
}

func (bp *BufPane) Cursor() *Cursor {
	return &bp.cursors[bp.cur]
}
