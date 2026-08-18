package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// Realistic Go-like content: indentation, tabs, keywords, strings, and
// comments, so tab expansion, syntax matching, and rendering behave as they
// would on real code.
var benchLines = []string{
	"// Package widget implements the frobnicator over a cell grid.",
	"func (w *Widget) Update(x, y int) error {",
	"\tif x < 0 || y < 0 {",
	"\t\treturn fmt.Errorf(\"invalid coords %d,%d\", x, y)",
	"\t}",
	"\tw.cells[y][x] = computeValue(x, y, w.scale)",
	"\tfor i := range w.observers {",
	"\t\tw.observers[i].Notify(EventUpdate, x, y)",
	"\t}",
	"\treturn nil",
	"}",
	"",
}

func benchText(size int) string {
	var sb strings.Builder
	for i := 0; sb.Len() < size; i++ {
		sb.WriteString(benchLines[i%len(benchLines)])
		sb.WriteByte('\n')
	}
	return sb.String()
}

// benchEditor builds an editor with a simulation screen, the given buffer
// content, and the cursor mid-file, warmed to a steady state so iterations
// measure only per-frame work.
func benchEditor(b *testing.B, content string, wrap, syntax bool) *Editor {
	b.Helper()
	configDirOverride = b.TempDir()
	dataDirOverride = b.TempDir()
	b.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})

	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(sim.Fini)
	sim.SetSize(120, 40)

	ed := newTestEditor()
	ed.screen = sim
	ed.Resize(120, 40)

	buf := ed.ActiveView().buf
	buf.text.Insert(0, []byte(content))
	// Establish the freshly-loaded state (savedHash for small files, none
	// for large ones), as opening a file would.
	buf.markUnmodified()
	*buf.Cursor() = buf.Cursor().MoveTo(buf.Len() / 2)
	ed.ActiveView().SoftWrap = wrap

	if syntax {
		buf.Filetype = "go"
		buf.InitSyntax(ed.config, "go")
	}
	// First frame re-centers the syntax window on the cursor and starts a
	// background pass; wait it out and render once more so the timed
	// frames see steady state.
	ed.Display()
	if syntax {
		waitHighlight(b, buf)
	}
	ed.Display()
	return ed
}

func waitHighlight(b *testing.B, buf *Buffer) {
	b.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		ss := buf.syntax
		ss.mu.Lock()
		done := !ss.bgActive
		ss.mu.Unlock()
		if done {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	b.Fatal("background highlight never finished")
}

// frames runs one full keystroke+redraw per iteration, alternating j/k so
// the cursor stays in place across any number of iterations.
func frames(b *testing.B, ed *Editor) {
	b.Helper()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			ed.dispatchKey("j")
		} else {
			ed.dispatchKey("k")
		}
		ed.Display()
	}
}

// BenchmarkFrame is the headline number: one keystroke (j/k motion) plus
// the full redraw, across the scenario matrix.
func BenchmarkFrame(b *testing.B) {
	cases := []struct {
		name         string
		size         int
		wrap, syntax bool
	}{
		{"small", 4 << 10, false, false},
		{"small_syntax", 4 << 10, false, true},
		{"large", 8 << 20, false, false},
		{"large_syntax", 8 << 20, false, true},
		{"large_wrap", 8 << 20, true, false},
		{"large_wrap_syntax", 8 << 20, true, true},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			ed := benchEditor(b, benchText(c.size), c.wrap, c.syntax)
			frames(b, ed)
		})
	}
}

// BenchmarkFrameGrownBuffer stages a buffer that was small at load time
// (savedHash set) and then grew past the hash cutoff: Modified() re-hashes
// the whole buffer on every status-bar draw, so frames cost O(file). This
// is the stale-hash bug documented in BENCH.md; once fixed, this should
// match BenchmarkFrame/large.
func BenchmarkFrameGrownBuffer(b *testing.B) {
	ed := benchEditor(b, benchText(512<<10), false, false)
	buf := ed.ks.Buf()
	buf.text.Insert(buf.Len(), []byte(benchText(8<<20)))
	*buf.Cursor() = buf.Cursor().MoveTo(buf.Len() / 2)
	ed.Display()
	frames(b, ed)
}

// BenchmarkFrameTyping measures an insert-mode keystroke (buffer edit +
// incremental syntax + redraw) in a large file.
func BenchmarkFrameTyping(b *testing.B) {
	ed := benchEditor(b, benchText(8<<20), false, true)
	ed.dispatchKey("i")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ed.dispatchKey("x")
		ed.Display()
	}
}

// BenchmarkFrameLongLine is the pathological softwrap case: a single
// multi-megabyte line, where per-line geometry walks cover the whole line.
func BenchmarkFrameLongLine(b *testing.B) {
	content := strings.Repeat("lorem ipsum dolor sit amet consectetur ", 50000) // ~2MB, one line
	ed := benchEditor(b, content+"\n", true, false)
	frames(b, ed)
}

// BenchmarkFrameMultiCursor: 8 cursors spread through the file.
func BenchmarkFrameMultiCursor(b *testing.B) {
	ed := benchEditor(b, benchText(1<<20), false, true)
	buf := ed.ks.Buf()
	for i := 1; i < 8; i++ {
		buf.SpawnCursor(buf.Len() / 2 / 8 * i)
	}
	frames(b, ed)
}

// --- Sub-benchmarks isolating the suspected hot paths ---

func BenchmarkRelocate(b *testing.B) {
	ed := benchEditor(b, benchText(8<<20), true, false)
	v := ed.ActiveView()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Relocate()
	}
}

func BenchmarkViewDisplay(b *testing.B) {
	ed := benchEditor(b, benchText(8<<20), false, true)
	v := ed.ActiveView()
	draw := func(x, y int, mainc rune, combc []rune, style Style) {}
	cur := func(x, y int, main bool) {}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Display(draw, cur, ed.theme)
	}
}

func BenchmarkLineColAt(b *testing.B) {
	buf := NewEmptyBuffer()
	buf.text.Insert(0, []byte(benchText(8<<20)))
	pos := buf.Len() / 2
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.LineColAt(pos + i%64)
	}
}

func BenchmarkVisualCol(b *testing.B) {
	buf := NewEmptyBuffer()
	buf.text.Insert(0, []byte(benchText(8<<20)))
	pos := buf.Len() / 2
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.VisualCol(pos + i%32)
	}
}
