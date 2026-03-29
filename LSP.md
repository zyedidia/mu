# LSP Support Plan

## Architecture

### Transport (`lsp.go`)

JSON-RPC 2.0 over stdin/stdout of the language server subprocess.

- **Framing**: `Content-Length: N\r\n\r\n{json}` (standard LSP header)
- **Requests**: client assigns an incrementing integer ID, creates a
  `chan json.RawMessage` in a map, sends the request, waits on the channel
  with a timeout
- **Responses**: background goroutine reads stdout, checks if the message
  has an ID (response) or a method (notification), routes accordingly
- **Notifications**: fire-and-forget messages in both directions

### Server Lifecycle

```
Manager (one per editor)
  └─ servers map[string]*LspServer   (one per language, keyed by filetype)

LspServer
  ├─ cmd *exec.Cmd                   (the subprocess)
  ├─ stdin io.WriteCloser
  ├─ stdout *bufio.Reader
  ├─ capabilities ServerCapabilities  (from initialize response)
  ├─ nextID int
  ├─ responses map[int]chan json.RawMessage
  └─ lock sync.Mutex
```

Servers are lazy-started: when a buffer is opened with a filetype that has an
entry in `lsp.yaml`, the manager starts the server, performs the `initialize`
handshake, and sends `initialized`. The server is shut down when the editor
exits.

### Buffer Integration

When a buffer opens:
1. Detect filetype
2. If LSP config exists for that filetype, call `Manager.Open(ft, path, content, version)`
3. Store `*LspServer` reference on the Buffer
4. Send `textDocument/didOpen`

On every edit: send `textDocument/didChange` (incremental, with version counter)
On save: send `textDocument/didSave`
On close: send `textDocument/didClose`

### Position Encoding

LSP uses UTF-16 code unit offsets. We store byte offsets internally.
`Buffer.Utf16Loc` and `Buffer.Utf8Loc` (already implemented in unicode.go)
handle the conversion.

## Features — Tier 1 (Implement Now)

### 1. Initialize / Shutdown

- Send `initialize` with client capabilities (completion, hover, definition,
  formatting, diagnostics)
- Receive server capabilities and store them
- Send `initialized` notification
- On editor exit: send `shutdown` request, then `exit` notification

### 2. Document Synchronization

- `textDocument/didOpen` — on buffer open (full content)
- `textDocument/didChange` — on every edit (incremental changes with version)
- `textDocument/didSave` — on `:w`
- `textDocument/didClose` — on buffer close

### 3. Diagnostics

Server pushes `textDocument/publishDiagnostics` with errors/warnings.

- Store diagnostics on the Buffer (we already have `Diagnostic` type and
  `AddDiagnostic`/`ClearDiagnostics`/`GetDiagnosticAt`)
- Show in gutter: `>` marker with error/warning style (already implemented
  in `view.go`)
- Show diagnostic text in infobar when cursor is on the affected line

**Keybinding**: `]d` / `[d` to jump to next/previous diagnostic.

### 4. Go to Definition

Request `textDocument/definition` at cursor position. Returns one or more
`Location` results.

- Same file: move cursor to the target position
- Different file: open the file and jump to position

**Keybinding**: `gd` in normal mode.
**Command**: `:lsp-def`

### 5. Hover

Request `textDocument/hover` at cursor position. Returns markdown/plaintext
documentation.

- Display in infobar (stripped to single line, newlines replaced with spaces)
- For multi-line content, show first meaningful line

**Keybinding**: `K` in normal mode.
**Command**: `:lsp-hover`

### 6. Completion

Request `textDocument/completion` at cursor position. Returns a list of
completion items.

- Trigger: manual via a keybinding, or automatic on trigger characters
  (`.`, `::`, etc. as reported by server capabilities)
- Display: show completion list in a popup or overlay near the cursor
- Navigation: `Ctrl-N` / `Ctrl-P` (or `Tab` / `Shift-Tab`) to cycle
- Accept: `Enter` or `Tab` inserts the selected item
- Cancel: `Escape` dismisses
- No snippet support initially (plain text insertions only)

**Keybinding**: `<C-x><C-o>` or `<C-space>` in insert mode.

### 7. Formatting

Request `textDocument/formatting` for the whole document. Returns a list of
`TextEdit` values.

- Apply edits via `Buffer.DoEdit` (preserves undo)
- Position encoding conversion via Utf16/Utf8 helpers

**Command**: `:lsp-format`

## Features — Tier 2 (Add Later)

### 8. Signature Help

`textDocument/signatureHelp` — shows function parameter info while typing.
Display in infobar with the active parameter highlighted.

### 9. Code Actions

`textDocument/codeAction` — quick fixes, refactors. Show a picker in the
infobar.

### 10. Rename

`textDocument/rename` with optional `textDocument/prepareRename`. Prompt for
new name, apply workspace edits.

**Command**: `:lsp-rename`

### 11. Find References

`textDocument/references` — find all usages. Show results in a picker or
jump through with keybindings.

**Keybinding**: `gr` in normal mode.

### 12. Document Symbols

`textDocument/documentSymbol` — list all symbols in the current file. Show
in a picker for quick navigation.

**Command**: `:lsp-symbols`

## Features — Tier 3 (Nice to Have)

### 13. Inlay Hints

`textDocument/inlayHint` — type annotations, parameter names.

- Do NOT render inline in the buffer text
- Show a `i` marker in the gutter for lines that have inlay hints
- When the cursor is on that line, display the hint text in the infobar

### 14. Workspace Symbols

`workspace/symbol` — fuzzy search for symbols across all project files.
Opens a picker with results. Like "go to symbol in workspace".

**Command**: `:lsp-workspace-symbols`

### 15. Call Hierarchy

`callHierarchy/incomingCalls` / `outgoingCalls` — show what calls a
function (incoming) or what it calls (outgoing). Tree-style display in a
picker.

## Keybinding Summary

| Key | Mode | Action |
|-----|------|--------|
| `gd` | Normal | Go to definition |
| `K` | Normal | Hover documentation |
| `gr` | Normal | Find references (tier 2) |
| `]d` | Normal | Next diagnostic |
| `[d` | Normal | Previous diagnostic |
| `<C-space>` | Insert | Trigger completion |
| `<C-n>` | Completion | Next item |
| `<C-p>` | Completion | Previous item |

## Command Summary

| Command | Action |
|---------|--------|
| `:lsp-def` | Go to definition |
| `:lsp-hover` | Show hover info |
| `:lsp-format` | Format document |
| `:lsp-rename` | Rename symbol (tier 2) |
| `:lsp-symbols` | Document symbols (tier 2) |
| `:lsp-workspace-symbols` | Workspace symbols (tier 3) |

## Implementation Order

1. JSON-RPC transport + Server struct + Manager
2. Initialize handshake + document sync (didOpen/didChange/didSave/didClose)
3. Diagnostics (publishDiagnostics → gutter + infobar)
4. Go to definition (gd)
5. Hover (K)
6. Formatting (:lsp-format)
7. Completion (popup, basic text insertion)
