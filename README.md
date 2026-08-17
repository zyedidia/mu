# Mu Text Editor

Mu is a simple terminal-based modal editor. I use it as a drop-in replacement
for Vim. It serves as a batteries-included editor configured to my preferences.
In terms of design, it is part-way between Micro and Vim.

It includes built-in support for LSP, autoclose, autocommenting, syntax
highlighting, visual block selection, multiple cursors, window state
persistence, splits, tabs, color themes, and more. It uses TCL for the
command-bar language and for small extensions. The core of the editor uses a
performant rope data structure and incremental syntax highlighting. Mu supports
a configuration directory at `~/.config/mu` (or at `XDG_CONFIG_HOME`). By
default it uses the configuration embedded into it in `embed/`.

The UI is kept minimal to reduce distractions. Markers only appear in the
single-column gutter next to line numbers, and messages only appear in the
command/info bar.

Press `Ctrl-P` for the searchable palette. It can find files, search file
contents, switch open buffers, and run editor commands. The modes are also
available directly as `:palette files`, `:palette text`, `:palette buffers`,
and `:palette commands`; use Up/Down to select and Enter to open.

Prebuilt binaries are included in the GitHub releases.

Building from source is easy: just run `go build`.
