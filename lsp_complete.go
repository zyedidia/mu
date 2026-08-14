package main

import (
	"log"
	"path/filepath"
	"sort"
	"unicode"

	lsp "go.lsp.dev/protocol"
)

// EditorCompletion tracks in-buffer LSP completion state.
type EditorCompletion struct {
	active     bool
	candidates []string             // display labels
	items      []lsp.CompletionItem // full items (for insertText)
	index      int                  // -1 = none selected, 0..n-1 = selected
	startPos   int                  // byte offset where the completed word starts
	origWord   string               // the original text before completion
}

func (ec *EditorCompletion) reset() {
	ec.active = false
	ec.candidates = nil
	ec.items = nil
	ec.index = -1
	ec.startPos = 0
	ec.origWord = ""
}

// triggerCompletion requests LSP completion at the cursor. If no LSP is
// available or returns nothing, falls back to buffer word completion.
func (e *Editor) triggerCompletion() {
	v := e.ActiveView()
	if v == nil {
		return
	}
	b := v.buf

	// Find the word prefix being completed.
	startPos := b.Cursor().Pos
	for startPos > 0 {
		r, sz := b.DecodeRuneBefore(startPos)
		if !IsWordChar(r) {
			break
		}
		startPos -= sz
	}
	origWord := string(b.Slice(startPos, b.Cursor().Pos))

	var candidates []string
	var items []lsp.CompletionItem

	// Try LSP first.
	if b.lspServer != nil {
		line, col := b.LineColAt(b.Cursor().Pos)
		pos := b.LspPosition(line, col)
		absPath, _ := filepath.Abs(b.Path)

		lspItems, err := b.lspServer.Completion(absPath, pos)
		if err == nil && len(lspItems) > 0 {
			sort.Slice(lspItems, func(i, j int) bool {
				si, sj := lspItems[i].SortText, lspItems[j].SortText
				if si == "" {
					si = lspItems[i].Label
				}
				if sj == "" {
					sj = lspItems[j].Label
				}
				return si < sj
			})
			items = lspItems
			candidates = make([]string, len(items))
			for i, item := range items {
				candidates[i] = item.Label
			}
		} else if err != nil && err != ErrLspNotSupported {
			log.Printf("[lsp] completion: %v", err)
		}
	}

	// Fall back to buffer word completion.
	if len(candidates) == 0 {
		candidates = bufferComplete(b, origWord)
		items = nil
	}

	if len(candidates) == 0 {
		return
	}

	e.completion = EditorCompletion{
		active:     true,
		candidates: candidates,
		items:      items,
		index:      0,
		startPos:   startPos,
		origWord:   origWord,
	}

	e.applyCompletion()
}

// bufferComplete collects unique words from the buffer that match the prefix,
// excluding the prefix itself.
func bufferComplete(b *Buffer, prefix string) []string {
	if prefix == "" {
		return nil
	}

	seen := make(map[string]bool)
	var candidates []string

	pos := 0
	blen := b.Len()
	for pos < blen {
		r, _, sz := b.DecodeGraphemeAt(pos)
		if !IsWordChar(r) {
			pos += sz
			continue
		}
		// Read the whole word.
		wordStart := pos
		for pos < blen {
			r, _, sz = b.DecodeGraphemeAt(pos)
			if !IsWordChar(r) {
				break
			}
			pos += sz
		}
		word := string(b.Slice(wordStart, pos))
		if word != prefix && len(word) > len(prefix) && word[:len(prefix)] == prefix && !seen[word] {
			seen[word] = true
			candidates = append(candidates, word)
		}
	}

	sort.Strings(candidates)
	return candidates
}

// applyCompletion replaces the word at startPos with the current candidate.
func (e *Editor) applyCompletion() {
	ec := &e.completion
	if !ec.active || ec.index < 0 || ec.index >= len(ec.candidates) {
		return
	}

	b := e.ActiveView().buf

	// LSP items carry an insertText; buffer-word candidates are plain text.
	text := ec.candidates[ec.index]
	if ec.index < len(ec.items) {
		item := ec.items[ec.index]
		if item.InsertText != "" {
			text = item.InsertText
		} else if item.Label != "" {
			text = item.Label
		}
	}

	// Replace from startPos to current cursor.
	curPos := b.Cursor().Pos
	if curPos > ec.startPos {
		b.Remove(ec.startPos, curPos)
	}
	b.Insert(ec.startPos, []byte(text))
}

// acceptCompletion finalizes the current completion and closes the UI.
func (e *Editor) acceptCompletion() {
	e.completion.reset()
}

// cancelCompletion restores the original text and closes the UI.
func (e *Editor) cancelCompletion() {
	ec := &e.completion
	if !ec.active {
		return
	}

	b := e.ActiveView().buf
	curPos := b.Cursor().Pos
	if curPos > ec.startPos {
		b.Remove(ec.startPos, curPos)
	}
	b.Insert(ec.startPos, []byte(ec.origWord))
	ec.reset()
}

