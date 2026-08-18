package main

import (
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"os"
	"path/filepath"
	"time"

	"github.com/zyedidia/mu/text"
	lsp "go.lsp.dev/protocol"
)

func init() {
	gob.RegisterName("mu.Edit", &Edit{})
}

// Buffer is the editor-level buffer that wraps a text.Buffer with undo history,
// multiple cursors, and modification tracking.
type Buffer struct {
	text *text.Buffer

	undo    *UndoTree[*Buffer, Cursor]
	cursors []Cursor
	cur     int // active cursor index

	// undoGroup, while positive, suppresses undo barriers so that all
	// edits coalesce into one undo event (used for macro replay).
	undoGroup int

	// bufnum is the stable buffer-list number assigned by the editor.
	bufnum int

	modified    bool
	readonly    bool
	Path        string
	Filetype    string
	modTime     time.Time // mtime when last loaded/saved
	savedHash   []byte    // md5 of contents at last save/load (nil if too large)
	savedLen    int       // content length at last save/load
	editGen     int       // bumped on every edit (cache invalidation)
	hashGen     int       // editGen the cached hash verdict belongs to
	hashClean   bool      // cached "content matches savedHash" verdict
	diagnostics []Diagnostic
	// Keep the original protocol diagnostics as well as the display-friendly
	// projection above; code-action requests must echo relevant diagnostics.
	lspDiagnostics []lsp.Diagnostic
	syntax         *SyntaxState

	lspServer  *LspServer
	lspVersion int32
	lspFt      string // filetype for LSP

	vis *Visualizer // set by the view, used for visual column calculations

	watchDone   chan struct{} // closed to stop the file watcher
	onReload    func(*Buffer) // called from watcher when auto-reloaded
	onHighlight func()        // called when background highlighting finishes
}

// NewBuffer creates a new editor buffer from raw file data, auto-detecting
// charset and line endings.
func NewBuffer(data []byte, path string) (*Buffer, error) {
	tb, err := text.NewBuffer(data, text.Options{})
	if err != nil {
		return nil, err
	}
	return newBufferFromText(tb, path), nil
}

// NewBufferFromText creates a new editor buffer from an existing text.Buffer.
func NewBufferFromText(tb *text.Buffer, path string) *Buffer {
	return newBufferFromText(tb, path)
}

func newBufferFromText(tb *text.Buffer, path string) *Buffer {
	b := &Buffer{
		text:    tb,
		cursors: []Cursor{{Num: 0}},
		cur:     0,
		Path:    path,
	}
	b.undo = NewUndoTree[*Buffer, Cursor](b, NoCutoff)
	b.markUnmodified()
	return b
}

// NewEmptyBuffer creates an empty editor buffer.
func NewEmptyBuffer() *Buffer {
	tb := text.NewBufferFromUTF8([]byte{}, text.Options{})
	return newBufferFromText(tb, "")
}

// --- Delegated text.Buffer methods ---

// Len returns the number of bytes in the buffer.
func (b *Buffer) Len() int { return b.text.Len() }

// NumLines returns the number of lines in the buffer.
func (b *Buffer) NumLines() int { return b.text.NumLines() }

// Slice returns the bytes in range [start:end).
func (b *Buffer) Slice(start, end int) []byte { return b.text.Slice(start, end) }

// ByteAt returns the byte at position pos.
func (b *Buffer) ByteAt(pos int) byte { return b.text.ByteAt(pos) }

// DecodeRuneAt returns the rune at offset and its byte size.
func (b *Buffer) DecodeRuneAt(off int) (rune, int) { return b.text.DecodeRuneAt(off) }

// DecodeRuneBefore returns the rune before offset and its byte size.
func (b *Buffer) DecodeRuneBefore(off int) (rune, int) { return b.text.DecodeRuneBefore(off) }

// OffsetAt returns the byte offset for a line/col pair.
func (b *Buffer) OffsetAt(line, col int) int { return b.text.OffsetAt(line, col) }

// LineColAt returns the line/col pair for a byte offset.
func (b *Buffer) LineColAt(pos int) (int, int) { return b.text.LineColAt(pos) }

