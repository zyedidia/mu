package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zyedidia/mu/text"
)

// Tests in this file capture bugs found during a subsystem audit (search,
// registers, history, persistence, config, view, editor shell). Each test
// documents the expected behavior.

// --- Search anchors ---

// ^ anchors to line starts, not to the start of the searched region:
// /^foo from the middle of line 0 must find line 1, not offset 1.
func TestSearchCaretAnchorsToLines(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("xfoo\nfoo\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	ed.searchForward("^foo")
	if b.Cursor().Pos != 5 {
		t.Fatalf("/^foo: pos=%d, want 5", b.Cursor().Pos)
	}
}

// $ anchors to line ends: /foo$ must match "foo" at the end of a line even
// when the buffer continues afterwards.
func TestSearchDollarAnchorsToLines(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("foobar\nfoo\nmore\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	ed.searchForward("foo$")
	if b.Cursor().Pos != 7 {
		t.Fatalf("/foo$: pos=%d, want 7", b.Cursor().Pos)
	}
}

// Substituting an anchored pattern must not re-anchor at every replacement
// offset: s/^a/X on "aaa" gives "Xaa", not "XXX".
func TestSubstituteCaretAnchor(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("aaa\n"))

	ed.RunCommand("s {^a} X all")
	got := string(b.Slice(0, b.Len()))
	if got != "Xaa\n" {
		t.Fatalf("s/^a/X/all: got %q, want %q", got, "Xaa\n")
	}
}

// Backward search must not treat a mid-line restart as a line start:
// ?^foo on "foofoo" matches only offset 0.
func TestFindUpNoFalseCaretAnchor(t *testing.T) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte("foofoo\n"))

	re, err := compileSearch("^foo")
	if err != nil {
		t.Fatal(err)
	}
	loc := b.FindUp(re, b.Len())
	if loc == nil || loc[0] != 0 {
		t.Fatalf("?^foo: loc=%v, want start 0", loc)
	}
}

// A pattern that can match empty must terminate and behave like vim:
// s/x*/Y/g on "abc" gives "YaYbYcY".
func TestSubstituteEmptyMatchTerminates(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("abc"))

	done := make(chan struct{})
	go func() {
		ed.RunCommand("s {x*} Y all")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("substitute with empty-matching pattern did not terminate")
	}
	got := string(b.Slice(0, b.Len()))
	if got != "YaYbYcY" {
		t.Fatalf("s/x*/Y/all: got %q, want %q", got, "YaYbYcY")
	}
}