// nextCompletion cycles to the next candidate.
func (e *Editor) nextCompletion() {
	ec := &e.completion
	if !ec.active || len(ec.candidates) == 0 {
		return
	}
	ec.index = (ec.index + 1) % len(ec.candidates)
	e.applyCompletion()
}

// prevCompletion cycles to the previous candidate.
func (e *Editor) prevCompletion() {
	ec := &e.completion
	if !ec.active || len(ec.candidates) == 0 {
		return
	}
	ec.index = (ec.index - 1 + len(ec.candidates)) % len(ec.candidates)
	e.applyCompletion()
}

// hasCompletion returns true if editor completion is active.
func (e *Editor) hasCompletion() bool {
	return e.completion.active
}

// handleCompletionKey processes a key while completion is active.
func (e *Editor) handleCompletionKey(key string) {
	switch key {
	case "<C-n>", KeyDown, KeyTab:
		e.nextCompletion()
	case "<C-p>", KeyUp, "<S-Tab>":
		e.prevCompletion()
	case KeyEnter:
		e.acceptCompletion()
	case KeyEscape, "<C-c>":
		e.cancelCompletion()
		// Pass through so Escape also exits insert mode, etc.
		e.ks.HandleKey(key)
	default:
		// Accept current completion and pass the key through.
		e.acceptCompletion()
		e.ks.HandleKey(key)
	}
}

// drawEditorCompletions renders the LSP completion candidates on the given
// screen row, with the selected item in brackets and horizontal scrolling.
func (e *Editor) drawEditorCompletions(y int) {
	ec := &e.completion
	if !ec.active || len(ec.candidates) == 0 {
		return
	}

	style := e.theme.Default().Add(AttrReverse)
	ts := style.TCellStyle()

	type item struct {
		text  string
		width int
	}
	items := make([]item, len(ec.candidates))
	for i, s := range ec.candidates {
		if len(s) > 30 {
			s = s[:27] + "..."
		}
		var display string
		if i == ec.index {
			display = "[" + s + "]"
		} else {
			display = " " + s + " "
		}
		items[i] = item{display, len([]rune(display))}
	}

	// Horizontal scroll to keep selected item visible.
	scroll := 0
	if ec.index >= 0 {
		selStart := 0
		for i := 0; i < ec.index; i++ {
			selStart += items[i].width
		}
		selEnd := selStart + items[ec.index].width
		if selEnd > scroll+e.w {
			scroll = selEnd - e.w
		}
		if selStart < scroll {
			scroll = selStart
		}
	}

	x := 0
	col := 0
	for _, it := range items {
		for _, r := range it.text {
			if col >= scroll && x < e.w {
				e.screen.SetContent(x, y, r, nil, ts)
				x++
			}
			col++
		}
	}
	for x < e.w {
		e.screen.SetContent(x, y, ' ', nil, ts)
		x++
	}
}

// cursorAfterNonSpace returns true if the character before the cursor is
// not whitespace, so that Tab triggers completion after word chars and
// punctuation like `.`, `::`, `->`, etc.
func (e *Editor) cursorAfterNonSpace() bool {
	b := e.ActiveView().buf
	if b.Cursor().Pos == 0 {
		return false
	}
	r, sz := b.DecodeRuneBefore(b.Cursor().Pos)
	return sz > 0 && !unicode.IsSpace(r)
}

// registerCompletionBindings adds completion triggers in insert mode.
func (e *Editor) registerCompletionBindings() {
	// Ctrl-Space: always trigger completion.
	e.ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		e.triggerCompletion()
	}, "<C-space>")

	// Tab in insert mode: trigger/cycle completion if in a word,
	// otherwise insert a tab.
	e.ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		if e.hasCompletion() {
			e.nextCompletion()
		} else if e.cursorAfterNonSpace() {
			e.triggerCompletion()
		} else {
			b := ks.Buf()
			for i := 0; i < b.NumCursors(); i++ {
				b.Insert(b.cursors[i].Pos, []byte("\t"))
			}
		}
	}, KeyTab)

	// Shift-Tab in insert mode: previous completion.
	e.ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		if e.hasCompletion() {
			e.prevCompletion()
		}
	}, "<S-Tab>")

	// Ctrl-N / Ctrl-P: also work for completion.
	e.ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		if e.hasCompletion() {
			e.nextCompletion()
		} else {
			e.triggerCompletion()
		}
	}, "<C-n>")

	e.ks.modes[ModeInsert].Bindings.Bind(func(ks *KeyState) {
		if e.hasCompletion() {
			e.prevCompletion()
		} else {
			e.triggerCompletion()
		}
	}, "<C-p>")
}
