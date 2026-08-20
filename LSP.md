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
**Command**: `:lsp-diagnostics` — palette listing every diagnostic across
all open buffers; selecting one jumps to it.

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

`=` is a real vim-style operator (registered through the same
`registerOperator` machinery as `d`/`c`/`y`/`>`/`<`), backed by
`textDocument/rangeFormatting` (gated on `DocumentRangeFormattingProvider`)
instead of whole-document formatting:

- `=` + motion or text object formats that range (`=ip`, `=G`, `=w`, with
  counts)
- `==` formats the current line, the doubled-operator convention (like
  `dd`/`yy`)
- In visual/visual-line mode, `=` formats the selection

Because formatting is a network round trip, the operator fires the request
and applies the resulting edit asynchronously, guarded by the same
buffer-version check `:lsp-format` uses (an edit landing on a since-changed
buffer is rejected rather than applied). One consequence: firing several
concurrent range-format requests (multi-cursor `=`) races that guard — only
the first response to land applies cleanly, and the rest are rejected as
"buffer changed" rather than applied to now-stale positions.

**Setting**: `autoformat` (buffer-scoped option, default `false` — see
`embed/options.toml`, settable per filetype/glob like any other option).
When on, every save (`:w`, `:wa`, `ZZ`, sudo save) runs `textDocument/formatting`
and applies the edits *before* writing, via `Buffer.beforeSave` (wired in
`configureView`). Unlike the async `:lsp-format`/`=`, this call is
synchronous (bounded by `lspRequestTimeout`) so the file on disk always
reflects the formatted content and `:wq` still saves before quitting — the
trade-off is a save briefly blocking on a slow formatter. A server that
doesn't support formatting is skipped silently; any other error is reported
but never blocks the save itself.

## Features — Tier 2 (Implemented)

### 8. Signature Help

`textDocument/signatureHelp` — shows function parameter info on demand.
Displayed in the infobar with the active parameter bracketed (`foo(a, [b
int])`); handles both the label-substring and UTF-16 offset-pair forms of
`ParameterInformation.Label`.

**Keybinding**: `<C-k>` in insert mode.
**Command**: `:lsp-signature`

### 9. Code Actions

`textDocument/codeAction` — quick fixes, refactors. Show a picker in the
infobar.

### 10. Rename

`textDocument/rename`. Prompts for a new name (prefilled with the word
under the cursor) and applies the resulting workspace edit via the same
path as code actions.

**Command**: `:lsp-rename`

### 11. Find References

`textDocument/references` — find all usages. Results open in the searchable
palette; selecting one jumps to it (opening the file first if needed).

**Keybinding**: `gr` in normal mode.
**Command**: `:lsp-references`

### 12. Document Symbols

`textDocument/documentSymbol` — list all symbols in the current file
(flattened from the hierarchical response, indented by nesting depth; the
flat legacy `SymbolInformation` shape is normalized to match). Opens in the
searchable palette for quick navigation.

**Command**: `:lsp-symbols`

## Features — Tier 3 (Implemented)

The vendored `go.lsp.dev/protocol` package (v0.12.0) predates LSP 3.17 and
has no inlay-hint types at all; `lsp_inlayhints.go` hand-rolls the wire
types it needs (mirroring the signature-help label workaround from Tier 2)
and tracks `inlayHintProvider` support separately from the typed
`ServerCapabilities`, since that struct has no field for it either.

### 13. Inlay Hints

`textDocument/inlayHint` — type annotations, parameter names.

- Not rendered inline in the buffer text
- An `i` marker is shown in the gutter for lines that have inlay hints
  (a diagnostic marker on the same line takes precedence)
- When the cursor is on that line, the hint text is displayed in the
  infobar

Hints are fetched for the whole document on demand; re-run the command
after edits to refresh them.

**Command**: `:lsp-inlay-hints`

### 14. Workspace Symbols

`workspace/symbol` — fuzzy search for symbols across all project files.
Prompts for a query, then opens a searchable palette of results; selecting
one jumps to it (opening the file first if needed). Like "go to symbol in
workspace".