// * must not leave its count pending for the next command.
func TestStarClearsCount(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("foo bar foo\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	ed.ks.HandleKey("2")
	ed.ks.HandleKey("*")
	if ed.ks.RawCount() != 0 {
		t.Fatalf("* left count=%d pending", ed.ks.RawCount())
	}
}

// --- Registers ---

// Reading an uppercase register reads the corresponding lowercase one.
func TestRegisterUppercaseRead(t *testing.T) {
	rs := NewRegisterSet()
	rs.Set('a', []byte("hello"), false)
	if got := string(rs.Get('A').Content); got != "hello" {
		t.Fatalf(`Get('A') = %q, want "hello"`, got)
	}
}

// Appending charwise text to a linewise register keeps it linewise, with
// the appended text on its own line.
func TestRegisterAppendLinewiseMerge(t *testing.T) {
	rs := NewRegisterSet()
	rs.Set('a', []byte("aa\n"), true)
	rs.Set('A', []byte("bb"), false)
	r := rs.Get('a')
	if !r.Linewise {
		t.Fatal("append to linewise register lost the linewise flag")
	}
	if string(r.Content) != "aa\nbb\n" {
		t.Fatalf("append: content=%q, want %q", r.Content, "aa\nbb\n")
	}
}

// --- Prompt history ---

// promptSearch types a search in the incremental prompt and presses Enter.
func promptSearch(ed *Editor, text string) {
	ed.ks.HandleKey("/")
	for _, ch := range text {
		ed.infobar.HandleKey(string(ch))
	}
	ed.infobar.HandleKey(KeyEnter)
}

// History browsing state must reset between prompt sessions: recalling the
// most recent entry always starts from the newest one.
func TestIncrementalPromptHistoryReset(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("foo bar\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	promptSearch(ed, "foo")
	promptSearch(ed, "bar")

	// Recall "bar" with Up, accept it.
	ed.ks.HandleKey("/")
	ed.infobar.HandleKey(KeyUp)
	ed.infobar.HandleKey(KeyEnter)

	// A fresh prompt must start browsing at the newest entry again.
	ed.ks.HandleKey("/")
	ed.infobar.HandleKey(KeyUp)
	if got := string(ed.infobar.input); got != "bar" {
		t.Fatalf("history recall after prior browse: got %q, want %q", got, "bar")
	}
	ed.infobar.HandleKey(KeyEscape)
}

// Accepting a history-recalled search must actually run the search.
func TestHistoryRecallRunsSearch(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("xx\nfoo\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	promptSearch(ed, "foo")
	if b.Cursor().Pos != 3 {
		t.Fatalf("initial search: pos=%d, want 3", b.Cursor().Pos)
	}

	*b.Cursor() = b.Cursor().MoveTo(0)
	ed.ks.HandleKey("/")
	ed.infobar.HandleKey(KeyUp) // recall "foo"
	ed.infobar.HandleKey(KeyEnter)
	if b.Cursor().Pos != 3 {
		t.Fatalf("recalled search: pos=%d, want 3", b.Cursor().Pos)
	}
}

// --- Cursor persistence ---

// LoadCursorPos restores only the primary cursor position: no selections,
// no extra cursors, and the position clamped to the buffer.
func TestLoadCursorPosSanitized(t *testing.T) {
	dataDirOverride = t.TempDir()
	defer func() { dataDirOverride = "" }()

	path := filepath.Join(t.TempDir(), "f.txt")
	b, _ := NewBuffer([]byte("hello world\n"), path)
	b.SpawnCursor(8)
	b.cursors[0].HasSel = true
	b.cursors[0].Sel = [2]int{2, 9}
	b.cursors[0].Pos = 9
	NewView(b, 4).SaveCursorPos()

	nb, _ := NewBuffer([]byte("hi\n"), path) // shorter file
	NewView(nb, 4).LoadCursorPos()
	if nb.NumCursors() != 1 {
		t.Fatalf("restored %d cursors, want 1", nb.NumCursors())
	}
	c := nb.Cursor()
	if c.HasSel {
		t.Fatal("restored cursor has a phantom selection")
	}
	if c.Pos > nb.Len() {
		t.Fatalf("restored cursor out of range: %d > %d", c.Pos, nb.Len())
	}
}

// --- Config ---

// Glob sections override filetype sections deterministically (PLAN.md:
// glob match > filetype match), regardless of TOML map iteration order.
func TestOptionsGlobOverridesFiletype(t *testing.T) {
	src := `
tabsize = 4

[go]
tabsize = 2

["glob:*.go"]
tabsize = 8
`
	for i := 0; i < 30; i++ {
		opts, err := LoadOptions([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		m := opts.Resolve("x.go", "go")
		if n, _ := GetOptInt(m, "tabsize"); n != 8 {
			t.Fatalf("iteration %d: tabsize=%d, want 8 (glob must win)", i, n)
		}
	}
}

// Opening a file resolves options with its detected filetype, so
// [filetype] sections in options.toml apply.
func TestOpenFileResolvesFiletypeOptions(t *testing.T) {
	ed := newTestEditor()
	ed.config.opts.ft = append(ed.config.opts.ft, ftOpts{
		ft:   "go",
		opts: map[string]any{"tabsize": int64(2)},
	})

	path := filepath.Join(t.TempDir(), "x.go")
	os.WriteFile(path, []byte("package main\n"), 0644)
	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	v := ed.ActiveView()
	if v.buf.Filetype != "go" {
		t.Fatalf("filetype=%q, want go", v.buf.Filetype)
	}
	if n, _ := GetOptInt(v.Opts, "tabsize"); n != 2 {
		t.Fatalf("tabsize=%d, want 2 (from [go] section)", n)
	}
	if v.vis.TabSize != 2 {
		t.Fatalf("visualizer tabsize=%d, want 2", v.vis.TabSize)
	}
}

// --- :set ---

// :set on a buffer-scoped option takes effect on open views.
func TestSetBufferOption(t *testing.T) {
	ed := newTestEditor()
	v := ed.ActiveView()

	ed.RunCommand("set tabsize 8")
	if n, _ := GetOptInt(v.Opts, "tabsize"); n != 8 {
		t.Fatalf("view opts tabsize=%d, want 8", n)
	}
	if v.vis.TabSize != 8 {
		t.Fatalf("visualizer tabsize=%d, want 8", v.vis.TabSize)
	}

	ed.RunCommand("set softwrap true")
	if !v.SoftWrap {
		t.Fatal("softwrap not applied to view")
	}
}

// :set theme with an unknown theme fails without changing the option.
func TestSetThemeInvalidNotApplied(t *testing.T) {
	ed := newTestEditor()
	prev := ed.config.GlobalStrOpt("theme")

	ed.RunCommand("set theme no-such-theme-xyz")
	if got := ed.config.GlobalStrOpt("theme"); got != prev {
		t.Fatalf("invalid theme was stored: %q (was %q)", got, prev)
	}
}

// --- View scrolling ---

// Relocate must be stable when the pane is shorter than twice the scroll
// margin: the viewport may not oscillate between positions.
func TestRelocateStableSmallPane(t *testing.T) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte(strings.Repeat("line\n", 100)))
	v := NewView(b, 4)
	v.ScrollMargin = 5
	v.Resize(80, 8) // height < 2*margin+1

	*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(50, 0))
	v.Relocate()
	top1 := v.topline
	v.Relocate()
	top2 := v.topline
	v.Relocate()
	top3 := v.topline
	if top1 != top2 || top2 != top3 {
		t.Fatalf("viewport oscillates: %d, %d, %d", top1, top2, top3)
	}
	// The cursor must be on screen.
	if 50 < top1 || 50 >= top1+v.height {
		t.Fatalf("cursor line 50 not visible: topline=%d height=%d", top1, v.height)
	}
}

// Horizontal relocation must be stable in very narrow panes.
func TestRelocateStableNarrowPane(t *testing.T) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte(strings.Repeat("x", 200)+"\n"))
	v := NewView(b, 4)
	v.HScrollMargin = 5
	v.LineNums = false
	v.Resize(8, 10) // bufferWidth <= 2*margin

	*b.Cursor() = b.Cursor().MoveTo(100)
	v.Relocate()
	st1 := v.stcol
	v.Relocate()
	st2 := v.stcol
	v.Relocate()
	st3 := v.stcol
	if st1 != st2 || st2 != st3 {
		t.Fatalf("horizontal scroll oscillates: %d, %d, %d", st1, st2, st3)
	}
}

