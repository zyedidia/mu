package main

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// InfoBar is the command/message bar at the bottom of the editor.
type InfoBar struct {
	// Message display.
	message    string
	msgErr     bool
	showCursor bool // show cursor at end of message (for y/n prompts)

	// Command-line input state.
	active    bool
	prompt    string // e.g. ":", "/", "?"
	input     []rune
	cursorPos int
	callback  func(input string) // called on Enter
	onChange  func(input string) // called on each input change (incremental search)
	onCancel  func()             // called on Escape (restore state)

	// Single-key prompt state (for y/n prompts).
	charCallback func(key string)

	// Tab completion.
	completer  Completer
	completion completionState

	// Command history (per prompt type: ":", "/", "?").
	history   map[string][]string
	histIndex int    // -1 = editing new input, 0..n = browsing history
	histSaved []rune // the input being edited before browsing history
}

// NewInfoBar creates a new info bar.
func NewInfoBar() *InfoBar {
	return &InfoBar{
		history:   make(map[string][]string),
		histIndex: -1,
	}
}

// Message shows a message in the info bar.
func (ib *InfoBar) Message(msg string) {
	ib.message = msg
	ib.msgErr = false
	ib.showCursor = false
}

// Prompt shows a message and waits for a single keypress, which is passed
// to the callback. The prompt closes automatically after one key.
func (ib *InfoBar) Prompt(msg string, cb func(key string)) {
	ib.message = msg
	ib.msgErr = false
	ib.showCursor = true
	ib.charCallback = cb
}

// Error shows an error message in the info bar.
func (ib *InfoBar) Error(msg string) {
	ib.message = msg
	ib.msgErr = true
}

// Clear clears the message.
func (ib *InfoBar) Clear() {
	ib.message = ""
	ib.msgErr = false
}

// IsActive returns true if any input (command-line or single-key prompt) is active.
func (ib *InfoBar) IsActive() bool {
	return ib.active || ib.charCallback != nil
}

// SetCompleter sets the tab-completion function for the active prompt.
func (ib *InfoBar) SetCompleter(c Completer) {
	ib.completer = c
	ib.completion.reset()
}

// StartPrompt activates command-line input with the given prompt character.
// The callback is called with the entered text when the user presses Enter.
func (ib *InfoBar) StartPrompt(prompt string, cb func(input string)) {
	ib.active = true
	ib.prompt = prompt
	ib.input = nil
	ib.cursorPos = 0
	ib.callback = cb
	ib.onChange = nil
	ib.onCancel = nil
	ib.completer = nil
	ib.completion.reset()
	ib.histIndex = -1
	ib.histSaved = nil
	ib.message = ""
}

// StartPromptIncremental activates command-line input with callbacks for
// incremental updates. onChange is called after every keystroke that modifies
// the input. onCancel is called when the user presses Escape.
func (ib *InfoBar) StartPromptIncremental(prompt string, onChange func(string), onDone func(string), onCancel func()) {
	ib.active = true
	ib.prompt = prompt
	ib.input = nil
	ib.cursorPos = 0
	ib.callback = onDone
	ib.onChange = onChange
	ib.onCancel = onCancel
	ib.completer = nil
	ib.completion.reset()
	ib.histIndex = -1
	ib.histSaved = nil
	ib.message = ""
}

// Cancel deactivates the command-line input.
func (ib *InfoBar) Cancel() {
	onCancel := ib.onCancel
	ib.active = false
	ib.input = nil
	ib.cursorPos = 0
	ib.callback = nil
	ib.onChange = nil
	ib.onCancel = nil
	ib.completer = nil
	ib.completion.reset()
	ib.histIndex = -1
	ib.histSaved = nil
	if onCancel != nil {
		onCancel()
	}
}

