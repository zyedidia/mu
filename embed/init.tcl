# mu startup script.
#
# This is the built-in default. To customize, create init.tcl in the user
# config directory (~/.config/mu/init.tcl); it replaces this file entirely.
# Any ex command can be used here (set, map, ...).
#
# Key mapping commands (all mappings are non-recursive: the expansion
# always has its default meaning):
#
#   map <keys> <expansion>       normal, visual, and operator-pending modes
#   nmap / vmap / imap / omap    normal / visual / insert / operator-pending
#   unmap <keys>                 remove a mapping (nunmap etc. per mode)
#
# Special keys are written in angle brackets: <CR>, <Esc>, <Tab>, <BS>,
# <Del>, <Space>, <C-x>, <A-x>, <Up>, <Down>, <Left>, <Right>, <Home>,
# <End>, <PgUp>, <PgDn>, <S-Tab>, <lt> for a literal '<', and <Nop> to
# disable a key.

# Swap 0 and ^: 0 moves to the first non-blank character of the line and
# ^ moves to column 0.
map 0 ^
map ^ 0

# Move by display lines: with softwrap on, j/k go up and down one visual
# line (gj/gk are the unmapped originals). Mapped only in normal and visual
# modes so that operators keep buffer-line semantics (dj still deletes two
# whole lines).
nmap j gj
nmap k gk
vmap j gj
vmap k gk

# Space in visual mode applies the q macro to every selected line
# (record one with qq ... q, select lines, press space).
vmap <Space> @q

nmap <Space>a :palette actions<cr>
