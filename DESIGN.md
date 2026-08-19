# Mu Text Editor - Design

Mu is a terminal text editor written in Go with vim keybindings, designed as a
personal editor and potential successor to micro. It draws from three reference
projects: vis (vim keybinding architecture), micro (Go text editor by the same
author), and the previous mu rewrite (rope data structure, flare highlighting,
TCL commands, TOML config, themes, LSP).

## Design Decisions

- **Flat source structure** like vis: all editor Go files in the root package,
  one sub-package (`text/`) for the reusable rope buffer.
- **Hard-coded vim keybindings** in Go (like vis), not a PEG DSL. TCL is used
  only for ex-commands.
- **Multiple cursors** built in from day one.
- **Tree-based undo** with branching and time coalescing (from old mu).
- **Windowed syntax highlighting** via flare with ~1MB window, re-centered when
  the cursor approaches the edge.
- **TOML options** with per-filetype and glob-pattern overrides.
- **YAML themes** with hierarchical style lookups.
- **TCL command language** (gotcl) for ex-commands. No Lua plugin system.
- **Basic LSP** support: completion, hover, go-to-definition, diagnostics,
  formatting.
- **gdamore/tcell/v2** for terminal I/O (upstream, not micro-editor fork).
- **Grapheme-aware** cursor movement and display via zyedidia/uniseg.

Deferred: terminal pane, directory browser, mouse support, macro recording.

## Core Data Structures

### Editor

Central orchestrator. Owns all global state.

```go
type Editor struct {
    tabs    []*Tab
    curtab  int
    active  Pane           // currently focused pane

    buffers []*Buffer      // all open buffers (shared across splits)

    interp  *tcl.Interp   // TCL interpreter for ex-commands
    lsp     *LspManager    // LSP server lifecycle

    theme   *Theme
    config  *Config        // parsed TOML options

    clipboard Clipboard    // system clipboard

    w, h    int            // terminal dimensions
    infobar *InfoBar

    Redraw  chan struct{}
    Errors  chan error
    Suspend chan func()
    Resume  chan struct{}
}
```

Event loop (in `main.go`):
1. Initialize tcell screen, load config/theme, open files.
2. Spawn goroutine polling `screen.PollEvent()` into a channel.
3. Main select loop: handle key/resize events, redraw signals, errors, suspend.
4. On key event: dispatch through the active pane's vim mode system.
5. After processing: draw all visible panes via `Editor.Display()`.

### Buffer

Editor-level buffer wrapping `text.Buffer`. One per open file, shared across
splits.

```go
type Buffer struct {
    *text.Buffer                   // underlying rope-based text

    cursors  []*Cursor             // multi-cursor list (primary is [0])
    undo     *UndoTree[*Buffer]    // tree-based undo

    Path     string                // file path
    Name     string                // display name
    Modified bool

    highlighter *flare.Highlighter // syntax highlighting
    syntaxWin   SyntaxWindow       // windowed highlight state
    matches     *flare.Matches

    lsp      *LspServer            // active LSP connection
    ft       string                // detected filetype
    opts     map[string]any        // resolved options for this buffer
}
```

### Cursor

Byte-offset based cursor with selection support.

```go
type Cursor struct {
    buf     *Buffer
    Pos     int        // byte offset in buffer
    HasSel  bool
    Sel     [2]int     // selection range [start, end)
    Orig    [2]int     // original anchor for visual mode
    Vx      int        // desired visual column (for vertical movement)
    id      int        // unique cursor id
}
```

Multi-cursor operations: all motions/operators apply to every cursor. Cursors
are kept sorted by position. Overlapping selections merge after each operation.

### UndoTree

Generic tree-based undo with branching redo paths. Ported from old mu.

```go
type Delta interface {
    Do(buf *Buffer)
    Undo(buf *Buffer)
}

type Event struct {
    Deltas  []Delta
    Time    time.Time
    Cursors []CursorState  // snapshot cursor positions
    Next    []int          // children event indices (branching redo)
    Prev    int            // parent event index
}

type UndoTree struct {
    events  []Event
    current int            // index of current event
    cutoff  int            // max events (0 = unlimited)
}
```

Edits within a short time window (~1s) coalesce into one event. Undo moves to
parent, redo moves to the most recently visited child.

## Vim Mode System

Modeled after vis. Modes form an inheritance tree for key binding fallback.

### Modes