// HandleKey processes a key event while the command-line is active.
// Returns true if the editor should redraw, false if the prompt closed.
func (ib *InfoBar) HandleKey(key string) (redraw bool, done bool) {
	// Single-key prompt: deliver the key and close.
	if ib.charCallback != nil {
		cb := ib.charCallback
		ib.charCallback = nil
		ib.showCursor = false
		ib.message = ""
		cb(key)
		return true, true
	}

	changed := false

	switch key {
	case KeyTab:
		ib.tabComplete(1)
		return true, false
	case "<S-Tab>":
		ib.tabComplete(-1)
		return true, false
	case KeyEscape:
		if ib.completion.active {
			ib.completion.reset()
			return true, false
		}
		ib.Cancel()
		return true, true
	case "<C-c>":
		ib.completion.reset()
		ib.Cancel()
		return true, true
	case KeyEnter:
		input := string(ib.input)
		// Save non-empty input to history for this prompt type.
		if input != "" {
			h := ib.history[ib.prompt]
			// Avoid consecutive duplicates.
			if len(h) == 0 || h[len(h)-1] != input {
				ib.history[ib.prompt] = append(h, input)
			}
		}
		cb := ib.callback
		ib.active = false
		ib.callback = nil
		ib.onChange = nil
		ib.onCancel = nil
		ib.completer = nil
		ib.completion.reset()
		ib.histIndex = -1
		ib.histSaved = nil
		// Reset the input BEFORE running the callback: a callback may open
		// the next prompt with prefilled input (rename), which a reset
		// afterwards would wipe.
		ib.input = nil
		ib.cursorPos = 0
		if cb != nil {
			cb(input)
		}
		return true, true
	case KeyUp, "<C-p>":
		ib.historyNav(-1)
		return true, false
	case KeyDown, "<C-n>":
		ib.historyNav(1)
		return true, false
	case KeyBacksp:
		if ib.cursorPos > 0 {
			ib.input = append(ib.input[:ib.cursorPos-1], ib.input[ib.cursorPos:]...)
			ib.cursorPos--
			changed = true
		} else if len(ib.input) == 0 {
			ib.Cancel()
			return true, true
		}
	case KeyLeft:
		if ib.cursorPos > 0 {
			ib.cursorPos--
		}
	case KeyRight:
		if ib.cursorPos < len(ib.input) {
			ib.cursorPos++
		}
	case KeyHome, "<C-a>":
		ib.cursorPos = 0
	case KeyEnd, "<C-e>":
		ib.cursorPos = len(ib.input)
	case "<C-w>":
		// Delete word before cursor.
		if ib.cursorPos > 0 {
			start := ib.cursorPos
			// Skip trailing spaces.
			for start > 0 && ib.input[start-1] == ' ' {
				start--
			}
			// Skip word chars.
			for start > 0 && ib.input[start-1] != ' ' {
				start--
			}
			ib.input = append(ib.input[:start], ib.input[ib.cursorPos:]...)
			ib.cursorPos = start
			changed = true
		}
	case "<C-u>":
		// Delete to start of line.
		if ib.cursorPos > 0 {
			ib.input = ib.input[ib.cursorPos:]
			ib.cursorPos = 0
			changed = true
		}
	default:
		if len(key) == 0 || (len(key) > 1 && key[0] == '<') {
			return false, false
		}
		rs := []rune(key)
		tail := make([]rune, len(ib.input[ib.cursorPos:]))
		copy(tail, ib.input[ib.cursorPos:])
		ib.input = append(ib.input[:ib.cursorPos], append(rs, tail...)...)
		ib.cursorPos += len(rs)
		changed = true
	}

	if changed {
		ib.completion.reset()
		if ib.onChange != nil {
			ib.onChange(string(ib.input))
		}
	}
	return true, false
}

// historyNav navigates through command history. dir is -1 for older (Up)
// or +1 for newer (Down).
//
// histIndex: -1 = editing new input, 0 = most recent, 1 = next older, etc.
func (ib *InfoBar) historyNav(dir int) {
	h := ib.history[ib.prompt]
	if len(h) == 0 {
		return
	}
	if ib.histIndex == -1 && dir > 0 {
		return // already at current input
	}
	if ib.histIndex == -1 {
		// Save current input before browsing.
		ib.histSaved = make([]rune, len(ib.input))
		copy(ib.histSaved, ib.input)
	}

	// Up = older = increment index; Down = newer = decrement.
	newIndex := ib.histIndex - dir
	if newIndex >= len(h) {
		return // at oldest
	}
	if newIndex < -1 {
		newIndex = -1
	}

	ib.histIndex = newIndex
	if newIndex == -1 {
		ib.input = ib.histSaved
	} else {
		ib.input = []rune(h[len(h)-1-newIndex])
	}
	ib.cursorPos = len(ib.input)
	ib.completion.reset()
	// Recalled input counts as an input change (incremental search).
	if ib.onChange != nil {
		ib.onChange(string(ib.input))
	}
}

// tabComplete performs one step of tab completion. dir is +1 for forward
// (Tab) or -1 for backward (Shift-Tab).
//
// The cycle includes the original (uncompleted) input as index -1, then
// candidates at indices 0..n-1. First Tab goes to candidate 0 immediately.
func (ib *InfoBar) tabComplete(dir int) {
	if ib.completer == nil {
		return
	}

	cs := &ib.completion
	if !cs.active {
		// First Tab: compute candidates and save original input.
		input := string(ib.input)
		cs.candidates = ib.completer(input)
		if len(cs.candidates) == 0 {
			return
		}
		word := lastWord(input)
		cs.replaceFrom = len([]rune(input)) - len([]rune(word))
		cs.origWord = word
		cs.index = -1 // -1 = original input
		cs.active = true
	}

	if len(cs.candidates) == 0 {
		return
	}

	// Advance: cycle through -1 (original), 0, 1, ..., n-1, back to -1.
	total := len(cs.candidates) + 1 // +1 for original
	// Map index from [-1, n-1] to [0, n], advance, map back.
	pos := cs.index + 1 // now [0, n]
	pos = (pos + dir + total) % total
	cs.index = pos - 1 // back to [-1, n-1]

	// Replace the word portion with the selected candidate or original.
	var replacement string
	if cs.index == -1 {
		replacement = cs.origWord
	} else {
		replacement = cs.candidates[cs.index]
	}
	prefix := ib.input[:cs.replaceFrom]
	newInput := append([]rune{}, prefix...)
	newInput = append(newInput, []rune(replacement)...)
	ib.input = newInput
	ib.cursorPos = len(ib.input)
}

