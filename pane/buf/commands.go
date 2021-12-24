package buf

import (
	"fmt"
	"log"
	"regexp"

	"github.com/zyedidia/ned/pkg/output"
	"github.com/zyedidia/ned/pkg/tclutil"
)

func (bp *BufPane) Save() error {
	return bp.Buffer.Save()
}

func (bp *BufPane) SaveAs(path string) error {
	bp.SetOutput(&output.File{
		Path: path,
	})
	return bp.Save()
}

// --- Editing ---

func (bp *BufPane) InsertAt(pos int, val string) {
	bp.Buffer.Insert(pos, []byte(val))
}

func (bp *BufPane) Remove(from, to int) {
	if from < 0 || from >= to {
		return
	}
	log.Println(from, to)
	bp.Buffer.Remove(from, to)
}

// --- Reading ---

func (bp *BufPane) Read(from, to int) string {
	b := make([]byte, to-from)
	n, _ := bp.ReadAt(b, int64(from))
	return string(b[:n])
}

func (bp *BufPane) ReadLine(l int) string {
	return string(bp.GetLine(l))
}

func (bp *BufPane) ReadAll() string {
	return string(bp.Bytes())
}

// --- Searching ---

func (bp *BufPane) FindDown(off int, regex string) ([]int, error) {
	r, err := regexp.Compile(regex)
	if err != nil {
		return nil, err
	}
	match := bp.Buffer.FindDown(r, off)
	if len(match) < 1 {
		return nil, fmt.Errorf("no match found")
	}
	return match, nil
}

func (bp *BufPane) FindUp(off int, regex string) ([]int, error) {
	r, err := regexp.Compile(regex)
	if err != nil {
		return nil, err
	}
	match := bp.Buffer.FindUp(r, off)
	if len(match) < 1 {
		return nil, fmt.Errorf("no match found")
	}
	return match, nil
}

// --- Cursors ---

func (bp *BufPane) CursorUp(from int) int {
	b := bp
	c := SpawnCursorAt(from).Up(b.Buffer)
	return c.Pos
}

func (bp *BufPane) CursorDown(from int) int {
	b := bp
	c := SpawnCursorAt(from).Down(b.Buffer)
	return c.Pos
}

func (bp *BufPane) CursorLeft(from int) int {
	b := bp
	c := SpawnCursorAt(from).Left(b.Buffer)
	return c.Pos
}

func (bp *BufPane) CursorRight(from int) int {
	b := bp
	c := SpawnCursorAt(from).Right(b.Buffer)
	return c.Pos
}

// --- Locations ---

func (bp *BufPane) LineCol(pos int) []int {
	line, col := bp.LineColAt(pos)
	return []int{line, col}
}

func (bp *BufPane) Offset(line, col int) int {
	return bp.OffsetAt(line, col)
}

func (bp *BufPane) Size() int {
	return int(bp.Buffer.Size())
}

// --- Options ---

func (bp *BufPane) Filetype() string {
	return bp.Buffer.Filetype()
}

func (bp *BufPane) Name() string {
	return bp.Buffer.Name()
}

var commands = []tclutil.Command{
	{
		"save",
		(*BufPane).Save,
		"save: save the current buffer",
	},
	{
		"save-as",
		(*BufPane).SaveAs,
		"save-as: change the current buffer's output and save",
	},
	{
		"insert-at",
		(*BufPane).InsertAt,
		"insert-at <pos> <text>: insert <text> at <pos>",
	},
	{
		"remove",
		(*BufPane).Remove,
		"remove <from> <to>: remove the bytes in the range [<from>:<to>)",
	},
	{
		"read",
		(*BufPane).Read,
		"read <from> <to>: return the buffer contents in the range [<from>:<to>)",
	},
	{
		"read-line",
		(*BufPane).ReadLine,
		"read-line <line>: return the contents of <line>",
	},
	{
		"read-all",
		(*BufPane).ReadAll,
		"read-all: return the contents of the current buffer",
	},
	{
		"find-down",
		(*BufPane).FindDown,
		"find-down <pos> <regex>: search down from <pos> for <regex> and return match as a pair of positions",
	},
	{
		"find-up",
		(*BufPane).FindUp,
		"find-up <pos> <regex>: search up from <pos> for <regex> and return match as a pair of positions",
	},
	{
		"filetype",
		(*BufPane).Filetype,
		"filetype: return the filetype of the current buffer",
	},
	{
		"name",
		(*BufPane).Name,
		"name: return the name of the current buffer",
	},
	{
		"line-col",
		(*BufPane).LineCol,
		"line-col <pos>: return the line/col pair corresponding to a byte offset",
	},
	{
		"offset",
		(*BufPane).Offset,
		"offset <line> <col>: return the offset corresponding to a line/col pair",
	},
	{
		"size",
		(*BufPane).Size,
		"size: return the number of bytes in the buffer",
	},
	{
		"cursor-left",
		(*BufPane).CursorLeft,
		"cursor-left <pos>: returns the resulting position from moving a cursor at <pos> left one character",
	},
	{
		"cursor-right",
		(*BufPane).CursorRight,
		"cursor-right <pos>: returns the resulting position from moving a cursor at <pos> right one character",
	},
	{
		"cursor-up",
		(*BufPane).CursorUp,
		"cursor-up <pos>: returns the resulting position from moving a cursor at <pos> up one line",
	},
	{
		"cursor-down",
		(*BufPane).CursorDown,
		"cursor-down <pos>: returns the resulting position from moving a cursor at <pos> down one line",
	},
}
