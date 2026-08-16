package main

import (
	"bytes"
	"encoding/gob"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// dataDirOverride can be set by tests to avoid writing to ~/.config/mu.
var dataDirOverride string

// dataDir returns the path to ~/.config/mu/data/, creating it if needed.
func dataDir() string {
	if dataDirOverride != "" {
		os.MkdirAll(dataDirOverride, 0755)
		return dataDirOverride
	}
	dir := filepath.Join(configDir(), "data")
	os.MkdirAll(dir, 0755)
	return dir
}

// escapePath converts a filesystem path to a safe filename by replacing
// path separators with %.
func escapePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	escaped := strings.ReplaceAll(abs, string(filepath.Separator), "%")
	if len(escaped) > 0 && escaped[0] == '%' {
		escaped = escaped[1:]
	}
	return escaped
}

// --- Undo history persistence ---

// SaveUndoHistory serializes the buffer's undo tree to disk with the
// file's modification time and content hash so stale history can be
// detected on reload.
func (b *Buffer) SaveUndoHistory() {
	if b.Path == "" {
		return
	}
	// Persisted history is only valid for the on-disk contents. A modified
	// buffer's tree references states that were never written to the file,
	// so undoing from it after a fresh load would corrupt the buffer.
	if b.Modified() {
		return
	}
	path := filepath.Join(dataDir(), escapePath(b.Path)+".undo")
	f, err := os.Create(path)
	if err != nil {
		log.Printf("save undo: %v", err)
		return
	}
	defer f.Close()
	if err := b.undo.Serialize(f, b.modTime, b.hash()); err != nil {
		log.Printf("save undo: %v", err)
	}
}

// LoadUndoHistory restores the buffer's undo tree from disk. If the file's
// current modification time or content hash doesn't match the saved ones,
// the history is discarded.
func (b *Buffer) LoadUndoHistory() {
	if b.Path == "" {
		return
	}
	path := filepath.Join(dataDir(), escapePath(b.Path)+".undo")
	f, err := os.Open(path)
	if err != nil {
		return // no saved undo, that's fine
	}
	defer f.Close()
	t, err := FromReader[*Buffer, Cursor](f, b, NoCutoff, b.modTime, b.hash())
	if err != nil {
		log.Printf("load undo: %v", err)
		return
	}
	if t != nil {
		b.undo = t
	}
}

// --- Cursor and viewport persistence (the savecursor feature) ---

// SavedView is the persisted per-file editing position: the cursors and the
// view's scroll state.
type SavedView struct {
	Cursors []Cursor
	TopLine int // first visible buffer line
	TopCol  int // byte column of the first visible wrap row (softwrap)
	StCol   int // horizontal scroll column (no softwrap)
}

// SaveCursorPos serializes the view's cursor positions and viewport to disk.
func (v *View) SaveCursorPos() {
	b := v.buf
	if b.Path == "" {
		return
	}
	path := filepath.Join(dataDir(), escapePath(b.Path)+".cursor")
	f, err := os.Create(path)
	if err != nil {
		log.Printf("save cursor: %v", err)
		return
	}
	defer f.Close()
	sv := SavedView{
		Cursors: b.cursors,
		TopLine: v.topline,
		TopCol:  v.topcol,
		StCol:   v.stcol,
	}
	if err := gob.NewEncoder(f).Encode(sv); err != nil {
		log.Printf("save cursor: %v", err)
	}
}

// LoadCursorPos restores the cursor position and viewport from disk.
func (v *View) LoadCursorPos() {
	b := v.buf
	if b.Path == "" {
		return
	}
	path := filepath.Join(dataDir(), escapePath(b.Path)+".cursor")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var sv SavedView
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&sv); err != nil {
		// Older versions stored a bare cursor list without the viewport.
		var cursors []Cursor
		if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&cursors); err != nil {
			log.Printf("load cursor: %v", err)
			return
		}
		sv = SavedView{Cursors: cursors}
	}

	if len(sv.Cursors) > 0 {
		// Restore only the primary cursor's position. Selections and extra
		// cursors from the previous session would otherwise come back as
		// phantom state (highlighted regions, edits applied at old spots).
		c := sv.Cursors[0]
		c.Num = 0
		c.HasSel = false
		c.Sel = [2]int{}
		c.Orig = [2]int{}
		c.BlockSel = false
		c.BlockEOL = false
		if c.Pos > b.Len() {
			c.Pos = b.Len()
		}
		if c.Pos < 0 {
			c.Pos = 0
		}
		b.cursors = []Cursor{c}
		b.cur = 0
		v.savedCursor = c
	}

	// Restore the viewport, clamped to the current file contents. Relocate
	// re-anchors it if the geometry changed since (edits, resize, softwrap
	// toggled) and scrolls if the cursor is no longer within the margins.
	if sv.TopLine < 0 {
		sv.TopLine = 0
	}
	if sv.TopLine > b.NumLines() {
		sv.TopLine = b.NumLines()
	}
	if sv.TopCol < 0 || sv.TopCol >= b.LineLen(sv.TopLine) {
		sv.TopCol = 0
	}
	if sv.StCol < 0 {
		sv.StCol = 0
	}
	v.topline = sv.TopLine
	v.topcol = sv.TopCol
	v.stcol = sv.StCol
}

// --- Command history persistence ---

const historyFile = "history.gob"

// SaveHistory serializes the infobar command history to disk.
func (ib *InfoBar) SaveHistory() {
	path := filepath.Join(dataDir(), historyFile)
	f, err := os.Create(path)
	if err != nil {
		log.Printf("save history: %v", err)
		return
	}
	defer f.Close()
	enc := gob.NewEncoder(f)
	if err := enc.Encode(ib.history); err != nil {
		log.Printf("save history: %v", err)
	}
}

// LoadHistory restores command history from disk.
func (ib *InfoBar) LoadHistory() {
	path := filepath.Join(dataDir(), historyFile)
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	var hist map[string][]string
	dec := gob.NewDecoder(f)
	if err := dec.Decode(&hist); err != nil {
		log.Printf("load history: %v", err)
		return
	}
	ib.history = hist
}
