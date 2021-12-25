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

// --- Movement ---

func (bp *BufPane) Up(from int) int {
	b := bp
	c := SpawnCursorAt(from).Up(b.Buffer)
	return c.Pos
}

func (bp *BufPane) Down(from int) int {
	b := bp
	c := SpawnCursorAt(from).Down(b.Buffer)
	return c.Pos
}

func (bp *BufPane) Left(from int) int {
	b := bp
	c := SpawnCursorAt(from).Left(b.Buffer)
	return c.Pos
}

func (bp *BufPane) Right(from int) int {
	b := bp
	c := SpawnCursorAt(from).Right(b.Buffer)
	return c.Pos
}

// --- Cursors ---

func (bp *BufPane) MoveTo(pos int) {
	c := bp.Cursor()
	*c = c.MoveTo(pos)
}

func (bp *BufPane) SelectTo(pos int) {
	c := bp.Cursor()
	*c = c.SelectTo(pos)
}

func (bp *BufPane) SwitchCursor(idx int) error {
	if idx >= 0 && idx < len(bp.cursors) {
		bp.cur = idx
		return nil
	}
	return fmt.Errorf("invalid cursor: %d", idx)
}

func (bp *BufPane) SpawnCursor(at int) {
	bp.cursors = append(bp.cursors, SpawnCursorAt(at))
}

func (bp *BufPane) RemoveCursor(idx int) error {
	if idx < 0 || idx >= len(bp.cursors) {
		return fmt.Errorf("invalid cursor: %d", idx)
	}
	copy(bp.cursors[idx:], bp.cursors[idx+1:])
	bp.cursors = bp.cursors[:len(bp.cursors)-1]
	return nil
}

func (bp *BufPane) NumCursors() int {
	return len(bp.cursors)
}

func (bp *BufPane) CursorPos() int {
	return bp.Cursor().Pos
}

func (bp *BufPane) CursorRange() []int {
	sel := bp.Cursor().Sel
	return []int{sel[0], sel[1]}
}

func (bp *BufPane) CursorHasSelection() bool {
	return bp.Cursor().HasSelection()
}

func (bp *BufPane) CursorSelection() string {
	return string(bp.Cursor().Selection(bp.Buffer))
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
		"left",
		(*BufPane).Left,
		"left <pos>: returns the resulting position from moving a cursor at <pos> left one character",
	},
	{
		"right",
		(*BufPane).Right,
		"right <pos>: returns the resulting position from moving a cursor at <pos> right one character",
	},
	{
		"up",
		(*BufPane).Up,
		"up <pos>: returns the resulting position from moving a cursor at <pos> up one line",
	},
	{
		"down",
		(*BufPane).Down,
		"down <pos>: returns the resulting position from moving a cursor at <pos> down one line",
	},
	{
		"move-to",
		(*BufPane).MoveTo,
		"move-to <pos>: move the current cursor to <pos>",
	},
	{
		"select-to",
		(*BufPane).SelectTo,
		"select-to <pos>: move the current cursor to <pos> and make a selection",
	},
	{
		"switch-cursor",
		(*BufPane).SwitchCursor,
		"switch-cursor <idx>: change the active cursor to the <idx>-th cursors",
	},
	{
		"spawn-cursor",
		(*BufPane).SpawnCursor,
		"spawn-cursor <pos>: spawn a new cursor at <pos>",
	},
	{
		"remove-cursor",
		(*BufPane).RemoveCursor,
		"remove-cursor <idx>: remove the <idx>-th cursor",
	},
	{
		"num-cursors",
		(*BufPane).NumCursors,
		"num-cursors: returns the number of cursors",
	},
	{
		"cursor-pos",
		(*BufPane).CursorPos,
		"cursor-pos: returns the position of the current cursor",
	},
	{
		"cursor-range",
		(*BufPane).CursorRange,
		"cursor-range: returns the selection range of the current cursor",
	},
	{
		"cursor-has-selection",
		(*BufPane).CursorHasSelection,
		"cursor-range: returns whether the current cursor has a selection",
	},
	{
		"cursor-selection",
		(*BufPane).CursorSelection,
		"cursor-selection: returns the text of the current cursor's selection",
	},
}