// LineLen returns the length of line i in bytes, excluding the '\n'.
func (b *Buffer) LineLen(i int) int { return b.text.LineLen(i) }

// GetLine returns the bytes for line i without the trailing '\n'.
func (b *Buffer) GetLine(i int) []byte { return b.text.GetLine(i) }

// ReadAt implements io.ReaderAt, used by search to avoid copying the buffer.
func (b *Buffer) ReadAt(p []byte, off int64) (int, error) { return b.text.ReadAt(p, off) }

const hashCutoff = 1024 * 1024 // 1MB

// Modified returns whether the buffer has been modified since last save.
// For small files, uses a hash comparison to avoid false positives (e.g.
// after undoing all changes). Called on every status-bar draw, so it must
// be cheap: a different length decides without hashing (this also covers
// buffers grown past hashCutoff since load, whose saved hash describes a
// smaller content), and the hash verdict is cached per edit generation so
// idle redraws never re-hash.
func (b *Buffer) Modified() bool {
	if b.savedHash == nil {
		return b.modified
	}
	if b.Len() != b.savedLen {
		return true
	}
	if b.hashGen != b.editGen {
		b.hashClean = bytes.Equal(b.hash(), b.savedHash)
		b.hashGen = b.editGen
	}
	return !b.hashClean
}

// markUnmodified records the current state as the "saved" baseline.
func (b *Buffer) markUnmodified() {
	b.modified = false
	b.savedLen = b.Len()
	if b.Len() <= hashCutoff {
		b.savedHash = b.hash()
		b.hashGen = b.editGen
		b.hashClean = true
	} else {
		b.savedHash = nil
	}
}

// EditGen returns a counter that changes on every buffer edit, for caches
// keyed on buffer content (row geometry, the Modified hash verdict).
func (b *Buffer) EditGen() int {
	return b.editGen
}

func (b *Buffer) hash() []byte {
	h := md5.New()
	b.text.WriteTo(h)
	return h.Sum(nil)
}

// Text returns the underlying text.Buffer.
func (b *Buffer) Text() *text.Buffer { return b.text }

// --- Edit operations ---

// Edit represents a modification to the buffer that can be undone.
type Edit struct {
	Start, End int
	Text       []byte // bytes to insert
	Sub        []byte // bytes that were removed (populated by Do)
	C          Cursor // cursor state at time of edit
}

func (e *Edit) State() Cursor {
	return e.C
}

// Do applies this edit to the buffer.
func (e *Edit) Do(buf *Buffer) {
	e.Sub = buf.text.Slice(e.Start, e.End)
	buf.applyEdit(e.Start, e.End, e.Text)
}

// Undo reverts this edit on the buffer.
func (e *Edit) Undo(buf *Buffer) {
	buf.applyEdit(e.Start, e.Start+len(e.Text), e.Sub)
}

// applyEdit performs a raw edit on the text buffer and adjusts all cursor
// positions accordingly.
func (b *Buffer) applyEdit(start, end int, val []byte) {
	// Compute LSP positions before modifying the buffer, since the LSP
	// expects ranges in the pre-edit document.
	b.lspSendEdit(start, end, val)

	b.text.Remove(start, end)
	b.text.Insert(start, val)
	b.modified = true
	b.editGen++

	// Update syntax highlighting memo table.
	b.SyntaxApplyEdit(start, end, len(val))

	// Adjust all cursor positions for the edit.
	insLen := len(val)
	delLen := end - start
	for i, c := range b.cursors {
		b.cursors[i].Pos = adjustPos(c.Pos, start, delLen, insLen)
		b.cursors[i].Orig[0] = adjustPos(c.Orig[0], start, delLen, insLen)
		b.cursors[i].Orig[1] = adjustPos(c.Orig[1], start, delLen, insLen)
		b.cursors[i].Sel[0] = adjustPos(c.Sel[0], start, delLen, insLen)
		b.cursors[i].Sel[1] = adjustPos(c.Sel[1], start, delLen, insLen)
	}
}

