package main

import (
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

// --- Cursor position persistence ---

// SaveCursorPos serializes the buffer's cursor positions to disk.
func (b *Buffer) SaveCursorPos() {
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
	enc := gob.NewEncoder(f)
	if err := enc.Encode(b.cursors); err != nil {
		log.Printf("save cursor: %v", err)
	}
}

// LoadCursorPos restores cursor positions from disk.
func (b *Buffer) LoadCursorPos() {
	if b.Path == "" {
		return
	}
	path := filepath.Join(dataDir(), escapePath(b.Path)+".cursor")
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	var cursors []Cursor
	dec := gob.NewDecoder(f)
	if err := dec.Decode(&cursors); err != nil {
		log.Printf("load cursor: %v", err)
		return
	}
	if len(cursors) > 0 {
		b.cursors = cursors
		b.cur = 0
		// Clamp positions to buffer size.
		for i := range b.cursors {
			if b.cursors[i].Pos > b.Len() {
				b.cursors[i].Pos = b.Len()
			}
		}
	}
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
