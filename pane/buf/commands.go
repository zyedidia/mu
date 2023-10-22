package buf

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"unicode"

	"github.com/zyedidia/mu/pkg/output"
	"github.com/zyedidia/mu/pkg/tclutil"
)

func (bp *BufPane) Save(args []string) error {
	if len(args) >= 1 {
		return bp.SaveAs(args[0])
	}

	if !bp.HasOutput() {
		path, canceled := bp.messager.Prompt("save", "Filename: ")
		if canceled {
			return errors.New("save failed: no output file")
		}
		return bp.SaveAs(path)
	}
	err := bp.Buffer.Save()
	if errors.Is(err, os.ErrPermission) {
		if ok, err := bp.saveWithSudo(); ok {
			return err
		}
	}
	return err
}

func (bp *BufPane) SaveAs(path string) error {
	bp.SetOutput(&output.File{
		Path: path,
	})
	err := bp.Buffer.Save()
	if errors.Is(err, os.ErrPermission) {
		if ok, err := bp.saveWithSudo(); ok {
			return err
		}
	}
	return err
}

func (bp *BufPane) saveWithSudo() (bool, error) {
	if f := bp.FileOutput(); f != nil && output.HasRootFile {
		choice, cancel := bp.messager.CharPrompt("File cannot be written, save with sudo? (y,n,esc)")
		if cancel {
			return false, nil
		}
		if choice == "y" {
			suspend, resume := bp.editor.SuspendResume()
			bp.SetOutput(&output.RootFile{
				Suspend: suspend,
				Resume:  resume,
				RootCmd: "sudo",
				Path:    f.Path,
			})
			return true, bp.Save(nil)
		}
	}
	return false, nil
}

// --- Editing ---

func (bp *BufPane) InsertAt(pos int, val string) {
	bp.Buffer.Insert(pos, []byte(val))
}

func (bp *BufPane) Remove(from, to int) int {
	if from > to {
		from, to = to, from
	}
	if from < 0 || from >= to {
		return from
	}
	bp.Buffer.Remove(from, to)
	return from
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
	c := bp.CursorUp(bp.GetCursorAt(from))
	return c.Pos
}

func (bp *BufPane) Down(from int) int {
	c := bp.CursorDown(bp.GetCursorAt(from))
	return c.Pos
}

func (bp *BufPane) Left(from int) int {
	c := bp.GetCursorAt(from).Left(bp.Buffer)
	return c.Pos
}

func (bp *BufPane) Right(from int) int {
	c := bp.GetCursorAt(from).Right(bp.Buffer)
	return c.Pos
}

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
func isNotSpace(r rune) bool {
	return !unicode.IsSpace(r)
}

func (bp *BufPane) WordLeft(from int) int {
	c := bp.GetCursorAt(from).WordLeft(bp.Buffer, isWord)
	return c.Pos
}

func (bp *BufPane) WordRight(from int) int {
	c := bp.GetCursorAt(from).WordRight(bp.Buffer, isWord)
	return c.Pos
}

func (bp *BufPane) WordLeftWS(from int) int {
	c := bp.GetCursorAt(from).WordLeft(bp.Buffer, isNotSpace)
	return c.Pos
}

func (bp *BufPane) WordRightWS(from int) int {
	c := bp.GetCursorAt(from).WordRight(bp.Buffer, isNotSpace)
	return c.Pos
}

func (bp *BufPane) WordEnd(from int) int {
	c := bp.GetCursorAt(from).WordEnd(bp.Buffer, isWord)
	return c.Pos
}

func (bp *BufPane) WordEndWS(from int) int {
	c := bp.GetCursorAt(from).WordEnd(bp.Buffer, isNotSpace)
	return c.Pos
}

func (bp *BufPane) FindChar(c rune, from int) int {
	_, sz := bp.DecodeRuneAt(from)
	p := from + sz
	for {
		r, sz := bp.DecodeRuneAt(p)
		if r == '\n' || sz == 0 {
			return from
		} else if r == c {
			return p
		}
		p += sz
	}
}