```go
type ModeID int

const (
    ModeNormal ModeID = iota
    ModeInsert
    ModeReplace
    ModeVisual
    ModeVisualLine
    ModeOperatorPending
    ModeCommand         // : prompt
)

type Mode struct {
    ID       ModeID
    Name     string
    Parent   *Mode                           // binding inheritance
    Bindings map[string]*KeyBinding          // key sequence -> binding
    OnEnter  func(e *Editor)
    OnLeave  func(e *Editor)
    OnInput  func(e *Editor, key string)     // unbound key handler
    IsVisual bool
}
```

Mode hierarchy:
```
Normal
├── Visual
│   └── Visual-Line
├── Operator-Pending
Insert
Replace (parent: Insert for shared Escape handling)
Command
```

### Action Composition

Like vis, the pending action accumulates state as keys arrive:

```go
type Action struct {
    Count    int            // numeric prefix (0 = default)
    Register RegisterID     // target register (default = ")
    Op       *Operator      // nil until operator key pressed
    Motion   *Motion        // nil until motion key pressed
    TextObj  *TextObject    // alternative to motion (iw, a", etc.)
    Linewise bool
}
```

Flow: `[count][register][operator][count][motion|textobject]`

Example: `"a3dw` → register=a, count=3, op=delete, motion=word-right.

When both operator and motion/textobject are set, execute:
1. For each cursor, compute the range from motion or textobject.
2. Apply the operator to each range.
3. Update cursor positions.
4. Record in undo tree.
5. Clear the action.

### Motions

```go
type Motion struct {
    Fn       func(b *Buffer, c *Cursor, count int) int  // returns new byte offset
    Flags    MotionFlags  // Linewise, Inclusive, Jump, etc.
}
```

Core motions: h/l (char), j/k (line), w/W/b/B/e/E (word), 0/^/$ (line
bounds), gg/G (file bounds), f/F/t/T (char find), /{?} (search), %
(matching bracket), {/} (paragraph).

### Operators

```go
type Operator struct {
    Fn func(e *Editor, b *Buffer, cursors []*Cursor, ranges []Range, reg RegisterID)
}
```

Core operators: d (delete), c (change), y (yank), > / < (indent), ~ (swap
case), gu/gU (lower/upper case), = (format via LSP).

Special cases:
- `dd`, `cc`, `yy`: operator doubled = operate on whole line.
- `.`: repeat last action (store the full Action for replay).

### Text Objects

```go
type TextObject struct {
    Inner func(b *Buffer, pos int, count int) Range
    Outer func(b *Buffer, pos int, count int) Range
}
```

Core text objects: w (word), W (WORD), s (sentence), p (paragraph), and
delimiter pairs: () [] {} <> "" '' ``.

### Key Dispatch

```go
type KeyBinding struct {
    Action  func(e *Editor, keys string) string  // returns unconsumed keys
    // OR
    Alias   string                                // replacement key sequence
}
```

Key dispatch algorithm (in `keybind.go`):
1. Accumulate input in a key buffer.
2. Search current mode's bindings, then parent modes.
3. If exact match: execute binding, consume keys.
4. If prefix match: wait for more input.
5. If no match: call mode's OnInput handler (insert char in insert mode, beep
   in normal mode).

### Registers

```go
type RegisterID byte  // '"', '+', '0', 'a'-'z', '_' (blackhole)

type Register struct {
    Content  []byte
    Linewise bool
}
```

Special registers:
- `"` (unnamed): default for d/c/y.
- `+` (clipboard): system clipboard.
- `0` (yank): last yank only (not delete).
- `_` (blackhole): discard.
- `a`-`z`: named. Uppercase `A`-`Z` appends.

## View and Display

### View

Each split pane has a View that manages the visible window into a buffer.

```go
type View struct {
    buf         *Buffer
    topLine     int          // first visible line number
    leftCol     int          // horizontal scroll offset
    width       int          // viewport columns
    height      int          // viewport rows
    scrollOff   int          // vertical scroll margin (from config)
    hScrollOff  int          // horizontal scroll margin
}
```

Scrolling: after cursor movement, adjust topLine/leftCol so the cursor stays
within the margins. Soft wrap: if enabled, long lines wrap at viewport width
(logical vs visual line distinction).

### Display / Visualizer

Rendering walks through the visible portion of the buffer grapheme by grapheme:

