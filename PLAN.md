# Mu Architecture Plan

## Overview

This document outlines architectural decisions for Mu, following a Neovim-inspired split between core engine (Go) and extensible features (Lua).

---

## Core Principles

### Go Core: Primitives and Performance

The Go core handles:
- Things called thousands of times per keystroke
- Fundamental data structures
- The "engine" that plugins build on

### Lua Plugins: Policy and Features

Lua handles features built on core primitives:
- Features that could reasonably be implemented by users
- Things that might evolve quickly or differ between users
- Logic that orchestrates core APIs

### Declarative Config: Static Settings

TOML/YAML for configuration that doesn't need code:
- Options, themes, keybindings (simple modes)
- Plugin manifests
- LSP server definitions

### Litmus Test

"Could a motivated user implement this as a plugin if we didn't ship it?"
- Yes → Lua (shipped as default plugin)
- No → Go core

---

## Component Split

### Go Core

| Component | Notes |
|-----------|-------|
| Buffer/rope, undo tree | Fundamental data structures |
| Cursors, marks, selections | Core editing state |
| Pane/split/tab management | Window layout |
| Display rendering | Performance critical |
| Event loop | Core infrastructure |
| Vim implementation | Stateful, performance-sensitive, core use case |
| TCL interpreter | Command bar execution |
| Lua runtime | Plugin infrastructure |
| LSP transport | JSON-RPC over stdio |
| Flare engine | Syntax highlighting, indent, fold |
| Clipboard | Platform-specific |
| Terminal emulator | Complex, performance-sensitive |
| File I/O | Core infrastructure |

### Declarative Config

| File | Purpose |
|------|---------|
| `options.toml` | Editor and buffer options |
| `bindings.toml` | Simple keybindings (micro mode, command bar) |
| `plugins.yaml` | Plugin manifest |
| `themes/*.yaml` | Color themes |
| `lsp.yaml` | LSP server definitions |
| `detectors/*.json` | Filetype detection rules |
| `highlighters/*.lang` | Flare syntax grammars |

### Lua (Shipped as Default Plugins)

| Feature | Notes |
|---------|-------|
| LSP client logic | Completion, hover, diagnostics, goto-def, etc. |
| Comment toggling | Simple buffer manipulation |
| Autoclose pairs | Insert-time hooks |
| Snippet expansion | Template insertion |
| Formatter/linter integration | Shell out, apply results |
| Git gutter | Shell out to git, display signs |
| Statusline customization | User-facing polish |
| File explorer enhancements | Beyond basic directory listing |

---

## Keybinding Architecture

### Remove PEG Grammar System

The current `.kbd` PEG-based system is being removed. It's over-engineered for simple bindings and insufficient for vim.

### Simple Bindings: TOML

For micro mode and command bar. Bindings map key sequences to TCL commands (same as typing in command bar).

#### Key Syntax (Kakoune-style, lowercase)

Modifiers (fully spelled out):
- `<ctrl-x>` - Control
- `<alt-x>` - Alt/Meta
- `<shift-x>` - Shift (for special keys)
- `<ctrl-shift-x>` - Combined modifiers

Special keys:
- `<ret>` or `<enter>` - Enter/Return
- `<esc>` - Escape
- `<space>` - Space
- `<tab>`, `<shift-tab>` - Tab
- `<backspace>` or `<bs>` - Backspace
- `<del>` - Delete
- `<up>`, `<down>`, `<left>`, `<right>` - Arrow keys
- `<home>`, `<end>` - Home/End
- `<pageup>`, `<pagedown>` - Page Up/Down
- `<f1>` through `<f12>` - Function keys

Regular characters are just themselves: `a`, `/`, `"`, etc.

Key sequences use multiple brackets: `<ctrl-x><ctrl-s>`

#### Example Bindings

```toml
[micro]
"<ctrl-s>" = "save"
"<ctrl-q>" = "quit"
"<ctrl-x><ctrl-s>" = "save"
"<ctrl-x><ctrl-c>" = "quit-all"
"<ctrl-f>" = "find-prompt"
"<ctrl-z>" = "undo"
"<ctrl-y>" = "redo"
"<alt-n>" = "tab-next"
"<alt-p>" = "tab-prev"
"<ctrl-shift-p>" = "command"

[command]
"<ret>" = "execute"
"<esc>" = "cancel"
"<tab>" = "complete"
"<shift-tab>" = "prev-completion"
"<up>" = "history-prev"
"<down>" = "history-next"
```