// adjustPos updates a position after a delete+insert at start.
func adjustPos(p, start, delLen, insLen int) int {
	end := start + delLen
	// Adjust for deletion.
	if p >= start && p < end {
		p = start
	} else if p >= end {
		p -= delLen
	}
	// Adjust for insertion.
	if p >= start {
		p += insLen
	}
	return p
}

// Insert inserts val at pos, recording the change in the undo tree.
func (b *Buffer) Insert(pos int, val []byte) {
	b.DoEdit(&Edit{
		Start: pos,
		End:   pos,
		Text:  val,
	})
}

// Remove removes the range [start:end), recording the change in the undo tree.
func (b *Buffer) Remove(start, end int) {
	b.DoEdit(&Edit{
		Start: start,
		End:   end,
		Text:  []byte{},
	})
}

// DoEdit applies an edit and records it in the undo tree.
func (b *Buffer) DoEdit(e *Edit) {
	e.C = *b.Cursor()
	b.undo.Apply(e)
}

// Undo reverts the most recent edit.
func (b *Buffer) Undo() {
	// Edits made after an undo must never coalesce backwards into the
	// event that is now current (possible inside an undo group, where
	// normal barriers are suppressed).
	b.undo.Barrier()
	c, ok := b.undo.PrevState()
	b.undo.Undo()
	if ok {
		c.HasSel = false
		b.PutCursor(c)
	}
}

// Redo reapplies the most recently undone edit.
func (b *Buffer) Redo() {
	b.undo.Barrier()
	if ep, ok := b.undo.MostRecent(); ok {
		c := b.undo.NextState(ep)
		c.HasSel = false
		b.PutCursor(c)
		b.undo.Redo(ep)
	}
}

// UndoBarrier prevents the next edit from coalescing with the previous one.
// Inside an undo group the barrier is suppressed, so a group's edits form a
// single undo event.
func (b *Buffer) UndoBarrier() {
	if b.undoGroup > 0 {
		return
	}
	b.undo.Barrier()
}

// BeginUndoGroup starts an undo group: until the matching EndUndoGroup,
// barriers are ignored and every edit coalesces into one undo event.
// Groups nest.
func (b *Buffer) BeginUndoGroup() {
	if b.undoGroup == 0 {
		b.undo.Barrier()
	}
	b.undoGroup++
}

// EndUndoGroup closes an undo group started with BeginUndoGroup.
func (b *Buffer) EndUndoGroup() {
	if b.undoGroup > 0 {
		b.undoGroup--
		if b.undoGroup == 0 {
			b.undo.Barrier()
		}
	}
}

// --- LSP integration ---

func (b *Buffer) lspSendEdit(start, end int, text []byte) {
	if b.lspServer == nil {
		return
	}
	b.lspVersion++
	absPath, _ := filepath.Abs(b.Path)

	startLine, startCol := b.LineColAt(start)
	endLine, endCol := b.LineColAt(end)
	_, startCol16 := b.Utf16Loc(startLine, startCol)
	_, endCol16 := b.Utf16Loc(endLine, endCol)

	change := lsp.TextDocumentContentChangeEvent{
		Range: lsp.Range{
			Start: lsp.Position{Line: uint32(startLine), Character: uint32(startCol16)},
			End:   lsp.Position{Line: uint32(endLine), Character: uint32(endCol16)},
		},
		Text: string(text),
	}
	b.lspServer.DidChange(absPath, b.lspVersion, []lsp.TextDocumentContentChangeEvent{change})
}

// LspClose notifies the language server that this buffer's document is
// closed. Call when the buffer is dropped from the editor.
func (b *Buffer) LspClose() {
	if b.lspServer == nil {
		return
	}
	absPath, _ := filepath.Abs(b.Path)
	b.lspServer.DidClose(absPath)
	b.lspServer = nil
}

// LspPosition converts a (line, byte-col) pair to an LSP Position (UTF-16).
func (b *Buffer) LspPosition(line, col int) lsp.Position {
	_, col16 := b.Utf16Loc(line, col)
	return lsp.Position{Line: uint32(line), Character: uint32(col16)}
}

