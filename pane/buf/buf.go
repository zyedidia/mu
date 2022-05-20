package buf

import (
	"io"

	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/ned/buffer"
	"github.com/zyedidia/ned/pkg/tclutil"
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

	cfg Config
}

func NewBufPane(b *buffer.Buffer, cfg Config) *BufPane {
	bp := &BufPane{
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
		cfg:           cfg,
	}
	bp.InitOpts()
	return bp
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
