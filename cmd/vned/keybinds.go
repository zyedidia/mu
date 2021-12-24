package main

import k "github.com/zyedidia/kbd"

func keybinds() k.Pattern {
	move := k.Alt(
		k.Cap(k.MustLit("left"), "cursor-left $pos"),
		k.Cap(k.MustLit("right"), "cursor-right $pos"),
		k.Cap(k.MustLit("up"), "cursor-up $pos"),
		k.Cap(k.MustLit("down"), "cursor-down $pos"),
		k.Cap(k.MustLit("ctrl+down"), "cursor-left [size]"),
		k.Cap(k.MustLit("ctrl+up"), "set pos 0"),
	)

	bindings := k.Alt(
		k.Cap(k.MustLit("ctrl+s"), "save"),
		k.Cap(k.MustLit("ctrl+q"), "quit"),
		k.Cap(k.MustLit("enter"), "insert-at $pos \"\\n\""),
		k.Cap(k.MustLit("backspace"), "set char [cursor-left $pos]; remove $char $pos; set pos $char"),
		k.Cap(k.MustLit("delete"), "set char [cursor-right $pos]; remove $pos $char"),
		k.Cap(k.AnyRune(), "insert-at $pos $0; set pos [cursor-right $pos]"),
		k.Cap(move, "set pos [$1]"),
	)

	return bindings
}
