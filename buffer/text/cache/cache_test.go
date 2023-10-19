package cache_test

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/zyedidia/mu/buffer/text/cache"
)

var letters = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randbytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return b
}

func check(want, got []byte, t *testing.T) {
	if !bytes.Equal(want, got) {
		t.Errorf("incorrect slices: want %s, got %s", string(want), string(got))
	}
}

func TestSlice(t *testing.T) {
	b := randbytes(100)
	br := bytes.NewReader(b)
	r := cache.NewReader(br)

	const ncheck = 100
	for i := 0; i < ncheck; i++ {
		start, end := rand.Intn(len(b)), rand.Intn(len(b))
		if end < start {
			start, end = end, start
		}
		p1 := r.Slice(start, end)
		p2 := b[start:end]

		check(p1, p2, t)
	}
}

func TestAt(t *testing.T) {
	b := randbytes(4096 * 2)
	br := bytes.NewReader(b)
	r := cache.NewReader(br)

	const ncheck = 100
	for i := 0; i < ncheck; i++ {
		pos := rand.Intn(len(b))
		b1 := r.At(pos)
		b2 := b[pos]

		if b1 != b2 {
			t.Errorf("incorrect value at %d: want %c, got %c", pos, b2, b1)
		}
	}
}

func TestReadAt(t *testing.T) {
	b := randbytes(4096 * 2)
	br := bytes.NewReader(b)
	r := cache.NewReader(br)

	const ncheck = 100
	for i := 0; i < ncheck; i++ {
		off := rand.Intn(len(b))
		sz := rand.Intn(len(b))

		p1 := make([]byte, sz)
		p2 := make([]byte, sz)

		n1, _ := br.ReadAt(p1, int64(off))
		n2, _ := r.ReadAt(p2, int64(off))

		check(p1[:n1], p2[:n2], t)
	}

}
