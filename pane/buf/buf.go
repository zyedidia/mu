package buf

import (
	"errors"
	"fmt"
	"io"
	"strings"

	tcl "github.com/zyedidia/gotcl"
	"github.com/zyedidia/mu/buffer"
	"github.com/zyedidia/mu/pkg/tclutil"
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
	Prompt(p string) (string, bool)
	CharPrompt(p string) (string, bool)
	Message(msg string)
	Error(msg string)
	Clear()
}

type Editor interface {
	EvalRet(cmd string, vars []interface{}) (string, error)
	SuspendResume() (chan func(), chan struct{})
}

type BufPane struct {
	*buffer.Buffer
	vis buffer.RuneVisualizer

	stpos, stcol  int
	width, height int

	softwrap, wordwrap bool
	scrollmargin       int
	hscrollmargin      int
	linenums           bool

	// Vertical is a bit of a hack for cursor movements to indicate
	// that they are performing a purely vertical move. This signals
	// that the editor should not recalculate the cursor's visual X
	// which gives vertical cursor movement a more natural feel.
	vertical bool

	status *tcl.Interp

	messager Messager
	clip     Clipboard
	cfg      Config
	editor   Editor
}

func NewBufPane(b *buffer.Buffer, msger Messager, clip Clipboard, cfg Config, editor Editor) *BufPane {
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
		linenums:      true,
		cfg:           cfg,
		clip:          clip,
		messager:      msger,
		editor:        editor,
		status:        tcl.NewInterp(),
	}
	for _, c := range statuscmds {
		tclutil.Register(bp.status, c.Name, c.Fn, bp)
	}
	bp.InitOpts()
	return bp
}

func NewBufPaneOpts(b *buffer.Buffer, msger Messager, clip Clipboard, cfg Config, editor Editor, linenums bool) *BufPane {
	bp := NewBufPane(b, msger, clip, cfg, editor)
	bp.linenums = linenums
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

func (bp *BufPane) Close() error {
	if bp.Buffer.Modified() {
		resp, canceled := bp.messager.CharPrompt(fmt.Sprintf("Save changes to %s before closing? (y,n,esc)", bp.Buffer.Name()))
		resp = strings.ToLower(resp)
		if canceled {
			return errors.New("close failed: unsaved changes")
		}
		if resp != "y" && resp != "n" {
			return errors.New("closed failed: invalid response")
		}
		if strings.ToLower(resp) == "y" {
			return bp.Save(nil)
		}
	}
	return nil
}

func (bp *BufPane) EvalRet(cmd string, vars []interface{}) (string, error) {
	obj, err := tclutil.EvalWithVars(bp.status, cmd, vars)
	if obj != nil && err == nil {
		return obj.AsString(), nil
	}
	return "", err
}