```go
type Visualizer struct {
    TabSize int
    CharMap map[rune]string  // e.g., '\t' -> "|  "
}
```

For each grapheme at offset `off`:
1. `uniseg.DecodeAt(buf, off)` → rune, combining runes, byte size, display width.
2. `Visualizer.Size(r, vx, width)` → actual columns consumed (tabs expand).
3. Apply syntax highlight style from `flare.Matches`.
4. Apply cursor/selection overlay.
5. Emit via `tcell.SetContent(x, y, mainc, combc, style)`.

### Gutter

Renders to the left of the text area:
- Line numbers (absolute or relative, from config).
- Diagnostic markers (errors/warnings from LSP).
- Width auto-adjusts to the number of digits needed.

## Syntax Highlighting

Uses `zyedidia/flare` with a **windowed** approach for large files.

```go
type SyntaxWindow struct {
    Start   int     // byte offset of window start
    End     int     // byte offset of window end
    Size    int     // window size in bytes (default ~1MB)
    Margin  int     // re-center threshold (~100KB from edge)
}
```

Algorithm:
1. On file open: if file size <= window size, highlight the entire file.
2. Otherwise, center a window around the cursor position.
3. Run `flare.Highlighter` on the windowed text, store matches.
4. On cursor movement: if cursor is within `Margin` bytes of the window edge,
   re-center the window around the cursor and re-highlight.
5. Highlighting runs in a background goroutine. A semaphore prevents concurrent
   highlights. A redraw signal is sent on completion.
6. Incremental re-highlighting: on edits, invalidate from the edited line and
   re-highlight forward using the memoization table.

Filetype detection: `zyedidia/ftdetect` matches filename patterns and content
heuristics to select the appropriate `.lang` grammar file. Grammars are looked
up in the config directory first (`~/.config/mu/highlighters/`), then the ones
built into flare; `include` directives inside a grammar resolve the same way.

## Configuration

### TOML Options

```toml
# Global options
autoindent = true
autoformat = false         # format via LSP before every save
cursor = "block"           # block, bar, underline
theme = "monokai"
syntax = true
tabsize = 4
tabstospaces = true
scrollmargin = 3
hscrollmargin = 1
linenums = true
softwrap = false
wordwrap = false
clipboard = "external"     # internal, external, terminal
cursorline = true

# Per-filetype overrides
[makefile]
tabstospaces = false

# Glob pattern overrides
["glob:*.md"]
softwrap = true
wordwrap = true
```

```go
type Config struct {
    globals map[string]any              // editor-wide options (theme, clipboard, cursor)
    top     map[string]any              // default buffer options
    ft      []FtOpts                    // filetype-specific overrides
}

type FtOpts struct {
    Pattern string                      // filetype name or "glob:pattern"
    Opts    map[string]any
}
```

Resolution order for a buffer option: glob match → filetype match → top-level
default → built-in default. Global options (theme, clipboard, cursor) are
editor-wide and never overridden per-buffer.

Config directory: `~/.config/mu/` (XDG).

### LSP Configuration

```yaml
go:
  command: gopls
  args: []
python:
  command: pylsp
  args: []
rust:
  command: rust-analyzer
  args: []
```

## Themes

YAML files with hierarchical style group lookups.

```yaml
default:
  fg: white
  bg: black
comment:
  fg: gray
  attr: italic
keyword:
  fg: blue
  attr: bold
string:
  fg: green
error:
  fg: red
  attr: underline
gutter:
  fg: gray
statusline:
  fg: black
  bg: white
```

```go
type Theme struct {
    def   Style
    rules map[string]Style
}

type Style struct {
    Fg   Color
    Bg   Color
    Attr Attr   // bold, italic, underline, reverse, dim
}

type Color struct {
    // Supports: named (red, blue), 256-palette (0-255),
    // true color (#RRGGBB), default (terminal default)
}
```

`Theme.Style("string.escape")` looks up `string.escape`, then falls back to
`string`, then to `default`.

## LSP

Minimal LSP client communicating via JSON-RPC 2.0 over stdin/stdout.

```go
type LspManager struct {
    servers map[string]*LspServer   // one per language/filetype
    config  map[string]LspConfig    // from lsp.yaml
}

type LspServer struct {
    cmd          *exec.Cmd
    stdin        io.WriteCloser
    stdout       *bufio.Reader
    capabilities ServerCapabilities
    nextID       int
    responses    map[int]chan json.RawMessage
    lock         sync.Mutex
}
```