func (bp *BufPane) FindCharBack(c rune, from int) int {
	p := from
	for {
		r, sz := bp.DecodeRuneBefore(p)
		if r == '\n' || sz == 0 {
			return from
		} else if r == c {
			return p - sz
		}
		p -= sz
	}
}

func (bp *BufPane) TillChar(c rune, from int) int {
	_, sz := bp.DecodeRuneAt(from)
	last := sz
	p := from + sz
	for {
		r, sz := bp.DecodeRuneAt(p)
		if r == '\n' || sz == 0 {
			return from
		} else if r == c {
			return p - last
		}
		last = sz
		p += sz
	}
}

func (bp *BufPane) TillCharBack(c rune, from int) int {
	p := from
	for {
		r, sz := bp.DecodeRuneBefore(p)
		if r == '\n' || sz == 0 {
			return from
		} else if r == c {
			return p
		}
		p -= sz
	}
}

func (bp *BufPane) LineStart(from int) int {
	for {
		r, sz := bp.DecodeRuneBefore(from)
		if r == '\n' || sz == 0 {
			return from
		}
		from -= sz
	}
}

func (bp *BufPane) LineEnd(from int) int {
	for {
		r, sz := bp.DecodeRuneAt(from)
		if r == '\n' || sz == 0 {
			return from
		}
		from += sz
	}
}

func (bp *BufPane) NextLineStart(from int) int {
	for {
		r, sz := bp.DecodeRuneAt(from)
		if r == '\n' || sz == 0 {
			return from + 1
		}
		from += sz
	}
}

func (bp *BufPane) VimClamp(from int) int {
	r, sz := bp.DecodeRuneAt(from)
	if r != '\n' || sz == 0 {
		return from
	} else {
		b, sz := bp.DecodeRuneBefore(from)
		if b == '\n' || sz == 0 {
			return from
		}
	}
	return from - 1
}

// --- Cursors ---

func (bp *BufPane) MoveTo(pos int) {
	c := bp.Cursor()
	*c = c.MoveTo(pos)
	if !bp.vertical {
		bp.RecalcVX(c)
	}
	bp.vertical = false
}

func (bp *BufPane) SelectTo(pos int) {
	c := bp.Cursor()
	*c = c.SelectTo(pos)
	if !bp.vertical {
		bp.RecalcVX(c)
	}
	bp.vertical = false
}

