package text_test

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/zyedidia/mu/text"
)

// readerAtSlice wraps a byte slice as an io.ReaderAt.
type readerAtSlice []byte

func (r readerAtSlice) ReadAt(p []byte, off int64) (int, error) {
	n := copy(p, r[off:])
	return n, nil
}

func TestReaderBasic(t *testing.T) {
	data := []byte("hello world")
	r := text.NewReader(readerAtSlice(data))

	p := make([]byte, 5)
	n, _ := r.ReadAt(p, 0)
	if n != 5 || string(p) != "hello" {
		t.Fatalf("ReadAt(0,5): got (%d, %q)", n, p[:n])
	}

	n, _ = r.ReadAt(p, 6)
	if n != 5 || string(p) != "world" {
		t.Fatalf("ReadAt(6,5): got (%d, %q)", n, p[:n])
	}
}

func TestReaderByteAt(t *testing.T) {
	data := []byte("abcdef")
	r := text.NewReader(readerAtSlice(data))

	for i, expected := range data {
		got := r.ByteAt(i)
		if got != expected {
			t.Errorf("ByteAt(%d): got %c, want %c", i, got, expected)
		}
	}
}

func TestReaderCachedSlice(t *testing.T) {
	data := []byte("0123456789abcdef")
	r := text.NewReader(readerAtSlice(data))

	got := r.CachedSlice(4, 10)
	want := data[4:10]
	if !bytes.Equal(got, want) {
		t.Fatalf("CachedSlice(4,10): got %q, want %q", got, want)
	}
}

func TestReaderDecodeRuneAt(t *testing.T) {
	data := []byte("aé日")
	r := text.NewReader(readerAtSlice(data))

	ru, sz := r.DecodeRuneAt(0)
	if ru != 'a' || sz != 1 {
		t.Errorf("DecodeRuneAt(0): got (%c, %d)", ru, sz)
	}
	ru, sz = r.DecodeRuneAt(1)
	if ru != 'é' || sz != 2 {
		t.Errorf("DecodeRuneAt(1): got (%c, %d)", ru, sz)
	}
	ru, sz = r.DecodeRuneAt(3)
	if ru != '日' || sz != 3 {
		t.Errorf("DecodeRuneAt(3): got (%c, %d)", ru, sz)
	}
}

func TestReaderInvalidate(t *testing.T) {
	data := []byte("hello")
	r := text.NewReader(readerAtSlice(data))

	// Prime the cache
	r.ByteAt(0)

	// Invalidate and read again — should still work
	r.Invalidate()
	got := r.ByteAt(2)
	if got != 'l' {
		t.Fatalf("ByteAt(2) after invalidate: got %c, want 'l'", got)
	}
}

func TestReaderLargeData(t *testing.T) {
	// Create data larger than cacheBufSize (16384) to test cross-boundary reads
	data := make([]byte, 32768)
	for i := range data {
		data[i] = byte('a' + (i % 26))
	}
	r := text.NewReader(readerAtSlice(data))

	// Read across a cache boundary
	const ncheck = 200
	for i := 0; i < ncheck; i++ {
		start := rand.Intn(len(data) - 100)
		end := start + rand.Intn(100) + 1
		got := r.CachedSlice(start, end)
		want := data[start:end]
		if !bytes.Equal(got, want) {
			t.Fatalf("CachedSlice(%d,%d): mismatch", start, end)
		}
	}
}

func TestReaderCrossBoundary(t *testing.T) {
	// Data that spans exactly at cache boundary (16384)
	data := make([]byte, 20000)
	for i := range data {
		data[i] = byte(i % 256)
	}
	r := text.NewReader(readerAtSlice(data))

	// Read spanning the boundary
	p := make([]byte, 200)
	start := 16384 - 100
	n, _ := r.ReadAt(p, int64(start))
	if n != 200 {
		t.Fatalf("cross-boundary read: got %d bytes, want 200", n)
	}
	if !bytes.Equal(p, data[start:start+200]) {
		t.Fatal("cross-boundary read: data mismatch")
	}
}
