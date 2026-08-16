package main

import (
	"fmt"
	"strings"
	"unicode"
)

// Key mapping (:map and friends, usable from init.tcl). Mappings live in
// each Mode's Remaps trie and are consulted before the default bindings.
// The expansion is replayed with mapping disabled (vim noremap semantics),
// so mutual mappings like "map 0 ^" / "map ^ 0" cannot recurse.

// mapModeSets names the mode set each mapping command operates on.
var mapModeSets = map[string][]ModeID{
	"map":  {ModeNormal, ModeVisual, ModeVisualLine, ModeVisualBlock, ModeOperatorPending},
	"nmap": {ModeNormal},
	"vmap": {ModeVisual, ModeVisualLine, ModeVisualBlock},
	"imap": {ModeInsert},
	"omap": {ModeOperatorPending},
}

// makeMapCmd returns the command function for a mapping command covering the
// given modes.
func makeMapCmd(modes []ModeID) func(*Editor, []string) error {
	return func(e *Editor, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("usage: map <keys> <expansion>")
		}
		lhs, err := ParseKeys(args[0])
		if err != nil {
			return err
		}
		rhs, err := ParseKeys(strings.Join(args[1:], " "))
		if err != nil {
			return err
		}
		if len(lhs) == 0 || len(rhs) == 0 {
			return fmt.Errorf("map: empty key sequence")
		}
		for _, id := range modes {
			e.ks.modes[id].Remaps.Bind(func(ks *KeyState) {
				ks.replayKeys(rhs)
			}, lhs...)
		}
		return nil
	}
}

// makeUnmapCmd returns the command function for an unmapping command
// covering the given modes.
func makeUnmapCmd(modes []ModeID) func(*Editor, []string) error {
	return func(e *Editor, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("usage: unmap <keys>")
		}
		lhs, err := ParseKeys(args[0])
		if err != nil {
			return err
		}
		removed := false
		for _, id := range modes {
			if e.ks.modes[id].Remaps.Unbind(lhs...) {
				removed = true
			}
		}
		if !removed {
			return fmt.Errorf("unmap: no mapping for %q", args[0])
		}
		return nil
	}
}

// ParseKeys converts vim-style key notation into the internal key strings
// used by the dispatcher: plain characters are single keys, and special
// keys are written in angle brackets ("<C-x>", "<Esc>", "<Space>"). An
// unterminated '<' is a literal character.
func ParseKeys(s string) ([]string, error) {
	var keys []string
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '<' {
			keys = append(keys, string(rs[i]))
			continue
		}
		end := -1
		for j := i + 1; j < len(rs); j++ {
			if rs[j] == '>' {
				end = j
				break
			}
		}
		if end < 0 {
			keys = append(keys, "<")
			continue
		}
		key, err := normalizeKeyName(string(rs[i+1 : end]))
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
		i = end
	}
	return keys, nil
}

// keyNames maps lowercased angle-bracket key names to internal keys.
var keyNames = map[string]string{
	"esc":       KeyEscape,
	"escape":    KeyEscape,
	"cr":        KeyEnter,
	"enter":     KeyEnter,
	"return":    KeyEnter,
	"bs":        KeyBacksp,
	"backspace": KeyBacksp,
	"tab":       KeyTab,
	"del":       KeyDelete,
	"delete":    KeyDelete,
	"up":        KeyUp,
	"down":      KeyDown,
	"left":      KeyLeft,
	"right":     KeyRight,
	"home":      KeyHome,
	"end":       KeyEnd,
	"pgup":      KeyPgUp,
	"pageup":    KeyPgUp,
	"pgdn":      KeyPgDn,
	"pagedown":  KeyPgDn,
	"s-tab":     "<S-Tab>",
	"c-space":   "<C-space>",
	"space":     " ",
	"lt":        "<",
	// <Nop> expands to a key bound nowhere, so mapping to it disables the
	// left-hand side.
	"nop": "<Nop>",
}

// normalizeKeyName converts the inside of an angle-bracket key name to the
// internal key representation.
func normalizeKeyName(name string) (string, error) {
	lower := strings.ToLower(name)
	if k, ok := keyNames[lower]; ok {
		return k, nil
	}
	// Modifier forms: <C-x> (control, lowercased) and <A-x>/<M-x> (alt).
	if len(lower) > 2 && lower[1] == '-' {
		rest := []rune(name[2:])
		switch lower[0] {
		case 'c':
			if len(rest) == 1 {
				r := unicode.ToLower(rest[0])
				if r >= 'a' && r <= 'z' {
					return fmt.Sprintf("<C-%c>", r), nil
				}
			}
		case 'a', 'm':
			if len(rest) == 1 {
				return fmt.Sprintf("<A-%c>", rest[0]), nil
			}
		}
	}
	return "", fmt.Errorf("unknown key <%s>", name)
}
