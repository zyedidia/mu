package buf

import (
	"github.com/zyedidia/mu/pkg/tclutil"
)

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
		Fn:       (*BufPane).InsertString,
		Doc:      "insert-at <text>: insert <text> at the current cursor",
		Relocate: true,
	},
	{
		Name:     "newline",
		Fn:       (*BufPane).Newline,
		Doc:      "newline: insert a newline at the current cursor",
		Relocate: true,
	},
	{
		Name:     "indent",
		Fn:       (*BufPane).Indent,
		Doc:      "indent: increase the current indentation level",
		Relocate: true,
	},
	{
		Name:     "autoindent",
		Fn:       (*BufPane).Autoindent,
		Doc:      "autoindent: automatically indent the current line",
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
		Name: "remove-cursors",
		Fn:   (*BufPane).RemoveCursors,
		Doc:  "remove-cursors: remove all extra cursors",
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
		Name:     "copy",
		Fn:       (*BufPane).Copy,
		Doc:      "copy: copies the current selection to the clipboard",
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
		Name:     "replace",
		Fn:       (*BufPane).Replace,
		Doc:      "replace <search> <replacement>: replace <search> with <replacement> and prompt for each replacement",
		Relocate: true,
	},
	{
		Name:     "replace-all",
		Fn:       (*BufPane).ReplaceAll,
		Doc:      "replace-all <search> <replacement>: replace <search> with <replacement> without prompting",
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
		Name: "mouse-release",
		Fn:   (*BufPane).MouseRelease,
		Doc:  "mouse-release <pos>: handle a mouse release at <pos>",
	},
	{
		Name: "lsp-hover",
		Fn:   (*BufPane).LspHover,
		Doc:  "lsp-hover: lists LSP hover information from the current cursor",
	},
	{
		Name: "lsp-format",
		Fn:   (*BufPane).LspFormat,
		Doc:  "lsp-format: auto-format the current document",
	},
	{
		Name: "deselect",
		Fn:   (*BufPane).Deselect,
		Doc:  "deselect: removes the current selection",
	},
	{
		Name: "complete",
		Fn:   (*BufPane).Complete,
		Doc:  "complete <allow-empty>: make an autocompletion suggestion; allow-empty specifies whether the autocomplete may provide suggestions even without any input",
	},
	{
		Name: "next-completion",
		Fn:   (*BufPane).NextCompletion,
		Doc:  "next-completion: select the next completion",
	},
	{
		Name: "prev-completion",
		Fn:   (*BufPane).PrevCompletion,
		Doc:  "prev-completion: select the previous completion",
	},
	{
		Name: "cancel-completion",
		Fn:   (*BufPane).CancelCompletion,
		Doc:  "cancel-completion: cancel the current completion",
	},
	{
		Name: "word-wrap",
		Fn:   (*BufPane).WordWrap,
		Doc:  "word-wrap: word-wrap the current selection",
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
