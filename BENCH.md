# Benchmarking mu

The performance suite lives in `perf_bench_test.go`. It is fully
self-contained: it generates its own buffer content, renders into a tcell
`SimulationScreen`, and isolates config/data directories into temp dirs, so
every number below is rederivable on any machine with `go test` alone — no
terminal, no fixtures, no network. Absolute numbers vary with the CPU;
ratios and findings reproduce.

## What a "frame" is

The headline benchmarks measure mu's real unit of work: **one keystroke plus
one full redraw** — `dispatchKey` (the vim state machine) followed by
`Display()` (relocate, syntax window check, render walk, line numbers,
gutter, status bar, tcell diff + `Show`). The viewport is 120×40. Two things
are deliberately excluded: terminal I/O (the simulation screen diffs but
writes nowhere) and LSP servers (none are attached).

The run loop coalesces queued input (all pending events are handled, then
one frame renders), so real-world latency under key repeat is one frame,
not one frame per key. The benchmarks measure the per-frame cost itself.

Frame-time budget for judging results, based on keystroke echo latency:

- **≤ 2 ms** excellent — invisible even on key repeat
- **≤ 8 ms** fine — under a 120Hz refresh
- **> 16 ms** perceptible lag

Allocations matter as much as time: per-frame garbage becomes GC pauses
under sustained typing. Watch the `B/op` and `allocs/op` columns.

## Running

```sh
# The full suite (benchmarks are excluded from plain `go test ./...`):
go test -run '^$' -bench . -benchmem .

# The headline scenario matrix only:
go test -run '^$' -bench '^BenchmarkFrame$' -benchmem .

# One scenario. ANCHOR THE PATTERN: bench names match by unanchored
# substring per slash-segment, so -bench 'BenchmarkFrame/large' also runs
# BenchmarkFrameTyping, BenchmarkFrameLongLine, etc., and any profile you
# collect is contaminated.
go test -run '^$' -bench '^BenchmarkFrame$/^large$' -benchmem .

# Stable comparisons (before/after a fix): multiple runs + benchstat.
go test -run '^$' -bench '^BenchmarkFrame$' -benchmem -count=10 . > old.txt
# ...apply change...
go test -run '^$' -bench '^BenchmarkFrame$' -benchmem -count=10 . > new.txt
benchstat old.txt new.txt   # golang.org/x/perf/cmd/benchstat
```

`-benchtime=300x` (a fixed iteration count) is useful while iterating;
default time-based runs are better for final numbers.

## Profiling

```sh
go test -run '^$' -bench '^BenchmarkFrame$/^large$' -benchtime=2000x \
    -cpuprofile cpu.out -memprofile mem.out .

go tool pprof -top -nodecount=20 mu.test cpu.out
go tool pprof -top -sample_index=alloc_space mu.test mem.out
go tool pprof -peek 'RenderForward' mu.test cpu.out   # callers/callees
go tool pprof -http :8080 mu.test cpu.out             # flame graph
```

Profile exactly one benchmark at a time (anchored pattern), or the profile
mixes scenarios.

## Scenarios

| Benchmark | What it isolates |
| --- | --- |
| `Frame/small`, `/small_syntax` | Baseline viewport cost, 4KB file |
| `Frame/large*` (8MB, ×wrap ×syntax) | Whether frame cost scales with file size instead of viewport |
| `FrameGrownBuffer` | The stale-hash bug: buffer loaded small, grown past `hashCutoff` — `Modified()` re-hashes the whole buffer every frame |
| `FrameTyping` | Insert keystroke: rope edit + undo record + incremental syntax + redraw |
| `FrameLongLine` | Pathological softwrap: one ~2MB line (think minified JSON) |
| `FrameMultiCursor` | 8 cursors spread through a 1MB file |
| `Relocate`, `ViewDisplay` | The two halves of a redraw, isolated |
| `HighlightWindow` | Full syntax-window parse (flare+gpeg, fresh memo table): the delay before colors appear after a long jump |
| `LineColAt`, `VisualCol` | Per-call cost of the buffer position primitives |

Content is realistic Go-like code (tabs, keywords, strings, comments) so tab
expansion, grapheme decoding, and the highlighter behave as on real files.
When adding scenarios keep that property — `strings.Repeat("x", n)` flatters
every one of those paths.

## Baseline results (2026-08, Ryzen 9 7950X, after the fixes below)

