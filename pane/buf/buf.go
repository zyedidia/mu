package buf

import (
	"fmt"
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

	// Vertical is a bit of a hack for cursor movements to indicate
	// that they are performing a purely vertical move. This signals
	// that the editor should not recalculate the cursor's visual X
	// which gives vertical cursor movement a more natural feel.
	vertical bool

	theme *theme.Theme

	opts Options
}

func NewBufPane(b *buffer.Buffer, opts Options) *BufPane {
	monokai, err := cfg.LoadTheme("monokai")
	if err != nil {
		panic(fmt.Errorf("theme load error: %w\n", err))
	}
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
		theme:         monokai,
		opts:          opts,
	}
}

func (bp *BufPane) Set(opt string, val interface{}) error {
	return bp.opts.Set(opt, val)
}
func (bp *BufPane) Get(opt string) interface{} {
	return bp.opts.Get(opt)
}

func (bp *BufPane) GetCursorAt(pos int) Cursor {
	for _, c := range bp.cursors {
		if c.Pos == pos {
			return c
		}
	}
	return SpawnCursorAt(pos)
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