**Command**: `:lsp-workspace-symbols`

### 15. Call Hierarchy

`textDocument/prepareCallHierarchy` resolves the item under the cursor
(the first, if several are returned), then `callHierarchy/incomingCalls` /
`outgoingCalls` lists what calls it or what it calls in a searchable
palette; selecting one jumps to that function's definition.

**Commands**: `:lsp-incoming-calls`, `:lsp-outgoing-calls`

## Features — Tier 4 (Not Planned)

Everything below is real LSP surface that mu does not implement, and there
are no plans to. Listed here so the gap is a deliberate, documented
decision rather than an oversight to rediscover later.

### 16. Prepare Rename

`textDocument/prepareRename` — validate the rename range before prompting.
Rename (#10) goes straight from the word under the cursor to the rename
request; a server that would reject the range is only discovered after the
fact, via the same error path as any other rejected request.

### 17. Semantic Tokens

`textDocument/semanticTokens/*` — server-driven syntax highlighting.
Existing highlighters (tree-sitter/regex based, from `flare`) already cover
this need.

### 18. Code Lens

`textDocument/codeLens` — inline actionable annotations (e.g. "run test",
reference counts) above symbols.

### 19. Folding Ranges

`textDocument/foldingRange` — server-provided fold regions, as an
alternative/addition to syntax-based folding.

### 20. Document Links

`textDocument/documentLink` — clickable links detected in source (e.g.
import paths, URLs in comments).

### 21. Dynamic Capability Registration

`client/registerCapability` / `unregisterCapability` are acknowledged with
a no-op reply (see `lsp.go`) so servers that use them don't block, but mu
never registers for anything dynamically — it relies solely on the static
capabilities advertised in `initialize`.

## Keybinding Summary

| Key | Mode | Action |
|-----|------|--------|
| `gd` | Normal | Go to definition |
| `K` | Normal | Hover documentation |
| `gr` | Normal | Find references |
| `]d` | Normal | Next diagnostic |
| `[d` | Normal | Previous diagnostic |
| `<C-space>` | Insert | Trigger completion |
| `<C-n>` | Completion | Next item |
| `<C-p>` | Completion | Previous item |
| `<C-k>` | Insert | Signature help |
| `=` + motion, `==` | Normal | Format range (operator, like `d`/`c`/`y`) |
| `=` | Visual, Visual Line | Format selection |

## Command Summary

| Command | Action |
|---------|--------|
| `:lsp-def` | Go to definition |
| `:lsp-hover` | Show hover info |
| `:lsp-format` | Format document |
| `:lsp-actions` | Show code actions |
| `:lsp-rename` | Rename symbol |
| `:lsp-references` | Find references |
| `:lsp-symbols` | Document symbols |
| `:lsp-signature` | Show signature help |
| `:lsp-diagnostics` | List diagnostics across open buffers |
| `:lsp-workspace-symbols` | Fuzzy search symbols across the workspace |
| `:lsp-incoming-calls` | List functions that call the symbol under cursor |
| `:lsp-outgoing-calls` | List functions called by the symbol under cursor |
| `:lsp-inlay-hints` | Show inlay hints in the gutter |

All of the palette-based commands above (`:lsp-actions`, `:lsp-references`,
`:lsp-symbols`, `:lsp-diagnostics`, `:lsp-workspace-symbols`,
`:lsp-incoming-calls`, `:lsp-outgoing-calls`) are also reachable from the
top-level searchable palette (`<C-p>`, or `:palette`), which lists them
alongside Files/Text/Buffers/Commands.

## Implementation Order

1. JSON-RPC transport + Server struct + Manager
2. Initialize handshake + document sync (didOpen/didChange/didSave/didClose)
3. Diagnostics (publishDiagnostics → gutter + infobar)
4. Go to definition (gd)
5. Hover (K)
6. Formatting (:lsp-format)
7. Completion (popup, basic text insertion)