func (bp *BufPane) CursorPos() int {
	return bp.Cursor().Pos
}
func (bp *BufPane) CursorCol() int {
	_, c := bp.LineColAt(bp.Cursor().Pos)
	return c + 1
}
func (bp *BufPane) CursorLine() int {
	l, _ := bp.LineColAt(bp.Cursor().Pos)
	return l + 1
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

// --- Search ---

func (bp *BufPane) Find(search string) error {
	rxp, err := regexp.Compile(search)
	if err != nil {
		return err
	}
	match := bp.Buffer.FindDown(rxp, bp.Cursor().Pos)
	if match != nil {
		bp.MoveTo(match[0])
		return nil
	}

	return errors.New("no matches")
}

func (bp *BufPane) FindLiteral(search string) error {
	return bp.Find(regexp.QuoteMeta(search))
}

func (bp *BufPane) FindPrompt() error {
	search, canceled := bp.messager.Prompt("find", "Find: ")
	if canceled {
		return nil
	}
	return bp.Find(search)
}

func (bp *BufPane) FindLiteralPrompt() error {
	search, canceled := bp.messager.Prompt("find", "Find (no regex): ")
	if canceled {
		return nil
	}
	return bp.FindLiteral(search)
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

// --- View ---

func (bp *BufPane) RelocateToCur() {
	line, col := bp.LineColAt(bp.Cursor().Pos)
	bp.Relocate(bLoc{line, col})
}

// --- Options ---

func (bp *BufPane) Filetype() string {
	return bp.Buffer.Filetype()
}

func (bp *BufPane) Name() string {
	return bp.Buffer.Name()
}

func (bp *BufPane) Modified() string {
	if bp.Buffer.Modified() {
		return "+ "
	}
	return ""
}

var commands = []tclutil.Command{
	{
		"save",
		(*BufPane).Save,
		"save [path]: save the current buffer",
	},
	{
		"save-as",
		(*BufPane).SaveAs,
		"save-as <path>: change the current buffer's output and save",
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
		"word-left",
		(*BufPane).WordLeft,
		"word-left <pos>: returns the resulting position from moving a cursor at <pos> left one word",
	},
	{
		"word-right",
		(*BufPane).WordRight,
		"word-right <pos>: returns the resulting position from moving a cursor at <pos> right one word",
	},
	{
		"ws-left",
		(*BufPane).WordLeftWS,
		"ws-left <pos>: returns the resulting position from moving a cursor at <pos> left until whitespace",
	},
	{
		"ws-right",
		(*BufPane).WordRightWS,
		"ws-right <pos>: returns the resulting position from moving a cursor at <pos> right until the next word, defined by whitespace",
	},
	{
		"word-end",
		(*BufPane).WordEnd,
		"word-end <pos>: returns the resulting position from moving a cursor at <pos> right until the end of a word",
	},
	{
		"ws-end",
		(*BufPane).WordEndWS,
		"ws-end <pos>: returns the resulting position from moving a cursor at <pos> right until whitespace",
	},
	{
		"line-start",
		(*BufPane).LineStart,
		"line-start <pos>:",
	},
	{
		"next-line-start",
		(*BufPane).NextLineStart,
		"next-line-start <pos>:",
	},
	{
		"line-end",
		(*BufPane).LineEnd,
		"line-end <pos>:",
	},
	{
		"find-char",
		(*BufPane).FindChar,
		"find-char <char> <pos>: jump to the first occurrence of <char> in the current line, starting from <pos>",
	},
	{
		"find-char-back",
		(*BufPane).FindCharBack,
		"find-char-back <char> <pos>: jump backwards to the first occurrence of <char> in the current line",
	},
	{
		"till-char",
		(*BufPane).TillChar,
		"till-char <char> <pos>: jump to the first occurrence of <char> in the current line, starting from <pos>",
	},
	{
		"till-char-back",
		(*BufPane).TillCharBack,
		"till-char-back <char> <pos>: jump backwards to the first occurrence of <char> in the current line",
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
		"cursor-col",
		(*BufPane).CursorCol,
		"cursor-col: returns the column number of the current cursor",
	},
	{
		"cursor-line",
		(*BufPane).CursorLine,
		"cursor-line: returns the line number of the current cursor",
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
	{
		"relocate",
		(*BufPane).RelocateToCur,
		"relocate:",
	},
	{
		"vim-clamp",
		(*BufPane).VimClamp,
		"vim-clamp <pos>:",
	},
	{
		"undo",
		(*BufPane).Undo,
		"undo:",
	},
	{
		"redo",
		(*BufPane).Redo,
		"redo:",
	},
	{
		"paste",
		(*BufPane).Paste,
		"paste: inserts the contents of the clipboard at the current cursor's position",
	},
	{
		"find",
		(*BufPane).Find,
		"find <regex>: searches for a regular expression",
	},
	{
		"find-literal",
		(*BufPane).FindLiteral,
		"find-literal <search>: searches for a literal string",
	},
	{
		"find-prompt",
		(*BufPane).FindPrompt,
		"find-prompt: opens an interactive prompt for regex searching",
	},
	{
		"find-literal-prompt",
		(*BufPane).FindLiteralPrompt,
		"find-literal-prompt: opens an interactive prompt for literal searching",
	},
}

var statuscmds = []tclutil.Command{
	{
		"modified",
		(*BufPane).Modified,
		"modified: returns a symbol indicating if the buffer is modified",
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
		"cursor-col",
		(*BufPane).CursorCol,
		"cursor-col: returns the column number of the current cursor",
	},
	{
		"cursor-line",
		(*BufPane).CursorLine,
		"cursor-line: returns the line number of the current cursor",
	},
	{
		"filetype",
		(*BufPane).Filetype,
		"filetype: return the filetype of the current buffer",
	},
}
