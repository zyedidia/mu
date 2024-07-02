package buf

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

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
	Prompt(typ, p string) (string, bool)
	IPrompt(typ, p string, cb func(string)) (string, bool)
	CharPrompt(p string) (string, bool)
	Message(msg string)
	DiagnosticMessage(msg string)
	ClearDiagnostic()
	Error(msg string)
	Clear()
}

type Editor interface {
	EvalRet(cmd string, vars []interface{}) (string, error)
	SuspendResume() (chan func(), chan struct{})
}

type BufPane struct {
	*buffer.Buffer

	topline, topcol int
	stcol           int
	width, height   int
	needsReloc      bool

	gutter int

	complete *CompleteBar

	search *regexp.Regexp

	mode string

	mouse mouseState

	// Vertical is a bit of a hack for cursor movements to indicate
	// that they are performing a purely vertical move. This signals
	// that the editor should not recalculate the cursor's visual X
	// which gives vertical cursor movement a more natural feel.
	vertical bool

	status *tcl.Interp

	messager Messager
	clip     Clipboard
	cfg      Config
	Editor   Editor
}

type mouseState struct {
	click   bool
	clicktm time.Time
	last    int
	drag    bool
	double  bool
	triple  bool
}

const mouseClickThreshold = 500 * time.Millisecond

func NewBufPane(b *buffer.Buffer, msger Messager, clip Clipboard, cfg Config, editor Editor) *BufPane {
	bp := &BufPane{
		Buffer:     b,
		needsReloc: true,
		complete:   &CompleteBar{},
		gutter:     1,
		mode:       b.StrOpt("mode"),
		cfg:        cfg,
		clip:       clip,
		messager:   msger,
		status:     tcl.NewInterp(),
		Editor:     editor,
	}
	for _, c := range statuscmds {
		tclutil.Register(bp.status, c.Name, c.Fn, bp, nil, nil)
	}
	return bp
}

func NewInfoBufPane(b *buffer.Buffer, msger Messager, clip Clipboard, cfg Config, editor Editor, linenums bool, gutter int) *BufPane {
	bp := NewBufPane(b, msger, clip, cfg, editor)
	bp.SetOpt("linenums", false)
	bp.SetOpt("cursorline", false)
	bp.gutter = gutter
	return bp
}

func (bp *BufPane) SetMode(m string) {
	bp.mode = m
}

func (bp *BufPane) Register(interp *tcl.Interp) string {
	pre := func() error {
		bp.CheckModified()
		return nil
	}
	reloc := func() {
		bp.RelocateToCur()
	}
	for _, c := range commands {
		post := reloc
		if !c.Relocate {
			post = nil
		}
		tclutil.Register(interp, c.Name, c.Fn, bp, pre, post)
	}
	return bp.mode
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
			err := bp.Save(nil)
			if err != nil {
				return err
			}
		}
	}
	bp.Buffer.Close()
	return nil
}

func (bp *BufPane) Closed() bool {
	return false
}

func (bp *BufPane) WordPrefix() string {
	start := bp.Cursor()
	consumed := 0
	for {
		r, _, sz := bp.DecodeGraphemeBefore(start.Pos - consumed)
		if !isNotSpace(r) || sz == 0 {
			break
		}
		consumed += sz
	}
	return string(bp.Slice(start.Pos-consumed, start.Pos))
}

func (bp *BufPane) WordStart(from int) int {
	consumed := 0
	for {
		r, _, sz := bp.DecodeGraphemeBefore(from - consumed)
		if !isNotSpace(r) || sz == 0 {
			break
		}
		consumed += sz
	}
	return from - consumed
}

func (bp *BufPane) RuneAtCursor() string {
	r, _ := bp.DecodeRuneAt(bp.Cursor().Pos)
	return string(r)
}
