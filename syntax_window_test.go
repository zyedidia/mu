package main

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zyedidia/gpeg/memo"
	"golang.org/x/sync/semaphore"
)

// A stationary cursor on the phantom line at end of file must not re-center
// the highlight window: the previous check treated pos == Len as outside a
// window whose core ends at Len, resetting state and spawning a background
// parse on every frame, forever.
func TestSyntaxWindowEOFStable(t *testing.T) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte(strings.Repeat("x", syntaxWindowSize+4096)))
	b.syntax = &SyntaxState{} // no highlighter: background pass no-ops
	b.syntax.setWindow(0, b.Len())

	pos := b.Len() // cursor on the phantom line
	b.SyntaxCheckWindow(pos)
	g := b.syntax.gen
	if g != 1 {
		t.Fatalf("first check should re-center: gen = %d, want 1", g)
	}
	for i := 0; i < 5; i++ {
		b.SyntaxCheckWindow(pos)
	}
	if b.syntax.gen != g {
		t.Fatalf("stationary EOF cursor re-centers the window: gen %d -> %d", g, b.syntax.gen)
	}

	// Normal re-centering still works.
	b.SyntaxCheckWindow(0)
	if b.syntax.gen != g+1 {
		t.Fatalf("jump to start should re-center: gen = %d, want %d", b.syntax.gen, g+1)
	}
}

// Rapid window jumps queue several background passes; the superseded ones
// must skip their (large) parse and redraw notification, so the pass for
// the live window arrives as fast as a single jump's would.
func TestSyntaxStalePassesSkipped(t *testing.T) {
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	cfg, _ := LoadConfig()
	h, err := cfg.LoadHighlighter("go")
	if err != nil {
		t.Fatal(err)
	}

	b := NewEmptyBuffer()
	b.text.Insert(0, []byte(strings.Repeat("aaaa bbbb cccc dddd\n", (syntaxWindowSize+65536)/20)))

	var notified int32
	b.onHighlight = func() { atomic.AddInt32(&notified, 1) }

	b.syntax = &SyntaxState{
		highlighter: h,
		syntbl:      memo.NewTreeTable(512),
		hisem:       semaphore.NewWeighted(1),
	}
	b.syntax.setWindow(0, b.Len())

	// Park the semaphore so every spawned pass queues behind it, then
	// re-center three times: gens 0..3 all queue, only gen 3 is live.
	ss := b.syntax
	ss.hisem.Acquire(context.Background(), 1)
	b.startBackgroundHighlight() // gen 0
	b.SyntaxCheckWindow(b.Len() - 1)
	b.SyntaxCheckWindow(0)
	b.SyntaxCheckWindow(b.Len() - 1)
	ss.mu.Lock()
	gen := ss.gen
	ss.mu.Unlock()
	if gen != 3 {
		t.Fatalf("gen = %d, want 3 (three re-centers)", gen)
	}
	ss.hisem.Release(1)

	// Only the live pass parses and notifies.
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt32(&notified) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	// Grace period: a regression (stale passes parsing) would deliver
	// additional notifications shortly after.
	time.Sleep(200 * time.Millisecond)
	if n := atomic.LoadInt32(&notified); n != 1 {
		t.Fatalf("notifications = %d, want 1 (stale passes must not parse)", n)
	}
	ss.mu.Lock()
	bg := ss.bgActive
	gen = ss.gen
	ss.mu.Unlock()
	if bg || gen != 3 {
		t.Fatalf("after drain: bgActive=%v gen=%d, want settled at gen 3", bg, gen)
	}
}
