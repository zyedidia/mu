package main

import (
	"context"
	"io"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zyedidia/flare"
	"github.com/zyedidia/ftdetect"
	"github.com/zyedidia/gpeg/memo"
	"github.com/zyedidia/gpeg/vm"
	"golang.org/x/sync/semaphore"
)

// Windowed syntax highlighting: for large files, flare operates on a
// limited region. The "core window" is syntaxWindowSize bytes. When the
// cursor leaves the core, the window re-centers. The actual highlighted
// region extends syntaxOverlap beyond the core on each side, so there is
// pre-highlighted content when crossing the boundary.
const (
	syntaxWindowSize = 10 * 1024 * 1024 // 10MB core window
	syntaxOverlap    = 5 * 1024 * 1024  // 5MB overlap on each side
)

// SyntaxState holds the syntax highlighting state for a buffer.
type SyntaxState struct {
	highlighter *flare.Highlighter
	syntbl      memo.Table
	matches     *flare.Matches
	hisem       *semaphore.Weighted
	minvalid    bool // matches need recalculation

	// Core window: cursor leaving this range triggers re-centering.
	coreStart int
	coreEnd   int

	// Highlight region: the range actually fed to flare (core + overlap).
	hlStart int
	hlEnd   int

	mu sync.Mutex
}

// Global filetype detectors (lazily initialized).
var (
	ftDetectors     ftdetect.Detectors
	ftDetectorsOnce sync.Once
)

func loadDetectors(cfg *Config) ftdetect.Detectors {
	ftDetectorsOnce.Do(func() {
		ftDetectors = make(ftdetect.Detectors)
		walkFn := func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
				return nil
			}
			data, err := embedFS.ReadFile(path)
			if err != nil {
				return nil
			}
			det, err := ftdetect.LoadDetectorJson(data)
			if err != nil {
				return nil
			}
			ftDetectors.RegisterDetector(det)
			return nil
		}
		fs.WalkDir(embedFS, "embed/detectors", walkFn)
	})
	return ftDetectors
}

// DetectFiletype guesses the filetype from the filename and first line.
func DetectFiletype(cfg *Config, name string, firstLine []byte) string {
	ds := loadDetectors(cfg)
	d := ds.Detect(name, firstLine)
	if d == nil {
		return ""
	}
	return d.Name
}

// LoadHighlighter loads a flare highlighter for the given filetype name.
func (c *Config) LoadHighlighter(name string) (*flare.Highlighter, error) {
	data, err := c.ReadFile(filepath.Join("highlighters", name+".lang"))
	if err != nil {
		return nil, err
	}
	return flare.LoadHighlighter(name, data, true)
}

// --- Buffer syntax methods ---

// InitSyntax initializes syntax highlighting for the buffer based on its
// filetype. Should be called after the buffer is loaded and filetype is known.
func (b *Buffer) InitSyntax(cfg *Config, ft string) {
	if ft == "" {
		return
	}
	h, err := cfg.LoadHighlighter(ft)
	if err != nil {
		log.Printf("highlighter %q: %v", ft, err)
		return
	}
	b.syntax = &SyntaxState{
		highlighter: h,
		syntbl:      memo.NewTreeTable(512),
		hisem:       semaphore.NewWeighted(1),
	}
	b.syntax.setWindow(0, b.Len())

	// Run initial highlight in background.
	go b.initialHighlight()
}

// setWindow positions the core window and highlight region around cursorPos.
func (ss *SyntaxState) setWindow(cursorPos, bufLen int) {
	if bufLen <= syntaxWindowSize {
		// Small file: core and highlight cover everything.
		ss.coreStart = 0
		ss.coreEnd = bufLen
		ss.hlStart = 0
		ss.hlEnd = bufLen
		return
	}

	// Center the core window on the cursor.
	half := syntaxWindowSize / 2
	ss.coreStart = cursorPos - half
	if ss.coreStart < 0 {
		ss.coreStart = 0
	}
	ss.coreEnd = ss.coreStart + syntaxWindowSize
	if ss.coreEnd > bufLen {
		ss.coreEnd = bufLen
		ss.coreStart = ss.coreEnd - syntaxWindowSize
	}

	// Extend the highlight region by the overlap on each side.
	ss.hlStart = ss.coreStart - syntaxOverlap
	if ss.hlStart < 0 {
		ss.hlStart = 0
	}
	ss.hlEnd = ss.coreEnd + syntaxOverlap
	if ss.hlEnd > bufLen {
		ss.hlEnd = bufLen
	}
}