// --- Editor shell ---

// Closing a split restores the surviving pane's own cursor instead of
// leaving the shared buffer wherever the closed pane was.
func TestUnsplitRestoresSurvivorCursor(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte(strings.Repeat("line\n", 20)))
	*b.Cursor() = b.Cursor().MoveTo(0)

	ed.VSplit(nil) // new pane shares the buffer, becomes active
	*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(10, 0))
	ed.ClosePane()

	if got := b.Cursor().Pos; got != 0 {
		t.Fatalf("survivor cursor pos=%d, want 0", got)
	}
}

// When a file changed on disk under a modified buffer, the keystroke that
// triggers the prompt must not also answer it.
func TestExternalModifiedPromptNotConsumed(t *testing.T) {
	ed := newTestEditor()
	path := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(path, []byte("one\n"), 0644)
	if err := ed.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	b := ed.ActiveView().buf
	b.StopWatcher()               // avoid background reloads during the test
	b.Insert(0, []byte("local ")) // modify the buffer

	// Change the file on disk with a newer mtime.
	os.WriteFile(path, []byte("two\n"), 0644)
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(path, future, future)

	prompted := ed.checkExternalModified()
	if !prompted {
		t.Fatal("checkExternalModified did not report the prompt")
	}
	if !ed.infobar.IsActive() {
		t.Fatal("reload prompt is not active")
	}
	// The buffer must still contain the local modification.
	if got := string(b.Slice(0, b.Len())); got != "local one\n" {
		t.Fatalf("buffer changed before the user answered: %q", got)
	}
}

