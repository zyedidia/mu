package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
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
	syntaxWindowSize = 1024 * 1024 // 1MB core window
	syntaxOverlap    = 100 * 1024  // 100KB overlap on each side
)

// SyntaxState holds the syntax highlighting state for a buffer.
//
// Concurrency: all fields are guarded by mu. The background highlight pass
// works on a snapshot of the window content and a private memo table, so it
// never touches the buffer or the live table; on completion it replays the
// edits made meanwhile (pendingEdits) and swaps its table in, unless the
// window was re-positioned (gen changed) in the meantime.
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

	gen          int         // window generation; bumped when the window resets
	bgActive     bool        // a background highlight is in flight
	pendingEdits []memo.Edit // edits made while bgActive, replayed on completion

	mu sync.Mutex
}

// readDetectors loads every *.json detector under dir in fsys, in the lexical
// order WalkDir visits them. A malformed file is logged and skipped so one bad
// detector cannot take out the rest. A missing dir yields nothing.
func readDetectors(fsys fs.FS, dir, src string) []*ftdetect.Detector {
	var dets []*ftdetect.Detector
	fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			log.Printf("detectors: %s: %s: %v", src, path, err)
			return nil
		}
		det, err := ftdetect.LoadDetectorJson(data)
		if err != nil {
			log.Printf("detectors: %s: %s: %v", src, path, err)
			return nil
		}
		if det.Name == "" {
			log.Printf("detectors: %s: %s: missing name", src, path)
			return nil
		}
		dets = append(dets, det)
		return nil
	})
	return dets
}

// registerDisplacing registers d under each of its extensions and filenames,
// skipping any key in claimed. A detector whose keys are all claimed drops out
// entirely; one that declares no keys at all is header-only and always
// registers, since those are matched by regex rather than looked up by key.
func registerDisplacing(ds ftdetect.Detectors, d *ftdetect.Detector, claimed map[string]bool) {
	keep := func(keys []string) []string {
		var out []string
		for _, k := range keys {
			if !claimed[k] {
				out = append(out, k)
			}
		}
		return out
	}
	kept := *d
	kept.Exts = keep(d.Exts)
	kept.Files = keep(d.Files)
	if len(kept.Exts) == 0 && len(kept.Files) == 0 && (len(d.Exts) > 0 || len(d.Files) > 0) {
		return
	}
	ds.RegisterDetector(&kept)
}

// loadDetectors builds the filetype detector set: the embedded defaults with
// the user's detectors from ~/.config/mu/detectors/*.json layered over them.
// The result is cached on the Config.
//
// Layering happens on two levels. A user detector replaces an embedded one of
// the same name, and beyond that, any extension or filename a user detector
// claims is served by user detectors alone. The second rule is what makes an
// override usable: ftdetect resolves a key held by several detectors via their
// file/header regexes, so an embedded plain-extension detector left sitting
// next to a user one makes Detect match neither and report no filetype at all.
func loadDetectors(cfg *Config) ftdetect.Detectors {
	cfg.detectorsOnce.Do(func() {
		embedded := readDetectors(embedFS, "embed/detectors", "embedded")
		user := readDetectors(os.DirFS(cfg.dir), "detectors", cfg.dir)

		replaced := make(map[string]bool, len(user))
		claimed := make(map[string]bool)
		for _, d := range user {
			replaced[d.Name] = true
			for _, k := range d.Exts {
				claimed[k] = true
			}
			for _, k := range d.Files {
				claimed[k] = true
			}
		}

		ds := make(ftdetect.Detectors)
		for _, d := range embedded {
			if replaced[d.Name] {
				continue
			}
			registerDisplacing(ds, d, claimed)
		}
		for _, d := range user {
			ds.RegisterDetector(d)
		}
		cfg.detectors = ds
	})
	return cfg.detectors
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
	// Grammars can pull in other grammars with 'include', which flare resolves
	// through its package-level loader, so point that at this config as well.
	// Otherwise an included grammar would come from a different set than the
	// one being loaded here.
	flare.SetLoader(c.highlighterData)
	return flare.LoadHighlighterBuiltin(name, true)
}

