package main

import k "github.com/zyedidia/kbd"

func keybinds() k.Pattern {
	move := k.Alt(
		k.Cap(k.MustLit("left"), "cursor-left $pos"),
		k.Cap(k.MustLit("right"), "cursor-right $pos"),
		k.Cap(k.MustLit("up"), "cursor-up $pos"),
		k.Cap(k.MustLit("down"), "cursor-down $pos"),
	)

	bindings := k.Alt(
		k.Cap(k.MustLit("ctrl+s"), "save"),
		k.Cap(k.MustLit("ctrl+q"), "quit"),
		k.Cap(k.MustLit("enter"), "insert-at $pos \"\n\""),
		k.Cap(k.MustLit("backspace"), "remove $pos [- $pos 1]; set pos [cursor-left $pos]"),
		k.Cap(k.MustLit("delete"), "remove $pos [+ $pos 1]"),
		k.Cap(k.AnyRune(), "insert-at $pos $0; set pos [cursor-right $pos]"),
		k.Cap(move, "set pos $1"),
	)

	return bindings
}