// Opening several files keeps all of them reachable (one tab each).
func TestOpenMultipleFiles(t *testing.T) {
	ed := newTestEditor()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	c := filepath.Join(dir, "c.txt")
	os.WriteFile(a, []byte("aaa\n"), 0644)
	os.WriteFile(c, []byte("ccc\n"), 0644)

	ed.OpenFiles([]string{a, c})
	if len(ed.tabs) != 2 {
		t.Fatalf("got %d tabs, want 2", len(ed.tabs))
	}
	if got := ed.tabs[0].ActiveView().buf.Path; got != a {
		t.Fatalf("tab 0 shows %q, want %q", got, a)
	}
	if got := ed.tabs[1].ActiveView().buf.Path; got != c {
		t.Fatalf("tab 1 shows %q, want %q", got, c)
	}
}

// Replacing the buffer in a pane stops the dropped buffer's file watcher.
func TestReplacedBufferWatcherStopped(t *testing.T) {
	ed := newTestEditor()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	c := filepath.Join(dir, "c.txt")
	os.WriteFile(a, []byte("aaa\n"), 0644)
	os.WriteFile(c, []byte("ccc\n"), 0644)

	if err := ed.OpenFile(a); err != nil {
		t.Fatal(err)
	}
	old := ed.ActiveView().buf
	if old.watchDone == nil {
		t.Fatal("watcher not started for first file")
	}
	if err := ed.OpenFile(c); err != nil {
		t.Fatal(err)
	}
	// The replaced buffer is hidden, not dropped: its watcher keeps the
	// content fresh until the buffer is deleted from the buffer list.
	if old.watchDone == nil {
		t.Fatal("hidden buffer's watcher should keep running")
	}
	ed.deleteBuffer(old)
	if old.watchDone != nil {
		t.Fatal("deleted buffer's watcher still running")
	}
}

// --- H/M/L, Ctrl-E/Ctrl-Y and scroll margins ---

// H and L must respect the scroll margin so they don't force the viewport
// to jump on the next relocate.
func TestHMLRespectScrollMargin(t *testing.T) {
	ed := newTestEditor()
	v := ed.ActiveView()
	b := v.buf
	b.text.Insert(0, []byte(strings.Repeat("line\n", 100)))
	v.ScrollMargin = 5
	v.Resize(80, 22)
	v.topline = 20
	*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(30, 0))

	ed.ks.HandleKey("H")
	line, _ := b.LineColAt(b.Cursor().Pos)
	if line != 25 {
		t.Fatalf("H: line=%d, want 25 (topline+margin)", line)
	}
	v.Relocate()
	if v.topline != 20 {
		t.Fatalf("H moved the viewport: topline=%d, want 20", v.topline)
	}

	ed.ks.HandleKey("L")
	line, _ = b.LineColAt(b.Cursor().Pos)
	if line != 36 {
		t.Fatalf("L: line=%d, want 36 (topline+height-1-margin)", line)
	}
	v.Relocate()
	if v.topline != 20 {
		t.Fatalf("L moved the viewport: topline=%d, want 20", v.topline)
	}
}

// Ctrl-E keeps scrolling by pushing the cursor ahead of the margin instead
// of snapping the viewport back.
func TestCtrlEPushesCursor(t *testing.T) {
	ed := newTestEditor()
	v := ed.ActiveView()
	b := v.buf
	b.text.Insert(0, []byte(strings.Repeat("line\n", 100)))
	v.ScrollMargin = 5
	v.Resize(80, 22)
	v.topline = 20
	*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(27, 0))

	for i := 0; i < 3; i++ {
		ed.ks.HandleKey("<C-e>")
	}
	if v.topline != 23 {
		t.Fatalf("C-e x3: topline=%d, want 23", v.topline)
	}
	line, _ := b.LineColAt(b.Cursor().Pos)
	if line != 28 {
		t.Fatalf("C-e x3: cursor line=%d, want 28 (pushed to margin)", line)
	}
	v.Relocate()
	if v.topline != 23 {
		t.Fatalf("relocate after C-e snapped back: topline=%d, want 23", v.topline)
	}
}