// longestCommonPrefix returns the longest string that is a prefix of all
// candidates.
func longestCommonPrefix(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	lcp := candidates[0]
	for _, c := range candidates[1:] {
		for i := 0; i < len(lcp); i++ {
			if i >= len(c) || lcp[i] != c[i] {
				lcp = lcp[:i]
				break
			}
		}
	}
	return lcp
}

// lastWord returns the last space-separated word in s, or all of s if
// there are no spaces.
func lastWord(s string) string {
	i := strings.LastIndexByte(s, ' ')
	if i == -1 {
		return s
	}
	return s[i+1:]
}

// HasCompletions returns true if there are active completion candidates to show.
func (ib *InfoBar) HasCompletions() bool {
	return ib.completion.active && len(ib.completion.candidates) > 0
}

// DrawCompletions renders the completion candidates on the given screen row.
// The selected candidate is shown in brackets. Horizontally scrolls so the
// selected item is always visible.
func (ib *InfoBar) DrawCompletions(screen tcell.Screen, y, w int, th *Theme) {
	cs := &ib.completion
	if !cs.active || len(cs.candidates) == 0 {
		return
	}

	style := th.Default().Add(AttrReverse)
	ts := style.TCellStyle()

	// Build display strings and compute widths.
	type item struct {
		text  string
		width int
	}
	items := make([]item, len(cs.candidates))
	for i, s := range cs.candidates {
		if len(s) > 25 {
			s = "..." + s[len(s)-22:]
		}
		var display string
		if i == cs.index {
			display = "[" + s + "]"
		} else {
			display = " " + s + " "
		}
		items[i] = item{display, len([]rune(display))}
	}

	// Find the horizontal offset so the selected item is visible.
	scroll := 0
	if cs.index >= 0 {
		// Compute start position of selected item.
		selStart := 0
		for i := 0; i < cs.index; i++ {
			selStart += items[i].width
		}
		selEnd := selStart + items[cs.index].width

		if selEnd > scroll+w {
			scroll = selEnd - w
		}
		if selStart < scroll {
			scroll = selStart
		}
	}

	// Render items starting from the scroll offset.
	x := 0
	col := 0 // virtual column (before scrolling)
	for _, it := range items {
		for _, r := range it.text {
			if col >= scroll && x < w {
				screen.SetContent(x, y, r, nil, ts)
				x++
			}
			col++
		}
	}

	// Fill rest of line.
	for x < w {
		screen.SetContent(x, y, ' ', nil, ts)
		x++
	}
}

// Draw renders the info bar at the given screen row.
func (ib *InfoBar) Draw(screen tcell.Screen, y, w int, th *Theme) {
	if y < 0 {
		return
	}
	if ib.active {
		ib.drawPrompt(screen, y, w, th)
		return
	}
	ib.drawMessage(screen, y, w, th)
}

func (ib *InfoBar) drawPrompt(screen tcell.Screen, y, w int, th *Theme) {
	ts := th.Default().TCellStyle()
	x := 0

	// Draw prompt character.
	for _, r := range ib.prompt {
		if x >= w {
			break
		}
		screen.SetContent(x, y, r, nil, ts)
		x++
	}

	// Draw input text.
	promptLen := x
	for _, r := range ib.input {
		if x >= w {
			break
		}
		screen.SetContent(x, y, r, nil, ts)
		x++
	}

	// Clear rest of line.
	for x < w {
		screen.SetContent(x, y, ' ', nil, ts)
		x++
	}

	// Show cursor in the prompt.
	screen.ShowCursor(promptLen+ib.cursorPos, y)
}

func (ib *InfoBar) drawMessage(screen tcell.Screen, y, w int, th *Theme) {
	style := th.Default()
	if ib.msgErr {
		style = th.Style("error")
	}
	ts := style.TCellStyle()

	x := 0
	for _, r := range ib.message {
		if x >= w {
			break
		}
		screen.SetContent(x, y, r, nil, ts)
		x++
	}

	if ib.showCursor {
		screen.ShowCursor(x, y)
	}

	// Clear rest of line.
	defTs := th.Default().TCellStyle()
	for x < w {
		screen.SetContent(x, y, ' ', nil, defTs)
		x++
	}
}
