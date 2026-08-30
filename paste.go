package main

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// Bracketed paste: the terminal wraps pasted text in EventPaste markers, and
// the content arrives as ordinary key events between them. The run loop
// collects those keys with collectPasteKey — no key dispatch and no per-key
// frame — and hands the finished text to pasteText, which inserts it
// verbatim: one edit per cursor under a single undo barrier, so autoindent,
// autoclose, and completion never touch pasted text and a large paste is one
// buffer edit (one frame, one undo unit, one LSP didChange) instead of
// thousands.

// collectPasteKey appends one key event's text to the paste buffer.
func (e *Editor) collectPasteKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyRune:
		e.pasteBuf.WriteRune(ev.Rune())
	case tcell.KeyEnter: // terminals paste line breaks as CR
		e.pasteBuf.WriteByte('\r')
	case tcell.KeyLF:
		e.pasteBuf.WriteByte('\n')
	case tcell.KeyTab:
		e.pasteBuf.WriteByte('\t')
	default:
		// Any other special key inside a paste can only come from control
		// sequences embedded in the pasted text: drop it.
	}
}

// pasteText inserts pasted text, vim-style per mode: insert/replace mode
// inserts at every cursor; normal mode inserts before the cursor and leaves
// it on the last pasted character (as if typed with i…<Esc>); charwise and
// linewise visual selections are replaced. A command-line prompt takes the
// first line.
func (e *Editor) pasteText(text string) {
	// Line breaks arrive as CR (and CRLF survives from some sources); the
	// buffer stores LF.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return
	}

	// Command-line prompt: paste the first line as input.
	if e.infobar.IsActive() {
		line := text
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		for _, r := range line {
			e.infobar.HandleKey(string(r))
		}
		return
	}

	// An open completion menu accepts its candidate first, as typing does.
	if e.hasCompletion() {
		e.acceptCompletion()
	}

	ks := e.ks
	b := ks.Buf()
	b.UndoBarrier()

	// Charwise/linewise visual selections are replaced. Block selections
	// just collapse: a rectangle has no linear span to delete.
	if ks.Mode().IsVisual {
		for i := range b.cursors {
			c := b.cursors[i]
			if c.HasSelection() && !c.BlockSel {
				b.Remove(c.Sel[0], c.Sel[1])
			}
			b.cursors[i].ClearSelection()
		}
		ks.SetMode(ModeNormal)
	}

	data := []byte(text)
	for i := 0; i < b.NumCursors(); i++ {
		b.Insert(b.cursors[i].Pos, data)
	}

	// Outside insert mode the cursor steps back onto the last pasted
	// character, like leaving insert mode does.
	if mode := ks.ModeID(); mode != ModeInsert && mode != ModeReplace {
		for i := 0; i < b.NumCursors(); i++ {
			c := b.cursors[i]
			if c.Pos > 0 {
				_, _, sz := b.DecodeGraphemeBefore(c.Pos)
				if r, _ := b.DecodeRuneAt(c.Pos - sz); r != '\n' {
					b.cursors[i] = c.MoveTo(c.Pos - sz)
				}
			}
			b.cursors[i] = b.cursors[i].VimClamp(b)
		}
		b.MergeCursors()
	}
}
