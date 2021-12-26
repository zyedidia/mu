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
		k.Cap(k.MustLit("ctrl+left"), "word-left [cursor-pos]"),
		k.Cap(k.MustLit("ctrl+right"), "word-right [cursor-pos]"),
		k.Cap(k.MustLit("alt+right"), "ws-right [cursor-pos]"),
		k.Cap(k.MustLit("alt+left"), "ws-left [cursor-pos]"),
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

func vimkeys() k.Pattern {
	any := k.Cap(k.AnyRune(), "$0")
	numq := k.Cap(k.Opt(
		k.Seq(
			k.RangeRune('1', '9'),
			k.Star(k.RangeRune('0', '9')),
		),
	), "$0")

	rmove := k.Alt(
		k.Cap(k.MustLit("h"), "left-vim"),
		k.Cap(k.MustLit("j"), "down"),
		k.Cap(k.MustLit("k"), "up"),
		k.Cap(k.MustLit("l"), "right-vim"),
		k.Cap(k.MustLit("w"), "word-right"),
		k.Cap(k.MustLit("W"), "ws-right"),
		k.Cap(k.MustLit("b"), "word-left"),
		k.Cap(k.MustLit("B"), "ws-left"),
		k.Cap(k.Seq(k.MustLit("f"), any), "find-char $1"),
		k.Cap(k.Seq(k.MustLit("F"), any), "find-char-back $1"),
		k.Cap(k.Seq(k.MustLit("t"), any), "till-char $1"),
		k.Cap(k.Seq(k.MustLit("T"), any), "till-char-back $1"),
	)

	move := k.Alt(
		k.Cap(k.MustLit("0"), "line-start [cursor-pos]"),
		k.Cap(k.MustLit("$"), "line-end [cursor-pos]"),
		k.Cap(k.Seq(numq, rmove), "repeat-move $1 {$2}"),
	)

	raction := k.Alt(
		k.Cap(k.Seq(k.MustLit("d"), move), "move-to [remove [cursor-pos] [$1]]"),
	)

	action := k.Alt(
		k.Cap(move, "move-to [$1]"),
		k.Cap(k.MustLit("ctrl+q"), "quit"),
		k.Cap(k.Seq(numq, raction), "repeat-fn $1 {$2}"),
	)

	return action
}
