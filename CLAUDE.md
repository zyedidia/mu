# MU Text Editor

MU is a complete rewrite of the [micro](https://github.com/zyedidia/micro) text editor, focusing on correctness, performance, and modern built-in features. Written in Go, it provides a terminal-based text editing experience with tabs, splits, syntax highlighting, LSP support, and extensibility via Lua plugins.

## Build & Run

```bash
# Build the editor
make build

# Install to $GOPATH/bin
make install

# Build with debug mode enabled
DEBUG=1 make build

# Fast build (skip version info injection)
FASTBUILD=1 make build

# Run tests
make test

# Run with coverage
make cover
```

### Build Tags

The build uses custom tags: `-tags flare_custom,ftdetect_custom`

## Project Structure

```
cmd/
  mu/           # Main entry point (mu.go, stats.go)
  mutxt/        # Text utility tool

buffer/         # Core text buffer implementation
  text/         # Low-level text storage
    linerope/   # Rope data structure with line indexing
    linecache/  # Line caching for performance
    cache/      # Generic caching utilities
    endings/    # Line ending detection (LF/CRLF)
  undo/         # Undo tree implementation
  diff/         # Text diffing for SetContent operations
  cursor.go     # Cursor and selection handling
  buffer.go     # Main Buffer struct
  save.go       # File saving logic
  search.go     # Search/replace functionality
  lsp.go        # LSP integration for buffers

pane/           # View/display layer
  buf/          # Buffer pane (main editing view)
    commands.go # Editor commands (insert, delete, move, etc.)
    display.go  # Rendering logic
    cursor.go   # Cursor display and movement
    lsp.go      # LSP features (completion, hover, etc.)
    search.go   # Search UI
    edit.go     # Text editing operations
  term/         # Terminal emulator pane
  dir/          # Directory browser pane
  info/         # Info/command bar pane
  pane.go       # Pane interface definition

config/         # Configuration system
  embed/        # Embedded default configs
    bindings/   # Keybinding files (*.kbd)
    themes/     # Color themes (*.yaml)
    highlighters/ # Syntax definitions (*.lang)
    detectors/  # Filetype detection (*.json)
    options.toml # Default options

lsp/            # Language Server Protocol
  manager.go    # LSP server lifecycle management
  server.go     # LSP server communication
  requests.go   # LSP request handling
  notifications.go # LSP notification handling

plugin/         # Plugin system
  manager.go    # Plugin loading and management

lua/            # Lua scripting runtime
  lua.go        # Go standard library bindings for Lua

pkg/            # Utility packages
  theme/        # Theme/style handling
  input/        # File input abstraction (mmap support)
  output/       # File output abstraction
  tclutil/      # TCL interpreter utilities
  shell/        # Shell command execution
  completer/    # Completion utilities
  uniseg/       # Unicode segmentation
  grapheme/     # Grapheme cluster handling

split/          # Split/pane layout management
```

## Core Architecture

### Event Loop

The main event loop (`cmd/mu/mu.go`) uses tcell for terminal handling:
1. Poll events from tcell in a goroutine
2. Process events through `Editor.HandleEvent()`
3. Redraw on the `Editor.Redraw` channel
4. Handle errors, suspend/resume signals

### Editor Struct (`editor.go`)

Central orchestrator that manages:
- `tabs []*Tab` - Tab list with split panes
- `mode stack.Stack[*kbd.Config]` - Keyboard mode stack (micro, vim-normal, vim-insert, etc.)
- `interp *tcl.Interp` - TCL interpreter for command execution
- `lsp *lsp.Manager` - LSP server manager
- `clipboard` - System clipboard integration
- `config *config.ConfigFS` - Configuration filesystem

### Keybinding System

Uses a custom PEG-based grammar (`*.kbd` files) compiled by `github.com/zyedidia/kbd`:
- Bindings map key sequences to TCL commands
- Supports modes: `micro`, `vim-normal`, `vim-insert`, `vim-visual`, `cmd`, `term`, `complete`
- Mode stack allows pushing/popping modes

Example from `vim-normal.kbd`:
```
action <- { 'i',  'set-mode vim-insert' }
        / { 'dd', 'remove-range [line-start [cursor-pos]] [next-line-start [cursor-pos]]' }
```

### Buffer System

`Buffer` wraps `BufferData` which contains:
- `*text.Buffer` - Underlying rope-based text storage
- `undo *undo.UndoTree` - Serializable undo tree with branching
- `highlighter *flare.Highlighter` - Memoized syntax highlighting
- `Lsp *lsp.Server` - Active LSP connection

Key features:
- Rope data structure for O(log n) edits
- Parallel chunk loading for large files
- Line caching for fast line access
- Cursor positions stored as byte offsets

### Cursor and Selection

```go
type Cursor struct {
    Pos         int       // Byte offset in buffer
    HasSel      bool      // Selection active
    Sel         [2]int    // Selection range [start, end]
    Orig        [2]int    // Original selection anchor
    Vx          int       // Visual X for vertical movement
}
```

### Tab/Split Layout

- `Tab` contains a tree of split nodes (`split.Node`)
- Each leaf node holds a `pane.Pane` (buffer, terminal, or directory)
- `splitpane` associates a pane with its split node ID

### Command Execution

1. Key event matches binding in current mode
2. TCL command string extracted from binding
3. `Editor.Eval()` executes TCL with registered commands
4. Commands call methods on `Editor` or active `BufPane`

## Configuration

### Location

Config directory: `~/.config/mu/` (via XDG)

### Options (`options.toml`)

```toml
autoindent = true
cursor = "block"        # block, bar, underline (+ blinking variants)
theme = "monokai"
syntax = true
tabsize = 4
tabstospaces = true
linenums = true
softwrap = false
mode = "micro"          # Default keybinding mode
clipboard = "external"  # internal, external, terminal

# Per-filetype overrides
[makefile]
tabstospaces = false

# Glob patterns
["glob:*.md"]
softwrap = true
```

### Global Options

Only these apply editor-wide: `theme`, `clipboard`, `cursor`

### LSP Configuration (`lsp.yaml`)

```yaml
go:
  command: gopls
  args: []
python:
  command: pylsp
  args: []
```

## Key Modules

### Text Storage (`buffer/text/linerope/`)

Rope implementation with:
- `SplitLen = 4096` - Split threshold
- `JoinLen = 2048` - Join threshold
- Parallel construction using all CPU cores
- Line-indexed for fast line lookups

### Undo System (`buffer/undo/`)

Tree-based undo with:
- Branching (multiple redo paths)
- Time-based coalescing (edits within 1s merged)
- Serialization to disk (gzip + gob)
- Optional cutoff to limit history size

### Syntax Highlighting

Uses `github.com/zyedidia/flare`:
- PEG-based grammar definitions
- Memoized incremental highlighting
- Runs in background goroutine

### Plugin System

Lua-based (`plugin/manager.go`):
- Plugins defined in `~/.config/mu/plugins.yaml`
- Init script: `~/.config/mu/init.lua`
- Exposes Go stdlib via `import()`: fmt, os, strings, regexp, etc.

## Adding New Features

### Adding an Editor Command

1. Define in `commands.go` (root package):
```go
commands = append(commands, tclutil.Command{
    Name: "my-command",
    Fn:   (*Editor).MyCommand,
    Doc:  "my-command: description",
})

func (e *Editor) MyCommand(args []string) error {
    // Implementation
}
```

### Adding a Buffer/Pane Command

1. Add to `pane/buf/commands.go`:
```go
var commands = []tclutil.Command{
    {Name: "my-buf-cmd", Fn: (*BufPane).MyBufCmd, Doc: "...", Relocate: true},
}
```

### Adding a Configuration Option

1. Add default in `config/embed/options.toml`
2. For global options, add to `globals` map in `config/options.go`
3. For validation, add to `verify` map

## Debugging

### Logs

Logs written to `/tmp/mu.log`

### CPU Profiling

```bash
./mu -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

### Performance Stats

```bash
./mu -stats
```

## Key Dependencies

- `github.com/micro-editor/tcell/v2` - Terminal handling
- `github.com/zyedidia/flare` - Syntax highlighting
- `github.com/zyedidia/ftdetect` - Filetype detection
- `github.com/zyedidia/kbd` - Keybinding DSL
- `github.com/zyedidia/gotcl` - TCL interpreter
- `github.com/yuin/gopher-lua` - Lua runtime
- `go.lsp.dev/protocol` - LSP types
- `github.com/zyedidia/clipper` - Clipboard

## Development Focus Areas

Current priorities:
- **Vim keybindings** - Partial support in `vim-normal.kbd`, `vim-insert.kbd`, `vim-visual.kbd`
- **LSP support** - Basic functionality working (completion, diagnostics); needs expansion
- **Configurability** - Layered config system with filetype/glob overrides

Known limitations (from codebase):
- Multicursor support incomplete
- Vim repeat (`.`) not implemented
- LSP features like rename, references need work