// highlighterData loads a highlighting grammar by filetype name, preferring
// the config directory, then the grammars bundled with mu, then the ones built
// into flare.
func (c *Config) highlighterData(name string) ([]byte, error) {
	if data, err := c.ReadFile(filepath.Join("highlighters", name+".lang")); err == nil {
		return data, nil
	}
	if data, err := flare.BuiltinLoader(name); err == nil {
		return data, nil
	}
	return nil, fmt.Errorf("no highlighter for filetype %q", name)
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
	b.startBackgroundHighlight()
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

// startBackgroundHighlight snapshots the highlight window and fills a memo
// table for it in the background, making subsequent incremental highlights
// fast. Must be called from the main goroutine: the snapshot is what allows
// the highlight itself to run without touching the live buffer.
func (b *Buffer) startBackgroundHighlight() {
	ss := b.syntax
	if ss == nil || ss.highlighter == nil {
		return
	}
	ss.mu.Lock()
	gen := ss.gen
	hlStart, hlEnd := ss.hlStart, ss.hlEnd
	ss.bgActive = true
	ss.pendingEdits = nil
	ss.mu.Unlock()

	data := make([]byte, hlEnd-hlStart)
	copy(data, b.Slice(hlStart, hlEnd))

	go func() {
		ss.hisem.Acquire(context.Background(), 1)
		defer ss.hisem.Release(1)

		// Superseded while queued (the window was re-positioned again):
		// skip the parse entirely, so rapid window jumps cost one full
		// parse instead of queueing one per jump — the pass for the live
		// window would otherwise wait behind every stale one.
		ss.mu.Lock()
		stale := ss.gen != gen
		ss.mu.Unlock()
		if stale {
			return
		}

		tbl := memo.NewTreeTable(512)
		ss.highlighter.HighlightFunc(bytes.NewReader(data), tbl, nil, &vm.Interval{Low: 0, High: 0})

		ss.finishBackground(tbl, gen)

		if b.onHighlight != nil {
			b.onHighlight()
		}
	}()
}

// finishBackground publishes a completed background table if its window
// generation is still current. A stale pass must not touch the pending
// state: a newer pass for the current generation may still be queued and
// needs those edits replayed into its own table.
func (ss *SyntaxState) finishBackground(tbl memo.Table, gen int) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.gen != gen {
		return
	}
	for _, e := range ss.pendingEdits {
		tbl.ApplyEdit(e)
	}
	ss.syntbl = tbl
	ss.minvalid = true
	ss.bgActive = false
	ss.pendingEdits = nil
}

// DisableSyntax drops the buffer's highlighting state (the syntax option).
// All syntax methods no-op on a buffer without state, so rendering falls
// back to plain text. Safe while a background highlight runs: the goroutine
// holds its own reference to the old state and finishes into it.
func (b *Buffer) DisableSyntax() {
	b.syntax = nil
}

// SyntaxReset discards all highlighting state after the buffer content was
// replaced wholesale, and starts a fresh background highlight. Must be
// called from the main goroutine.
func (b *Buffer) SyntaxReset() {
	ss := b.syntax
	if ss == nil {
		return
	}
	ss.mu.Lock()
	ss.setWindow(0, b.Len())
	ss.syntbl = memo.NewTreeTable(512)
	ss.matches = nil
	ss.minvalid = true
	ss.gen++
	ss.bgActive = false
	ss.pendingEdits = nil
	ss.mu.Unlock()
	b.startBackgroundHighlight()
}

// SyntaxApplyEdit updates the syntax state after a text edit.
func (b *Buffer) SyntaxApplyEdit(start, end, insLen int) {
	ss := b.syntax
	if ss == nil {
		return
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()

	delta := insLen - (end - start)
	switch {
	case end <= ss.hlStart && start < ss.hlStart:
		// Entirely before the window: the window's content is unchanged,
		// only its position shifts.
		ss.hlStart += delta
		ss.hlEnd += delta
		ss.coreStart += delta
		ss.coreEnd += delta
	case start >= ss.hlStart && end <= ss.hlEnd:
		// Inside the window: update the memo table.
		edit := memo.Edit{
			Start: start - ss.hlStart,
			End:   end - ss.hlStart,
			Len:   insLen,
		}
		ss.syntbl.ApplyEdit(edit)
		if ss.bgActive {
			ss.pendingEdits = append(ss.pendingEdits, edit)
		}
		ss.minvalid = true
		ss.coreEnd += delta
		ss.hlEnd += delta
	case start >= ss.hlEnd:
		// Entirely after the window: no effect.
	default:
		// Spans a window boundary: the mapping is no longer reliable;
		// reset and let the next highlight rebuild. Any in-flight
		// background pass is now stale (gen changed), so drop its
		// pending-edit state too.
		ss.syntbl = memo.NewTreeTable(512)
		ss.matches = nil
		ss.minvalid = true
		ss.gen++
		ss.bgActive = false
		ss.pendingEdits = nil
		ss.coreEnd += delta
		ss.hlEnd += delta
		if ss.hlEnd < ss.hlStart {
			ss.hlEnd = ss.hlStart
		}
		if ss.coreEnd < ss.coreStart {
			ss.coreEnd = ss.coreStart
		}
	}
}

// SyntaxCheckWindow re-centers the window if the cursor has left the core.
func (b *Buffer) SyntaxCheckWindow(cursorPos int) {
	ss := b.syntax
	if ss == nil || b.Len() <= syntaxWindowSize {
		return
	}
	// The cursor can sit on the phantom line at b.Len(), while a window
	// reaching the end of the buffer has coreEnd == b.Len(): probing with
	// the raw position would count as outside on every frame, resetting
	// the window and spawning a fresh background parse each time — an
	// endless flashing loop, self-sustained by the redraw each completed
	// parse triggers.
	probe := cursorPos
	if probe >= b.Len() {
		probe = b.Len() - 1
	}
	ss.mu.Lock()
	outside := probe < ss.coreStart || probe >= ss.coreEnd
	if outside {
		ss.setWindow(probe, b.Len())
		ss.syntbl = memo.NewTreeTable(512)
		ss.matches = nil
		ss.minvalid = true
		ss.gen++
	}
	ss.mu.Unlock()

	if outside {
		b.startBackgroundHighlight()
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
