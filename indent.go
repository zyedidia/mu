package main

import "bytes"

// Indentation detection: a file already indented one way should keep being
// indented that way, whatever the configured default is, so that editing
// someone else's tab-indented file from a spaces-by-default setup does not
// mix the two. What the file says is only a default of its own, though —
// weaker than anything said about this specific file (see
// Editor.bufferOptions for the precedence).

// indentStyle is what a file's existing indentation says it uses.
type indentStyle int8

const (
	indentUnscanned indentStyle = iota // not looked at yet
	indentUnknown                      // looked at, nothing to go on
	indentSpaces
	indentTabs
)

// indentScanLines bounds the scan: a file's style is obvious from its first
// screens, and this runs on every open.
const indentScanLines = 400

// detectIndent reports how the buffer's existing lines are indented, by the
// character each indented line starts with. The majority wins; a file with
// no indented lines, or an even split, reports indentUnknown so the
// configured default stands.
func detectIndent(b *Buffer) indentStyle {
	tabs, spaces := 0, 0
	for line := 0; line <= b.NumLines() && line < indentScanLines; line++ {
		text := b.GetLine(line)
		if len(text) == 0 {
			continue
		}
		switch text[0] {
		case '\t':
			tabs++
		case ' ':
			// " * ..." is the continuation of a block comment lined up
			// under its opener, not an indented line.
			if rest := bytes.TrimLeft(text, " "); len(rest) > 0 && rest[0] == '*' {
				continue
			}
			spaces++
		}
	}
	switch {
	case tabs > spaces:
		return indentTabs
	case spaces > tabs:
		return indentSpaces
	}
	return indentUnknown
}

// IndentStyle returns the buffer's detected indentation, scanning its
// contents the first time it is asked. The answer is cached until the
// buffer is reloaded, since it is wanted every time options are resolved.
func (b *Buffer) IndentStyle() indentStyle {
	if b.indent == indentUnscanned {
		b.indent = detectIndent(b)
	}
	return b.indent
}
