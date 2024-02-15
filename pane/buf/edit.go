package buf

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zyedidia/mu/pkg/format"
)

func (bp *BufPane) InsertAt(pos int, val string) {
	bp.Buffer.Insert(pos, []byte(val))
	bp.RecalcVX(bp.Cursor())
}

func (bp *BufPane) InsertString(val string) {
	bp.InsertBytes([]byte(val))
}

func (bp *BufPane) InsertBytes(val []byte) {
	c := bp.Cursor()
	if c.HasSelection() {
		bp.Buffer.Remove(c.Sel[0], c.Sel[1])
		c.Deselect(0)
	}
	bp.Buffer.Insert(c.Pos, val)
	bp.RecalcVX(bp.Cursor())
}

func (bp *BufPane) Newline() {
	bp.InsertString("\n")
	if bp.autoindent {
		bp.Autoindent()
	}
}

func (bp *BufPane) Retab() {
	// TODO: do we want retab to only edit leading whitespace?
	var err error
	if bp.tabstospaces {
		err = bp.ReplaceAll("\t", strings.Repeat(" ", bp.tabsize))
	} else {
		err = bp.ReplaceAll(strings.Repeat(" ", bp.tabsize), "\t")
	}
	// err must be nil since we provide the regexp statically
	if err != nil {
		panic(err)
	}
}

func leadingws(b []byte) []byte {
	i := 0
	for i < len(b) {
		r, sz := utf8.DecodeRune(b[i:])
		if !unicode.IsSpace(r) {
			return b[0:i]
		}
		i += sz
	}
	return b[0:i]
}

func (bp *BufPane) Indent() {
	if bp.tabstospaces {
		bp.InsertString(strings.Repeat(" ", bp.tabsize))
	} else {
		bp.InsertString("\t")
	}
}

func (bp *BufPane) Autoindent() {
	line, _ := bp.LineColAt(bp.Cursor().Pos)
	if line > 0 {
		bline := bp.GetLine(line - 1)
		bp.Insert(bp.Offset(line, 0), leadingws(bline))
		switch {
		case bytes.HasSuffix(bline, []byte{'{'}),
			bytes.HasSuffix(bline, []byte{'('}),
			bytes.HasSuffix(bline, []byte{'['}),
			bytes.HasSuffix(bline, []byte{':'}):
			bp.Indent()
		}
	}
}

func (bp *BufPane) RemoveRange(from, to int) int {
	if from > to {
		from, to = to, from
	}
	if from < 0 || from >= to {
		return from
	}
	bp.Buffer.Remove(from, to)
	bp.RecalcVX(bp.Cursor())
	return from
}

func (bp *BufPane) RemoveTo(to int) int {
	from := bp.Cursor().Pos
	if from > to {
		from, to = to, from
	}
	return bp.RemoveRange(from, to)
}

func (bp *BufPane) RemoveSelection() {
	c := bp.Cursor()
	if c.HasSelection() {
		bp.RemoveRange(c.Sel[0], c.Sel[1])
		bp.Cursor().Deselect(0)
	}
}

func (bp *BufPane) Deselect() {
	bp.Cursor().HasSel = false
}

func (bp *BufPane) Undo() {
	bp.Buffer.Undo()
}

func (bp *BufPane) Redo() {
	bp.Buffer.Redo()
}

func (bp *BufPane) Paste() error {
	b, err := bp.clip.GetClipboard("clipboard")
	if err != nil {
		return err
	}
	bp.Buffer.Insert(bp.Cursor().Pos, b)
	return nil
}

func (bp *BufPane) Copy() error {
	if bp.Cursor().HasSelection() {
		bp.messager.Message("copied to clipboard")
		return bp.clip.SetClipboard("clipboard", bp.Cursor().Selection(bp.Buffer))
	}
	return nil
}

func (bp *BufPane) WordWrap() {
	if bp.Cursor().HasSelection() {
		sel := bp.Cursor().Selection(bp.Buffer)
		wrapped := format.WrapWords(sel, 80)
		bp.InsertBytes(wrapped)
		bp.InsertBytes([]byte{'\n'})
	}
}