Reference points, not targets — rerun on your machine before comparing:

| Scenario | ns/frame | B/op |
| --- | --- | --- |
| small | 0.35 ms | 24 KB |
| large (8MB) | 0.36 ms | 24 KB |
| large + wrap + syntax | 0.37 ms | 31 KB |
| grown buffer | 0.34 ms | 24 KB |
| typing, 8MB + syntax | 1.7 ms | 137 KB |
| 8 cursors, 1MB | 1.1 ms | 35 KB |
| one 2MB line, wrap | 0.70 ms | 44 KB |

## Fixed findings (the scenarios above are their regression guards)

1. **`Modified()` hashed the buffer every frame** (`buffer.go`). The
   status bar calls `Modified()` per frame; with `savedHash` set it
   md5-hashed the whole content — ~1ms/frame for files near `hashCutoff`,
   and O(file) forever for a buffer grown past the cutoff since load
   (stale hash). Fixed: a different length decides without hashing, and
   the hash verdict is cached per edit generation. `FrameGrownBuffer`
   went 7.96 ms → 0.55 ms and must stay equal to `Frame/large`.
2. **Every rope read allocated** (`text/rope.go`). `Rope.ReadAt` was
   built on `Slice`, which concatenates leaves into a fresh allocation —
   ~86% of all frame allocation. Fixed with a piecewise `readInto`.
   `Frame/large` allocation went 1.27 MB → 121 KB per frame.
3. **Long-line softwrap geometry** (`view.go`, `display.go`). Each
   geometry helper re-walked the line from its start, several times per
   frame, and `RenderForward` queried `LineColAt` per character. Fixed:
   per-line row-start cache (`View.rowStarts`, keyed on edit generation,
   length, and width; helpers now binary-search it and walk at most one
   row) and incremental line/col tracking in the render walk.
   `FrameLongLine` went 490 ms → 1.1 ms.
4. **Per-cell style resolution** (`theme.go`, `view.go`, `editor.go`).
   `Theme.Style` re-parsed group names per drawn cell and every cell paid
   a `TCellStyle` conversion; line numbers went through `fmt.Sprintf`.
   Fixed: memoized style lookups (copy-on-write cache), a last-style memo
   in the draw path, fmt-free line numbers, and reused per-frame scratch.
   `ViewDisplay` went 414 µs → 99 µs and syntax highlighting now adds
   almost nothing per frame.
5. **Scroll-frame overheads** (`syntax.go`, `view.go`, `editor.go`,
   `display.go`). Highlight matches were extracted for exactly the
   visible range, so scrolling re-ran the match VM every frame — they now
   carry a 16KB margin on scroll-triggered recomputes (edit-triggered
   ones stay viewport-sized, since edits invalidate every keystroke). The
   per-frame whole-screen `Fill` is gone: components tile the screen
   completely (views blank past end-of-buffer, bars pad their rows). The
   render walk memoizes the last syntax group's resolved style. Together:
   motion frames ~0.50 → ~0.36 ms and per-frame allocation 96 → 31 KB.

Remaining known costs, in likely-impact order:

- Editing a huge single line rebuilds its row-start cache next frame (one
  full-line walk — order 100ms on a 2MB line), so *typing* in such a line
  is still slow per keystroke; motion and scrolling are cached.
- The `text.Reader` takes a mutex per rune and grapheme decoding
  allocates per character — only visible inside full-line walks now.
- Most remaining allocs/op are the *simulation screen* allocating per
  changed cell (`simscreen.drawCell`) — a benchmark artifact; the real
  terminal backend has a different write path.

Healthy: syntax adds only 0.05–0.2ms (windowed memoization works), softwrap
is nearly free, and frame time is viewport-bound, not file-bound.

Note: profiles collected with `-cpuprofile` include benchmark *setup* — for
the syntax scenarios that is a full window parse (several seconds of gpeg
on slow machines). Prune with `-ignore 'HighlightFunc'` or profile
`HighlightWindow` separately when frames are the question.

The gpeg VM itself was optimized in tandem (upstreamed to
github.com/zyedidia/gpeg; the working clone lives in reference/gpeg):
in-place stack pushes, pre-decoded instructions,
a span fast path, inlinable memo-shift checks, and a zero-alloc checker
slice. Window parse: 115.6 → 82.4 ms and 500k → 10.8k allocs/op on x86.
