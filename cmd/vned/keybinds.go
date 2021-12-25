package main

import k "github.com/zyedidia/kbd"

func microkeys() k.Pattern {
	move := k.Alt(
		k.Cap(k.MustLit("left"), "left [cursor-pos]"),
		k.Cap(k.MustLit("right"), "right [cursor-pos]"),
		k.Cap(k.MustLit("up"), "up [cursor-pos]"),
		k.Cap(k.MustLit("down"), "down [cursor-pos]"),
		k.Cap(k.MustLit("ctrl+down"), "size"),
		k.Cap(k.MustLit("ctrl+up"), "return 0"),
	)

	bindings := k.Alt(
		k.Cap(k.MustLit("ctrl+s"), "save"),
		k.Cap(k.MustLit("ctrl+q"), "quit"),
		k.Cap(k.MustLit("enter"), "insert-at [cursor-pos] \"\\n\"; move-to [right [cursor-pos]]"),
		k.Cap(k.MustLit("backspace"), "set char [left [cursor-pos]]; remove $char [cursor-pos]; move-to $char"),
		k.Cap(k.MustLit("delete"), "set char [right [cursor-pos]]; remove [cursor-pos] $char"),
		k.Cap(k.AnyRune(), "insert-at [cursor-pos] $0; move-to [right [cursor-pos]]"),
		k.Cap(move, "move-to [$1]"),
	)

	return bindings
}