// FromLspPosition converts an LSP Position to a byte offset.
func (b *Buffer) FromLspPosition(pos lsp.Position) int {
	_, col8 := b.Utf8Loc(int(pos.Line), int(pos.Character))
	return b.OffsetAt(int(pos.Line), col8)
}

// --- File watcher ---

// StartWatcher launches a background goroutine that checks the file for
// external changes every second. If the buffer is unmodified and the file
// has changed, it auto-reloads. The onReload callback (if set) is called
// after a successful reload so the editor can trigger a redraw.
func (b *Buffer) StartWatcher() {
	if b.Path == "" {
		return
	}
	done := make(chan struct{})
	b.watchDone = done
	go b.watchLoop(done)
}

// StopWatcher stops the background file watcher.
func (b *Buffer) StopWatcher() {
	if b.watchDone != nil {
		close(b.watchDone)
		b.watchDone = nil
	}
}

// watchLoop polls for external modifications until done is closed. The
// channel is passed in (rather than read from the field) so StopWatcher can
// clear the field without racing this goroutine.
func (b *Buffer) watchLoop(done chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if b.Modified() || !b.ExternallyModified() {
				continue
			}
			if b.onReload != nil {
				// The owner performs the reload (on the editor's main
				// goroutine) to avoid concurrent buffer mutation.
				b.onReload(b)
			} else {
				b.Reload()
			}
		}
	}
}

// --- Reload and external change detection ---

const diffCutoff = 4096 * 64 // 256KB: above this, skip diff and replace

// maxReloadDiffNodes bounds the search nodes DiffBounded may allocate when
// diffing a reload (~24 bytes each). Large enough to preserve undo history
// across formatter-sized changes and cheap large appends; completely
// dissimilar content exhausts it in tens of milliseconds and falls back to
// wholesale replacement.
const maxReloadDiffNodes = 2 << 20

// SetContent replaces the buffer contents with those of newb. For small
// buffers with modest changes it diffs and applies edits so undo history is
// preserved. Otherwise it replaces wholesale (losing undo history).
func (b *Buffer) SetContent(newb *text.Buffer) {
	if b.Len() < diffCutoff && newb.Len() < diffCutoff {
		if edits, ok := DiffBounded(b.text, newb, maxReloadDiffNodes); ok {
			b.UndoBarrier()
			var pos int
			for _, e := range edits {
				switch e.Kind {
				case DiffInsert:
					b.Insert(pos, e.Text)
					pos += e.Length
				case DiffDelete:
					b.Remove(pos, pos+e.Length)
				case DiffEqual:
					pos += e.Length
				}
			}
			b.markUnmodified()
			return
		}
	}

	// Wholesale replacement. The incremental channels must not silently
	// diverge: describe the change to the LSP server as one edit covering
	// the whole old document (computed against the pre-swap content), and
	// reset the syntax window afterwards.
	if b.lspServer != nil {
		b.lspSendEdit(0, b.Len(), newb.Bytes())
	}
	b.text = newb
	b.undo = NewUndoTree[*Buffer, Cursor](b, NoCutoff)
	for i := range b.cursors {
		b.cursors[i] = b.cursors[i].Clamp(b)
	}
	b.markUnmodified()
	b.SyntaxReset()
}

// Reload reads the file from disk and applies changes via diff.
func (b *Buffer) Reload() error {
	if b.Path == "" {
		return nil
	}
	data, err := os.ReadFile(b.Path)
	if err != nil {
		return err
	}
	newb, err := text.NewBuffer(data, b.text.Opts)
	if err != nil {
		return err
	}
	b.SetContent(newb)
	b.updateModTime()
	return nil
}

// updateModTime stores the file's current mtime.
func (b *Buffer) updateModTime() {
	if b.Path == "" {
		return
	}
	if fi, err := os.Stat(b.Path); err == nil {
		b.modTime = fi.ModTime()
	}
}

// ExternallyModified returns true if the file on disk has been modified
// since we last loaded or saved it.
func (b *Buffer) ExternallyModified() bool {
	if b.Path == "" {
		return false
	}
	fi, err := os.Stat(b.Path)
	if err != nil {
		return false
	}
	return fi.ModTime().After(b.modTime)
}
