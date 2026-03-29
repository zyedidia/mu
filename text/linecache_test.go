package text_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/zyedidia/mu/text"
)

func TestLinerBasic(t *testing.T) {
	data := []byte("abc\ndef\nghi")
	rope := text.NewRope(data, text.DefaultRopeOptions)
	liner := text.NewLiner(rope)

	tests := []struct {
		line, col int
		off       int
	}{
		{0, 0, 0},
		{0, 2, 2},
		{1, 0, 4},
		{1, 2, 6},
		{2, 0, 8},
		{2, 2, 10},
	}

	for _, tt := range tests {
		off := liner.OffsetAt(tt.line, tt.col)
		if off != tt.off {
			t.Errorf("OffsetAt(%d,%d): got %d, want %d", tt.line, tt.col, off, tt.off)
		}
		line, col := liner.LineColAt(tt.off)
		if line != tt.line || col != tt.col {
			t.Errorf("LineColAt(%d): got (%d,%d), want (%d,%d)", tt.off, line, col, tt.line, tt.col)
		}
	}
}

func TestLinerNumLinesAndLen(t *testing.T) {
	data := []byte("a\nb\nc\n")
	rope := text.NewRope(data, text.DefaultRopeOptions)
	liner := text.NewLiner(rope)

	if got := liner.NumLines(); got != 3 {
		t.Fatalf("NumLines: got %d, want 3", got)
	}
	if got := liner.Len(); got != 6 {
		t.Fatalf("Len: got %d, want 6", got)
	}
}

func TestLinerInvalidate(t *testing.T) {
	data := []byte("hello\nworld")
	rope := text.NewRope(data, text.DefaultRopeOptions)
	liner := text.NewLiner(rope)

	// Prime cache
	liner.OffsetAt(0, 0)

	// Invalidate and verify still works
	liner.InvalidateLiner()

	off := liner.OffsetAt(1, 0)
	if off != 6 {
		t.Fatalf("OffsetAt(1,0) after invalidate: got %d, want 6", off)
	}
}

func TestLinerNegativeLine(t *testing.T) {
	data := []byte("hello\nworld")
	rope := text.NewRope(data, text.DefaultRopeOptions)
	liner := text.NewLiner(rope)

	off := liner.OffsetAt(-1, 0)
	if off != 0 {
		t.Fatalf("OffsetAt(-1,0): got %d, want 0", off)
	}
}

func TestLinerManyLines(t *testing.T) {
	// Create data with more lines than lineCacheSize (4096) to test refill
	var sb strings.Builder
	nlines := 5000
	for i := 0; i < nlines; i++ {
		sb.WriteString("line\n")
	}
	data := []byte(sb.String())
	rope := text.NewRope(data, text.DefaultRopeOptions)
	liner := text.NewLiner(rope)

	if got := liner.NumLines(); got != nlines {
		t.Fatalf("NumLines: got %d, want %d", got, nlines)
	}

	// Verify random lookups across cache boundaries
	for i := 0; i < 200; i++ {
		line := rand.Intn(nlines)
		off := liner.OffsetAt(line, 0)
		wantOff := line * 5 // each line is "line\n" = 5 bytes
		if off != wantOff {
			t.Errorf("OffsetAt(%d,0): got %d, want %d", line, off, wantOff)
		}
		gotLine, gotCol := liner.LineColAt(off)
		if gotLine != line || gotCol != 0 {
			t.Errorf("LineColAt(%d): got (%d,%d), want (%d,0)", off, gotLine, gotCol, line)
		}
	}
}

func TestLinerSingleLine(t *testing.T) {
	data := []byte("no newlines here")
	rope := text.NewRope(data, text.DefaultRopeOptions)
	liner := text.NewLiner(rope)

	if got := liner.NumLines(); got != 0 {
		t.Fatalf("NumLines: got %d, want 0", got)
	}

	off := liner.OffsetAt(0, 5)
	if off != 5 {
		t.Fatalf("OffsetAt(0,5): got %d, want 5", off)
	}
	line, col := liner.LineColAt(5)
	if line != 0 || col != 5 {
		t.Fatalf("LineColAt(5): got (%d,%d), want (0,5)", line, col)
	}
}

func TestLinerRoundTrip(t *testing.T) {
	data := randbytes(10000)
	rope := text.NewRope(data, text.DefaultRopeOptions)
	liner := text.NewLiner(rope)

	for i := 0; i < 500; i++ {
		pos := rand.Intn(rope.Len())
		line, col := liner.LineColAt(pos)
		off := liner.OffsetAt(line, col)
		if off != pos {
			t.Fatalf("round trip failed: pos=%d -> (%d,%d) -> %d", pos, line, col, off)
		}
	}
}