#### Implementation

Bindings are stored as a prefix map to support multi-key sequences:

```go
type Bindings struct {
    commands map[string]string   // full sequence -> TCL command
    prefixes map[string]bool     // partial sequences with continuations
}
```

Key dispatch:
1. Convert key event to string (e.g., `<ctrl-s>`)
2. Append to pending sequence
3. If full match → execute TCL command via `Editor.Eval()`
4. If prefix match → wait for more keys
5. Otherwise → reset, pass key through

### Vim Bindings: Go

Vim is implemented entirely in Go for:
- **Performance** - Every keystroke, no Lua overhead
- **Complexity** - Large stateful system (registers, macros, `.` repeat)
- **Core use case** - Goal is vim drop-in replacement
- **Debugging** - Go tooling beats Lua for complex state

Lua provides vim *customization*, not implementation:

```lua
-- User remaps
vim.map('n', 'H', '^')
vim.map('n', 'L', '$')

-- Custom text object
vim.textobj('ie', function(buf)
  return 0, buf:len()  -- entire buffer
end)
```

---

## Vim Implementation

### Goal

Drop-in replacement for Vim.

### Why Not Grammar-Based

Vim's behavior is deeply stateful:

| Feature | Requirement |
|---------|-------------|
| `.` repeat | Record actual changes, not keys |
| Counts | `3d2w` = 6 words, propagation |
| Registers | 26+ named, special (`"*`, `"+`, `"0`) |
| Macros | `qa...q`, `@a`, `@@` |
| Text objects | `ciw`, `da"`, `yi(` |
| Visual modes | `v`, `V`, `<C-v>` |
| Marks | Local and global |
| Jump list | `<C-o>`, `<C-i>` |
| Insert sub-modes | `<C-r>`, `<C-o>` |

### Package Structure

```
vim/
  vim.go       # State machine, key dispatch
  normal.go    # Normal mode
  insert.go    # Insert mode
  visual.go    # Visual modes
  operator.go  # Operator-pending (d, c, y, etc.)
  motion.go    # Motions (w, b, e, f, t, gg, G, etc.)
  textobj.go   # Text objects (iw, aw, i", etc.)
  register.go  # Register management
  macro.go     # Macro recording/playback
  repeat.go    # Dot repeat
  marks.go     # Mark storage
  jumplist.go  # Jump list
```

### Core State

```go
type Vim struct {
    mode       Mode
    count      int
    register   rune
    operator   Operator

    lastChange Change    // for . repeat
    recording  bool
    macros     map[rune][]Key
    macroReg   rune

    registers  map[rune][]byte
    marks      map[rune]Mark
    jumplist   *JumpList
}
```

### Editor Integration

Vim package returns actions, decoupled from editor:

```go
type Action interface{}

type MoveCursor struct { Pos int }
type Delete struct { Start, End int }
type Insert struct { Pos int; Text []byte }
type ChangeMode struct { Mode Mode }
type SetRegister struct { Reg rune; Text []byte }
```

---

## Flare Extensions

Flare handles syntax highlighting via PEG grammars. Planned extensions:

| Feature | Status |
|---------|--------|
| Syntax highlighting | Working |
| Auto-indentation | Planned |
| Code folding | Planned |

These remain declarative in `.lang` files. Lua calls `editor.reindent()` or `editor.fold()`, Flare does the work.

---

## Migration Path

### Phase 1: Keybindings
1. Implement TOML binding loader
2. Convert micro mode bindings to TOML
3. Remove `.kbd` files and PEG grammar code

### Phase 2: Vim
1. Create `vim/` package skeleton
2. Basic motions (h, j, k, l, w, b, e, 0, $, gg, G)
3. Operators (d, c, y) with motions
4. Counts
5. Insert mode
6. Visual modes
7. Text objects
8. Registers
9. `.` repeat
10. Macros
11. Marks and jump list

### Phase 3: Lua Plugin API
1. Define core API surface (buffer, cursor, window, editor)
2. Add hooks (onSave, onOpen, onBufEnter, onKey)
3. Move comment toggling to Lua
4. Move autoclose to Lua
5. Implement LSP client in Lua
6. Document plugin API

### Phase 4: Polish
1. Ship default plugins
2. Plugin repository/installation improvements
3. Documentation

---

## References

- Neovim: C core + Lua for LSP, treesitter queries, diagnostics
- Kakoune/Helix: Modal editing in code, not grammars
- Zed: Rust core + RPC plugins (future consideration for Mu)
