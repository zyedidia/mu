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

type Clipboard interface {
	SetClipboard(reg string, text []byte) error
	GetClipboard(reg string) ([]byte, error)
}

type Messager interface {
	Prompt(p string, done func(resp string, canceled bool) error)
	Message(msg string)
	Error(msg string)
	Clear()
}

type Evaluator interface {
	Eval(cmd string) error
}

type BufPane struct {
	*buffer.Buffer
	vis buffer.RuneVisualizer

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

	messager  Messager
	clipboard Clipboard
	cfg       Config
	eval      Evaluator
}

func NewBufPane(b *buffer.Buffer, msger Messager, clip Clipboard, cfg Config, eval Evaluator) *BufPane {
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
		cfg:           cfg,
		clipboard:     clip,
		messager:      msger,
		eval:          eval,
	}
	bp.InitOpts()
	return bp
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