Supported requests:
- `initialize` / `shutdown`
- `textDocument/didOpen`, `didChange`, `didSave`, `didClose`
- `textDocument/completion` → completion menu
- `textDocument/hover` → info popup
- `textDocument/definition` → jump to definition
- `textDocument/formatting` → format buffer
- `textDocument/publishDiagnostics` → gutter markers + infobar messages

Position encoding: LSP uses UTF-16 offsets. `Buffer.Utf16Loc` / `Utf8Loc`
convert between internal byte offsets and LSP positions.

## TCL Commands

TCL (via `zyedidia/gotcl`) is the command language for ex-commands. Every
`:` command is a TCL procedure.

```go
func (e *Editor) initTCL() {
    e.interp = tcl.NewInterp()
    // Register editor commands
    e.interp.RegisterCommand("quit", e.cmdQuit)
    e.interp.RegisterCommand("write", e.cmdWrite)
    e.interp.RegisterCommand("edit", e.cmdEdit)
    e.interp.RegisterCommand("set", e.cmdSet)
    e.interp.RegisterCommand("split", e.cmdSplit)
    e.interp.RegisterCommand("vsplit", e.cmdVsplit)
    e.interp.RegisterCommand("tabnew", e.cmdTabNew)
    e.interp.RegisterCommand("lsp-hover", e.cmdLspHover)
    e.interp.RegisterCommand("lsp-def", e.cmdLspDef)
    e.interp.RegisterCommand("lsp-format", e.cmdLspFormat)
    // ...
}
```

Ex-command `:` prompt: enter command mode, read a line, evaluate via
`e.interp.Eval(line)`. Supports command-line completion for command names and
file paths.

Vim aliases: `:w` = `write`, `:q` = `quit`, `:e` = `edit`, `:sp` = `split`,
`:vs` = `vsplit`, `:s` = `substitute`.

## Splits and Tabs

### Split Tree

Binary tree of split nodes. Each leaf is a pane (buffer view).

```go
type SplitType uint8
const (
    SplitVert  SplitType = iota   // left | right
    SplitHoriz                     // top / bottom
)

type SplitNode struct {
    Kind     SplitType
    Parent   *SplitNode
    Children []*SplitNode   // len 0 for leaves, len 2 for splits
    X, Y     int            // position
    W, H     int            // dimensions
    PropW    float64        // proportional width (0-1)
    PropH    float64        // proportional height (0-1)
    ID       uint           // leaf node ID
}
```

### Tabs

```go
type Tab struct {
    root   *SplitNode
    panes  []Pane          // indexed by SplitNode.ID
    active uint            // ID of focused pane
}
```

### Pane Interface

All pane types (buffer, future terminal, etc.) implement:

```go
type Pane interface {
    Display(screen tcell.Screen, x, y, w, h int, theme *Theme)
    HandleKey(ev *tcell.EventKey)
    Resize(w, h int)
    Name() string
    Close()
}
```

For now, the only pane type is `BufPane` (a View + Buffer).

## Status Bar and Info Bar

**StatusBar** (one per pane, at the bottom of each split):
- Left: filename, modified flag, filetype, line ending
- Right: cursor position (line:col), selection info, encoding

**InfoBar** (one per editor, at the very bottom):
- Shows messages, errors, and the `:` command prompt.
- Supports single-line text input with completion.

## Dependencies

| Package | Purpose |
|---------|---------|
| `gdamore/tcell/v2` | Terminal I/O |
| `zyedidia/flare` | PEG-based syntax highlighting |
| `zyedidia/ftdetect` | Filetype detection |
| `zyedidia/gpeg` | PEG VM (used by flare) |
| `zyedidia/gotcl` | TCL interpreter |
| `zyedidia/uniseg` | Grapheme segmentation + display width |
| `zyedidia/glob` | Glob pattern matching (config) |
| `mattn/go-runewidth` | Character display width |
| `gogs/chardet` | Charset auto-detection |
| `pelletier/go-toml` | TOML parsing |
| `gopkg.in/yaml.v2` | YAML parsing (themes, LSP config) |
| `golang.org/x/text` | Encoding transforms |
| `golang.org/x/sync` | Semaphore (syntax highlighting) |
| `go.lsp.dev/protocol` | LSP type definitions (step 13) |