// Ctrl-Y symmetrically pushes the cursor up ahead of the bottom margin.
func TestCtrlYPushesCursor(t *testing.T) {
	ed := newTestEditor()
	v := ed.ActiveView()
	b := v.buf
	b.text.Insert(0, []byte(strings.Repeat("line\n", 100)))
	v.ScrollMargin = 5
	v.Resize(80, 22)
	v.topline = 23
	*b.Cursor() = b.Cursor().MoveTo(b.OffsetAt(28, 0))

	for i := 0; i < 15; i++ {
		ed.ks.HandleKey("<C-y>")
	}
	if v.topline != 8 {
		t.Fatalf("C-y x15: topline=%d, want 8", v.topline)
	}
	line, _ := b.LineColAt(b.Cursor().Pos)
	if line != 24 {
		t.Fatalf("C-y x15: cursor line=%d, want 24 (pushed to bottom margin)", line)
	}
	v.Relocate()
	if v.topline != 8 {
		t.Fatalf("relocate after C-y snapped back: topline=%d, want 8", v.topline)
	}
}

// --- Diagnostic gutter styles ---

// The gutter uses the gutter-error/gutter-warning theme groups (which the
// shipped themes define), falling back to error/warning.
func TestDiagnosticGutterThemeGroups(t *testing.T) {
	th, err := LoadThemeYAML([]byte(`
default:
  fg: white
gutter-warning:
  fg: yellow
error:
  fg: red
`))
	if err != nil {
		t.Fatal(err)
	}
	warn := diagGutterStyle(th, DiagWarning)
	if warn != th.Style("gutter-warning") {
		t.Fatalf("warning gutter style not taken from gutter-warning group")
	}
	errStyle := diagGutterStyle(th, DiagError)
	if errStyle != th.Style("error") {
		t.Fatalf("error gutter style should fall back to error group")
	}
}

func TestInlayHintGutterThemeGroups(t *testing.T) {
	th, err := LoadThemeYAML([]byte(`
default:
  fg: white
comment:
  fg: gray
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := inlayHintGutterStyle(th); got != th.Style("comment") {
		t.Fatalf("inlay hint gutter style should fall back to comment group")
	}

	th2, err := LoadThemeYAML([]byte(`
default:
  fg: white
gutter-hint:
  fg: cyan
comment:
  fg: gray
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := inlayHintGutterStyle(th2); got != th2.Style("gutter-hint") {
		t.Fatalf("inlay hint gutter style not taken from gutter-hint group")
	}
}

// --- Tab bar ---

// The active tab must always be within the drawn range of the tab bar.
func TestTabBarActiveVisible(t *testing.T) {
	widths := []int{30, 30, 30, 30}
	start := tabBarScroll(widths, 3, 80)
	// Drawing from `start`, tab 3 must fit within 80 columns.
	sum := 0
	for i := start; i < 3; i++ {
		sum += widths[i]
	}
	if sum+widths[3] > 80 {
		t.Fatalf("active tab not visible: start=%d", start)
	}
	// First tab active: no scrolling.
	if got := tabBarScroll(widths, 0, 80); got != 0 {
		t.Fatalf("start=%d, want 0", got)
	}
}

// --- Save ---

// Saving preserves the file's permission bits (the atomic temp-file path
// must copy them over).
func TestSavePreservesPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.sh")
	os.WriteFile(path, []byte("old\n"), 0755)

	b, _ := NewBuffer([]byte("old\n"), path)
	b.text.Insert(0, []byte("new "))
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0755 {
		t.Fatalf("permissions changed: %v, want 0755", fi.Mode().Perm())
	}
	data, _ := os.ReadFile(path)
	if string(data) != "new old\n" {
		t.Fatalf("content: %q", data)
	}
}

// Saving through a symlink updates the target and keeps the link a link.
func TestSaveThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	os.WriteFile(target, []byte("old\n"), 0644)
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported")
	}

	b, _ := NewBuffer([]byte("old\n"), link)
	b.text.Insert(0, []byte("new "))
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced by a regular file")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "new old\n" {
		t.Fatalf("target content: %q", data)
	}
}

// Saving must not leave temp files behind.
func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	b, _ := NewBuffer([]byte("hello\n"), path)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("stray files after save: %v", names)
	}
}

// --- Diff bounds ---

type byteIndexer []byte

func (b byteIndexer) ByteAt(i int) byte { return b[i] }
func (b byteIndexer) Len() int          { return len(b) }

