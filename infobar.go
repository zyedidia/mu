package main

import (
	"github.com/gdamore/tcell/v2"
)

// InfoBar is the command/message bar at the bottom of the editor.
type InfoBar struct {
	// Message display.
	message string
	msgErr  bool

	// Command-line input state.
	active    bool
	prompt    string // e.g. ":", "/", "?"
	input     []rune
	cursorPos int
	callback  func(input string) // called on Enter
}

// NewInfoBar creates a new info bar.
func NewInfoBar() *InfoBar {
	return &InfoBar{}
}

// Message shows a message in the info bar.
func (ib *InfoBar) Message(msg string) {
	ib.message = msg
	ib.msgErr = false
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

// IsActive returns true if command-line input is active.
func (ib *InfoBar) IsActive() bool {
	return ib.active
}

// StartPrompt activates command-line input with the given prompt character.
// The callback is called with the entered text when the user presses Enter.
func (ib *InfoBar) StartPrompt(prompt string, cb func(input string)) {
	ib.active = true
	ib.prompt = prompt
	ib.input = nil
	ib.cursorPos = 0
	ib.callback = cb
	ib.message = ""
}

// Cancel deactivates the command-line input.
func (ib *InfoBar) Cancel() {
	ib.active = false
	ib.input = nil
	ib.cursorPos = 0
	ib.callback = nil
}

// HandleKey processes a key event while the command-line is active.
// Returns true if the editor should redraw, false if the prompt closed.
func (ib *InfoBar) HandleKey(key string) (redraw bool, done bool) {
	switch key {
	case KeyEscape, "<C-c>":
		ib.Cancel()
		return true, true
	case KeyEnter:
		if ib.callback != nil {
			ib.callback(string(ib.input))
		}
		ib.Cancel()
		return true, true
	case KeyBacksp:
		if ib.cursorPos > 0 {
			ib.input = append(ib.input[:ib.cursorPos-1], ib.input[ib.cursorPos:]...)
			ib.cursorPos--
		}
		return true, false
	case KeyLeft:
		if ib.cursorPos > 0 {
			ib.cursorPos--
		}
		return true, false
	case KeyRight:
		if ib.cursorPos < len(ib.input) {
			ib.cursorPos++
		}
		return true, false
	case KeyHome, "<C-a>":
		ib.cursorPos = 0
		return true, false
	case KeyEnd, "<C-e>":
		ib.cursorPos = len(ib.input)
		return true, false
	default:
		// Insert character.
		if len(key) == 0 || (len(key) > 1 && key[0] == '<') {
			return false, false
		}
		rs := []rune(key)
		// Insert at cursor position.
		tail := make([]rune, len(ib.input[ib.cursorPos:]))
		copy(tail, ib.input[ib.cursorPos:])
		ib.input = append(ib.input[:ib.cursorPos], append(rs, tail...)...)
		ib.cursorPos += len(rs)
		return true, false
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
	// Clear rest of line.
	defTs := th.Default().TCellStyle()
	for x < w {
		screen.SetContent(x, y, ' ', nil, defTs)
		x++
	}
}
