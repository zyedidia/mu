package main

import (
	"strings"
)

// Macro recording and replay (vim q<reg> / @<reg>). Macros are stored in
// ordinary registers as key-notation text ("ihi<Esc>"), so "qp pastes a
// macro's keys and text yanked into a register can be executed with @.

// maxMacroDepth bounds nested macro replays so a self-referential macro
// (@q inside "q) terminates instead of recursing forever.
const maxMacroDepth = 100

// validMacroReg reports whether ch names a register usable with q/@.
func validMacroReg(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '"' || ch == '+' || ch == '*'
}

// keysToString serializes recorded keys as key notation parseable by
// ParseKeys. A literal '<' becomes <lt> so specials round-trip.
func keysToString(keys []string) string {
	var sb strings.Builder
	for _, k := range keys {
		if k == "<" {
			sb.WriteString("<lt>")
		} else {
			sb.WriteString(k)
		}
	}
	return sb.String()
}

// macroKeys parses register content into keys. Content that isn't valid key
// notation (text yanked from a buffer containing a stray '<') falls back to
// one key per rune.
func macroKeys(content []byte) []string {
	keys, err := ParseKeys(string(content))
	if err == nil {
		return keys
	}
	var raw []string
	for _, r := range string(content) {
		raw = append(raw, string(r))
	}
	return raw
}

// runMacro replays the macro in reg count times, routing each key the way a
// real keystroke is routed.
func (ks *KeyState) runMacro(reg RegisterID, count int) {
	content := ks.regs.Get(reg).Content
	if len(content) == 0 {
		return
	}
	keys := macroKeys(content)
	if ks.macroDepth >= maxMacroDepth {
		return
	}
	ks.macroDepth++
	defer func() { ks.macroDepth-- }()
	// The whole replay (including all count repetitions) is one undo step.
	b := ks.Buf()
	b.BeginUndoGroup()
	defer b.EndUndoGroup()
	// Keys played from a register are subject to mappings (as in vim),
	// even when the macro itself was started from inside a mapping
	// expansion (e.g. "vmap <Space> @q"), where remapping is suppressed.
	savedRemap := ks.remapping
	ks.remapping = false
	defer func() { ks.remapping = savedRemap }()
	// Drop the partial "@<reg>" dot-repeat recording: the actions inside
	// the macro record themselves, so . repeats the macro's last change,
	// as in vim.
	ks.recording = nil
	for i := 0; i < count; i++ {
		for _, k := range keys {
			ks.dispatchKey(k)
		}
	}
}

// runMacroLines replays the macro once per line of [sl, el], with the
// cursor placed at the start of each line (vim's visual-mode @). The end
// line tracks buffer growth or shrinkage caused by the macro.
func (ks *KeyState) runMacroLines(reg RegisterID, sl, el int) {
	b := ks.Buf()
	// The whole per-line application is one undo step.
	b.BeginUndoGroup()
	defer b.EndUndoGroup()
	total := b.LastLine()
	for l := sl; l <= el; l++ {
		if l > b.LastLine() {
			break
		}
		*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(l, 0))
		ks.runMacro(reg, 1)
		nt := b.LastLine()
		el += nt - total
		total = nt
	}
}

// resolveMacroChar maps the character following @ to a register: @@ repeats
// the last macro.
func resolveMacroChar(ks *KeyState, ch string) RegisterID {
	if ch == "@" {
		return ks.lastMacro
	}
	if len(ch) == 1 && validMacroReg(ch[0]) {
		return RegisterID(ch[0])
	}
	return 0
}

// RegisterMacros registers q (record) and @ (replay) bindings.
func RegisterMacros(ks *KeyState) {
	// q: start recording into a register, or stop a recording in progress.
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		if ks.macroReg != 0 {
			// Stop: the terminating q was recorded; strip it.
			keys := ks.macroRec
			if n := len(keys); n > 0 && keys[n-1] == "q" {
				keys = keys[:n-1]
			}
			ks.regs.Set(ks.macroReg, []byte(keysToString(keys)), false)
			ks.macroReg = 0
			ks.macroRec = nil
			ks.ResetAction()
			return
		}
		ks.WaitForChar(func(ks *KeyState, ch string) {
			ks.ClearCounts()
			if len(ch) == 1 && validMacroReg(ch[0]) {
				ks.macroReg = RegisterID(ch[0])
				ks.macroRec = nil
			} else {
				ks.ResetAction()
			}
		})
	}, "q")

	// @<reg>: replay a macro ([count]@<reg>, @@ repeats the last one).
	ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		ks.WaitForChar(func(ks *KeyState, ch string) {
			count := ks.Count()
			ks.ResetAction()
			reg := resolveMacroChar(ks, ch)
			if reg == 0 {
				return
			}
			ks.lastMacro = reg
			ks.runMacro(reg, count)
		})
	}, "@")

	// @<reg> in visual modes: exit visual and replay the macro once per
	// selected line (vim 8.1's v_@).
	for _, mode := range []ModeID{ModeVisual, ModeVisualLine, ModeVisualBlock} {
		ks.modes[mode].Bindings.Bind(func(ks *KeyState) {
			ks.WaitForChar(func(ks *KeyState, ch string) {
				ks.ResetAction()
				reg := resolveMacroChar(ks, ch)
				b := ks.Buf()
				c := b.Cursor()
				sl, el := -1, -1
				if c.HasSelection() {
					sl, _ = b.LineColAt(c.Sel[0])
					el, _ = b.LineColAt(c.Sel[1])
					if el > sl && b.OffsetAt(el, 0) == c.Sel[1] {
						el--
					}
				}
				for i := range b.cursors {
					b.cursors[i].ClearSelection()
				}
				ks.SetMode(ModeNormal)
				if reg == 0 || sl < 0 {
					return
				}
				ks.lastMacro = reg
				ks.runMacroLines(reg, sl, el)
			})
		}, "@")
	}
}