// DiffBounded gives up quickly on very dissimilar inputs.
func TestDiffBoundedBailsOut(t *testing.T) {
	a := make([]byte, 64*1024)
	c := make([]byte, 64*1024)
	for i := range a {
		a[i] = byte('a' + i%13)
		c[i] = byte('A' + i%17)
	}
	done := make(chan bool, 1)
	go func() {
		_, ok := DiffBounded(byteIndexer(a), byteIndexer(c), maxReloadDiffNodes)
		done <- ok
	}()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("expected DiffBounded to give up on dissimilar inputs")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DiffBounded did not return within 5s")
	}
}

// SetContent falls back to wholesale replacement instead of hanging when
// the new content is completely different (e.g. git checkout of a
// generated file picked up by the auto-reloader).
func TestSetContentDissimilarFallsBack(t *testing.T) {
	a := make([]byte, 100*1024)
	c := make([]byte, 100*1024)
	for i := range a {
		a[i] = byte('a' + i%13)
		c[i] = byte('A' + i%17)
	}
	b, _ := NewBuffer(a, "")
	newb, _ := text.NewBuffer(c, b.text.Opts)

	done := make(chan struct{})
	go func() {
		b.SetContent(newb)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("SetContent did not finish within 10s")
	}
	if !bytes.Equal(b.Slice(0, b.Len()), c) {
		t.Fatal("content not replaced")
	}
}

// --- Rendering fills ---

// End-of-line fills (cursorline background) must extend to the visible
// right edge when the view is horizontally scrolled.
func TestRenderFillWithHorizontalScroll(t *testing.T) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte("abcdef\n"))

	vis := Visualizer{TabSize: 4, CharMap: map[rune]string{}}
	maxX := -1
	b.RenderForward(RenderTracker{
		FillTo: 9, // stcol(6) + width(3)
		Draw: func(bx, by, vx, vy int, mainc rune, combc []rune, style Style) {
			if vy == 0 && vx > maxX {
				maxX = vx
			}
		},
	}, &vis, 3, 5, 0, false, false, DefaultTheme)

	// The line has 6 chars; the fill must continue through column 8.
	if maxX < 8 {
		t.Fatalf("fill stopped at column %d, want through 8", maxX)
	}
}

// --- Syntax highlighting ---

// Edits before the syntax window shift the window instead of corrupting
// the offset mapping.
func TestSyntaxEditBeforeWindowShifts(t *testing.T) {
	ss := &SyntaxState{
		coreStart: 5000, coreEnd: 6000,
		hlStart: 4000, hlEnd: 7000,
		syntbl: nil,
	}
	b := NewEmptyBuffer()
	b.syntax = ss

	// Delete 100 bytes well before the window.
	b.SyntaxApplyEdit(100, 200, 0)
	if ss.hlStart != 3900 || ss.hlEnd != 6900 || ss.coreStart != 4900 || ss.coreEnd != 5900 {
		t.Fatalf("window not shifted: hl=[%d,%d) core=[%d,%d)",
			ss.hlStart, ss.hlEnd, ss.coreStart, ss.coreEnd)
	}
	// Insert 50 bytes before the window.
	b.SyntaxApplyEdit(100, 100, 50)
	if ss.hlStart != 3950 || ss.hlEnd != 6950 {
		t.Fatalf("window not shifted by insert: hl=[%d,%d)", ss.hlStart, ss.hlEnd)
	}
	// An edit entirely after the window is a no-op.
	b.SyntaxApplyEdit(8000, 8100, 0)
	if ss.hlStart != 3950 || ss.hlEnd != 6950 {
		t.Fatalf("after-window edit moved window: hl=[%d,%d)", ss.hlStart, ss.hlEnd)
	}
}

// Editing while the background highlight runs must be race-free (run with
// -race) and produce correct highlighting state.
func TestSyntaxConcurrentEdits(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	src := strings.Repeat("func foo() int {\n\treturn 42 // comment\n}\n\n", 2000)
	b, _ := NewBuffer([]byte(src), "x.go")
	b.InitSyntax(cfg, "go")
	if b.syntax == nil {
		t.Skip("go highlighter unavailable")
	}

	// Edit while the initial background highlight is running.
	for i := 0; i < 200; i++ {
		b.Insert(0, []byte("x"))
		b.Remove(0, 1)
	}

	// Wait for the background pass and render once.
	b.syntax.hisem.Acquire(context.Background(), 1)
	b.syntax.hisem.Release(1)
	b.HighlightRange(0, 200)
	b.SyntaxGroup(0)
}