// initialHighlight fills the memo table for the highlight region without
// generating matches. This makes subsequent incremental highlights fast.
func (b *Buffer) initialHighlight() {
	ss := b.syntax
	if ss == nil || ss.highlighter == nil {
		return
	}
	ss.hisem.Acquire(context.Background(), 1)
	defer ss.hisem.Release(1)

	r := io.NewSectionReader(b, int64(ss.hlStart), int64(ss.hlEnd-ss.hlStart))
	ss.highlighter.HighlightFunc(r, ss.syntbl, nil, &vm.Interval{Low: 0, High: 0})
}

// SyntaxApplyEdit updates the memo table after a text edit.
func (b *Buffer) SyntaxApplyEdit(start, end, insLen int) {
	ss := b.syntax
	if ss == nil {
		return
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Adjust edit positions relative to the highlight region.
	relStart := start - ss.hlStart
	relEnd := end - ss.hlStart
	if relStart < 0 {
		relStart = 0
	}

	ss.syntbl.ApplyEdit(memo.Edit{
		Start: relStart,
		End:   relEnd,
		Len:   insLen,
	})
	ss.minvalid = true

	// Adjust window boundaries for the size change.
	delta := insLen - (end - start)
	ss.coreEnd += delta
	ss.hlEnd += delta
}

// SyntaxCheckWindow re-centers the window if the cursor has left the core.
func (b *Buffer) SyntaxCheckWindow(cursorPos int) {
	ss := b.syntax
	if ss == nil || b.Len() <= syntaxWindowSize {
		return
	}
	if cursorPos < ss.coreStart || cursorPos >= ss.coreEnd {
		ss.mu.Lock()
		ss.setWindow(cursorPos, b.Len())
		ss.syntbl = memo.NewTreeTable(512)
		ss.matches = nil
		ss.minvalid = true
		ss.mu.Unlock()

		go b.initialHighlight()
	}
}

// HighlightRange computes syntax matches for the byte range [off, end).
// Non-blocking: if the semaphore is held, uses existing matches.
func (b *Buffer) HighlightRange(off, end int) {
	ss := b.syntax
	if ss == nil || ss.highlighter == nil {
		return
	}
	if !ss.hisem.TryAcquire(1) {
		return
	}
	defer ss.hisem.Release(1)

	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Clamp to highlight region.
	if off < ss.hlStart {
		off = ss.hlStart
	}
	if end > ss.hlEnd {
		end = ss.hlEnd
	}

	// Convert to highlight-region-relative offsets.
	relOff := off - ss.hlStart
	relEnd := end - ss.hlStart

	if ss.matches == nil || ss.minvalid || !ss.matches.InRange(relOff) || !ss.matches.InRange(relEnd-1) {
		r := io.NewSectionReader(b, int64(ss.hlStart), int64(ss.hlEnd-ss.hlStart))
		ss.matches = ss.highlighter.HighlightMatches(r, ss.syntbl, &vm.Interval{Low: relOff, High: relEnd})
		ss.minvalid = false
	}
}

// SyntaxGroup returns the syntax group name at the given absolute byte offset.
func (b *Buffer) SyntaxGroup(off int) string {
	ss := b.syntax
	if ss == nil || ss.matches == nil {
		return ""
	}
	relOff := off - ss.hlStart
	if relOff < 0 || relOff >= ss.hlEnd-ss.hlStart {
		return ""
	}
	return ss.matches.Group(relOff)
}
