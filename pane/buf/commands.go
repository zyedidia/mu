package buf

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
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
			suspend, resume := bp.Editor.SuspendResume()
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

func (bp *BufPane) CheckModified() error {
	if bp.ExtModified() {
		choice, cancel := bp.messager.CharPrompt("The file being edited has been externally modified. Reload from disk? (y,n,esc)")
		if choice == "y" && !cancel {
			return bp.Reload()
		}
		bp.SetExtModified()
	}
	return nil
}

// --- Editing ---

func (bp *BufPane) InsertAt(pos int, val string) {
	bp.Buffer.Insert(pos, []byte(val))
}

func (bp *BufPane) InsertCmd(val string) {
	c := bp.Cursor()
	if c.HasSelection() {
		bp.Buffer.Remove(c.Sel[0], c.Sel[1])
		c.Deselect(0)
	}
	bp.Buffer.Insert(c.Pos, []byte(val))
}

func (bp *BufPane) RemoveRange(from, to int) int {
	if from > to {
		from, to = to, from
	}
	if from < 0 || from >= to {
		return from
	}
	bp.Buffer.Remove(from, to)
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

func (bp *BufPane) Find(search string) error {
	rxp, err := regexp.Compile(search)
	if err != nil {
		return err
	}
	match := bp.Buffer.FindDown(rxp, bp.Cursor().Pos)
	if match != nil {
		bp.MoveTo(match[0])
		bp.SelectTo(match[1])
		bp.search = rxp
		return nil
	}

	return errors.New("no matches")
}

func (bp *BufPane) FindNext() error {
	if bp.search == nil {
		return errors.New("no search term")
	}

	match := bp.Buffer.FindDown(bp.search, bp.Cursor().Pos)
	if match != nil {
		bp.MoveTo(match[0])
		bp.SelectTo(match[1])
		return nil
	}
	return errors.New("no matches")
}

func (bp *BufPane) FindPrev() error {
	if bp.search == nil {
		return errors.New("no search term")
	}
	match := bp.Buffer.FindUp(bp.search, bp.Cursor().Pos)
	if match != nil {
		bp.MoveTo(match[0])
		bp.SelectTo(match[1])
		return nil
	}
	return errors.New("no matches")
}

func (bp *BufPane) FindLiteral(search string) error {
	return bp.Find(regexp.QuoteMeta(search))
}

func (bp *BufPane) FindPrompt() error {
	start := bp.Cursor().Pos
	search, canceled := bp.messager.IPrompt("find", "Find: ", func(cur string) {
		rxp, err := regexp.Compile(cur)
		if err != nil {
			bp.MoveTo(start)
			return
		}
		match := bp.Buffer.FindDown(rxp, start)
		if match != nil {
			bp.MoveTo(match[0])
			bp.SelectTo(match[1])
		} else {
			bp.MoveTo(start)
		}
		bp.RelocateToCur()
	})
	if canceled {
		bp.MoveTo(start)
		return nil
	}
	bp.MoveTo(start)
	return bp.Find(search)
}

func (bp *BufPane) FindLiteralPrompt() error {
	search, canceled := bp.messager.Prompt("find", "Find (no regex): ")
	if canceled {
		return nil
	}
	return bp.FindLiteral(search)
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

func (bp *BufPane) VisualPos(loc string) int {
	// TODO: we assume the location comes in as a list {x, y} but we should
	// actually check that, or even better use the TclObj interface directly
	// (looks like tcl.AsList is not working right).
	parts := strings.Split(loc[1:len(loc)-1], " ")
	x, _ := strconv.Atoi(parts[0])
	y, _ := strconv.Atoi(parts[1])

	line, col := bp.MouseLoc(x, y)
	return bp.OffsetAt(line, col)
}

var mouseRe = regexp.MustCompile(`\{\d+ \d+\}`)

func parseMouse(loc string) (int, int, error) {
	if !mouseRe.MatchString(loc) {
		return 0, 0, fmt.Errorf("invalid mouse location: %s", loc)
	}
	parts := strings.Split(loc[1:len(loc)-1], " ")
	x, _ := strconv.Atoi(parts[0])
	y, _ := strconv.Atoi(parts[1])
	return x, y, nil
}

func (bp *BufPane) MouseClick(loc string) error {
	x, y, err := parseMouse(loc)
	if err != nil {
		return err
	}
	line, col := bp.MouseLoc(x, y)
	off := bp.OffsetAt(line, col)

	if bp.mouse.drag {
		bp.SelectTo(off)
	} else {
		if time.Since(bp.mouse.release) < mouseClickThreshold {
			if bp.mouse.double {
				bp.mouse.triple = true
			} else {
				bp.mouse.double = true
			}
		} else {
			bp.mouse.double = false
			bp.mouse.triple = false
		}

		bp.MoveTo(off)
	}

	bp.mouse.drag = true
	return nil
}

func (bp *BufPane) MouseRelease(loc string) error {
	if bp.mouse.drag {
		bp.mouse.release = time.Now()
	}
	bp.mouse.drag = false
	return nil
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

func (bp *BufPane) ScrollUp(amt int) {
	topl, _ := bp.LineColAt(bp.stpos)
	topl = max(0, topl-amt)
	bp.stpos = bp.OffsetAt(topl, 0)
}

func (bp *BufPane) ScrollDown(amt int) {
	topl, _ := bp.LineColAt(bp.stpos)
	topl = min(bp.NumLines(), topl+amt)
	bp.stpos = bp.OffsetAt(topl, 0)
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

type command struct {
	Name        string
	Fn          interface{}
	Doc         string
	Relocate    bool
	Multicursor bool
}

var commands = []command{
	{
		Name: "save",
		Fn:   (*BufPane).Save,
		Doc:  "save [path]: save the current buffer",
	},
	{
		Name: "save-as",
		Fn:   (*BufPane).SaveAs,
		Doc:  "save-as <path>: change the current buffer's output and save",
	},
	{
		Name: "insert-at",
		Fn:   (*BufPane).InsertAt,
		Doc:  "insert-at <pos> <text>: insert <text> at <pos>",
	},
	{
		Name:     "insert",
		Fn:       (*BufPane).InsertCmd,
		Doc:      "insert-at <text>: insert <text> at the current cursor",
		Relocate: true,
	},
	{
		Name: "remove-range",
		Fn:   (*BufPane).RemoveRange,
		Doc:  "remove-range <from> <to>: remove the bytes in the range [<from>:<to>)",
	},
	{
		Name:     "remove-to",
		Fn:       (*BufPane).RemoveTo,
		Doc:      "remove-to <to>: remove the bytes in the range [<cursor>:<to>)",
		Relocate: true,
	},
	{
		Name:     "remove-selection",
		Fn:       (*BufPane).RemoveSelection,
		Doc:      "remove-selection: remove the current selection",
		Relocate: true,
	},
	{
		Name: "read",
		Fn:   (*BufPane).Read,
		Doc:  "read <from> <to>: return the buffer contents in the range [<from>:<to>)",
	},
	{
		Name: "read-line",
		Fn:   (*BufPane).ReadLine,
		Doc:  "read-line <line>: return the contents of <line>",
	},
	{
		Name: "read-all",
		Fn:   (*BufPane).ReadAll,
		Doc:  "read-all: return the contents of the current buffer",
	},
	{
		Name: "find-down",
		Fn:   (*BufPane).FindDown,
		Doc:  "find-down <pos> <regex>: search down from <pos> for <regex> and return match as a pair of positions",
	},
	{
		Name: "find-up",
		Fn:   (*BufPane).FindUp,
		Doc:  "find-up <pos> <regex>: search up from <pos> for <regex> and return match as a pair of positions",
	},
	{
		Name: "filetype",
		Fn:   (*BufPane).Filetype,
		Doc:  "filetype: return the filetype of the current buffer",
	},
	{
		Name: "name",
		Fn:   (*BufPane).Name,
		Doc:  "name: return the name of the current buffer",
	},
	{
		Name: "line-col",
		Fn:   (*BufPane).LineCol,
		Doc:  "line-col <pos>: return the line/col pair corresponding to a byte offset",
	},
	{
		Name: "offset",
		Fn:   (*BufPane).Offset,
		Doc:  "offset <line> <col>: return the offset corresponding to a line/col pair",
	},
	{
		Name: "size",
		Fn:   (*BufPane).Size,
		Doc:  "size: return the number of bytes in the buffer",
	},
	{
		Name: "left",
		Fn:   (*BufPane).Left,
		Doc:  "left <pos>: returns the resulting position from moving a cursor at <pos> left one character",
	},
	{
		Name: "right",
		Fn:   (*BufPane).Right,
		Doc:  "right <pos>: returns the resulting position from moving a cursor at <pos> right one character",
	},
	{
		Name: "up",
		Fn:   (*BufPane).Up,
		Doc:  "up <pos>: returns the resulting position from moving a cursor at <pos> up one line",
	},
	{
		Name: "down",
		Fn:   (*BufPane).Down,
		Doc:  "down <pos>: returns the resulting position from moving a cursor at <pos> down one line",
	},
	{
		Name: "word-left",
		Fn:   (*BufPane).WordLeft,
		Doc:  "word-left <pos>: returns the resulting position from moving a cursor at <pos> left one word",
	},
	{
		Name: "word-right",
		Fn:   (*BufPane).WordRight,
		Doc:  "word-right <pos>: returns the resulting position from moving a cursor at <pos> right one word",
	},
	{
		Name: "ws-left",
		Fn:   (*BufPane).WordLeftWS,
		Doc:  "ws-left <pos>: returns the resulting position from moving a cursor at <pos> left until whitespace",
	},
	{
		Name: "ws-right",
		Fn:   (*BufPane).WordRightWS,
		Doc:  "ws-right <pos>: returns the resulting position from moving a cursor at <pos> right until the next word, defined by whitespace",
	},
	{
		Name: "word-end",
		Fn:   (*BufPane).WordEnd,
		Doc:  "word-end <pos>: returns the resulting position from moving a cursor at <pos> right until the end of a word",
	},
	{
		Name: "ws-end",
		Fn:   (*BufPane).WordEndWS,
		Doc:  "ws-end <pos>: returns the resulting position from moving a cursor at <pos> right until whitespace",
	},
	{
		Name: "line-start",
		Fn:   (*BufPane).LineStart,
		Doc:  "line-start <pos>:",
	},
	{
		Name: "next-line-start",
		Fn:   (*BufPane).NextLineStart,
		Doc:  "next-line-start <pos>:",
	},
	{
		Name: "line-end",
		Fn:   (*BufPane).LineEnd,
		Doc:  "line-end <pos>:",
	},
	{
		Name: "find-char",
		Fn:   (*BufPane).FindChar,
		Doc:  "find-char <char> <pos>: jump to the first occurrence of <char> in the current line, starting from <pos>",
	},
	{
		Name: "find-char-back",
		Fn:   (*BufPane).FindCharBack,
		Doc:  "find-char-back <char> <pos>: jump backwards to the first occurrence of <char> in the current line",
	},
	{
		Name: "till-char",
		Fn:   (*BufPane).TillChar,
		Doc:  "till-char <char> <pos>: jump to the first occurrence of <char> in the current line, starting from <pos>",
	},
	{
		Name: "till-char-back",
		Fn:   (*BufPane).TillCharBack,
		Doc:  "till-char-back <char> <pos>: jump backwards to the first occurrence of <char> in the current line",
	},
	{
		Name:     "move-to",
		Fn:       (*BufPane).MoveTo,
		Doc:      "move-to <pos>: move the current cursor to <pos>",
		Relocate: true,
	},
	{
		Name:     "select-to",
		Fn:       (*BufPane).SelectTo,
		Doc:      "select-to <pos>: move the current cursor to <pos> and make a selection",
		Relocate: true,
	},
	{
		Name: "switch-cursor",
		Fn:   (*BufPane).SwitchCursor,
		Doc:  "switch-cursor <idx>: change the active cursor to the <idx>-th cursors",
	},
	{
		Name: "spawn-cursor",
		Fn:   (*BufPane).SpawnCursor,
		Doc:  "spawn-cursor <pos>: spawn a new cursor at <pos>",
	},
	{
		Name: "remove-cursor",
		Fn:   (*BufPane).RemoveCursor,
		Doc:  "remove-cursor <idx>: remove the <idx>-th cursor",
	},
	{
		Name: "num-cursors",
		Fn:   (*BufPane).NumCursors,
		Doc:  "num-cursors: returns the number of cursors",
	},
	{
		Name: "cursor-pos",
		Fn:   (*BufPane).CursorPos,
		Doc:  "cursor-pos: returns the position of the current cursor",
	},
	{
		Name: "cursor-col",
		Fn:   (*BufPane).CursorCol,
		Doc:  "cursor-col: returns the column number of the current cursor",
	},
	{
		Name: "cursor-line",
		Fn:   (*BufPane).CursorLine,
		Doc:  "cursor-line: returns the line number of the current cursor",
	},
	{
		Name: "cursor-range",
		Fn:   (*BufPane).CursorRange,
		Doc:  "cursor-range: returns the selection range of the current cursor",
	},
	{
		Name: "cursor-has-selection",
		Fn:   (*BufPane).CursorHasSelection,
		Doc:  "cursor-has-selection: returns whether the current cursor has a selection",
	},
	{
		Name: "cursor-selection",
		Fn:   (*BufPane).CursorSelection,
		Doc:  "cursor-selection: returns the text of the current cursor's selection",
	},
	{
		Name: "relocate",
		Fn:   (*BufPane).RelocateToCur,
		Doc:  "relocate:",
	},
	{
		Name: "scroll-up",
		Fn:   (*BufPane).ScrollUp,
		Doc:  "scroll-up <n>: scroll up <n> lines",
	},
	{
		Name: "scroll-down",
		Fn:   (*BufPane).ScrollDown,
		Doc:  "scroll-down <n>: scroll down <n> lines:",
	},
	{
		Name: "vim-clamp",
		Fn:   (*BufPane).VimClamp,
		Doc:  "vim-clamp <pos>:",
	},
	{
		Name: "undo",
		Fn:   (*BufPane).Undo,
		Doc:  "undo:",
	},
	{
		Name: "redo",
		Fn:   (*BufPane).Redo,
		Doc:  "redo:",
	},
	{
		Name:     "paste",
		Fn:       (*BufPane).Paste,
		Doc:      "paste: inserts the contents of the clipboard at the current cursor's position",
		Relocate: true,
	},
	{
		Name:     "find",
		Fn:       (*BufPane).Find,
		Doc:      "find <regex>: searches for a regular expression",
		Relocate: true,
	},
	{
		Name:     "find-literal",
		Fn:       (*BufPane).FindLiteral,
		Doc:      "find-literal <search>: searches for a literal string",
		Relocate: true,
	},
	{
		Name:     "find-prompt",
		Fn:       (*BufPane).FindPrompt,
		Doc:      "find-prompt: opens an interactive prompt for regex searching",
		Relocate: true,
	},
	{
		Name:     "find-literal-prompt",
		Fn:       (*BufPane).FindLiteralPrompt,
		Doc:      "find-literal-prompt: opens an interactive prompt for literal searching",
		Relocate: true,
	},
	{
		Name:     "find-next",
		Fn:       (*BufPane).FindNext,
		Doc:      "find-next: search for next occurrence of the last search term",
		Relocate: true,
	},
	{
		Name:     "find-prev",
		Fn:       (*BufPane).FindPrev,
		Doc:      "find-next: search for previous occurrence of the last search term",
		Relocate: true,
	},
	{
		Name: "check-modified",
		Fn:   (*BufPane).CheckModified,
		Doc:  "check-modified: checks if the current buffer has been externally modified",
	},
	{
		Name: "visual-pos",
		Fn:   (*BufPane).VisualPos,
		Doc:  "visual-pos <x> <y>: returns the buffer position associated with the visual x, y position",
	},
	{
		Name:     "mouse-click",
		Fn:       (*BufPane).MouseClick,
		Doc:      "mouse-click <pos>: handle a mouse click at <pos>",
		Relocate: true,
	},
	{
		Name:     "mouse-release",
		Fn:       (*BufPane).MouseRelease,
		Doc:      "mouse-release <pos>: handle a mouse release at <pos>",
		Relocate: true,
	},
}

var statuscmds = []tclutil.Command{
	{
		Name: "modified",
		Fn:   (*BufPane).Modified,
		Doc:  "modified: returns a symbol indicating if the buffer is modified",
	},
	{
		Name: "name",
		Fn:   (*BufPane).Name,
		Doc:  "name: return the name of the current buffer",
	},
	{
		Name: "line-col",
		Fn:   (*BufPane).LineCol,
		Doc:  "line-col <pos>: return the line/col pair corresponding to a byte offset",
	},
	{
		Name: "offset",
		Fn:   (*BufPane).Offset,
		Doc:  "offset <line> <col>: return the offset corresponding to a line/col pair",
	},
	{
		Name: "size",
		Fn:   (*BufPane).Size,
		Doc:  "size: return the number of bytes in the buffer",
	},
	{
		Name: "cursor-col",
		Fn:   (*BufPane).CursorCol,
		Doc:  "cursor-col: returns the column number of the current cursor",
	},
	{
		Name: "cursor-line",
		Fn:   (*BufPane).CursorLine,
		Doc:  "cursor-line: returns the line number of the current cursor",
	},
	{
		Name: "filetype",
		Fn:   (*BufPane).Filetype,
		Doc:  "filetype: return the filetype of the current buffer",
	},
}
