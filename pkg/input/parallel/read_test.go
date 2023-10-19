package parallel_test

import (
	"bytes"
	"math/rand"
	"runtime"
	"testing"

	"github.com/zyedidia/mu/pkg/input/parallel"
)

var letters = []byte("\nabcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randbytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return b
}

func TestReadFull(t *testing.T) {
	const datasz = 5000

	databuf := randbytes(datasz)
	data := bytes.NewReader(databuf)

	buf := make([]byte, data.Size())
	parallel.ReadFull(data, buf, runtime.NumCPU())

	if len(buf) != len(databuf) {
		t.Errorf("incorrect length: want %d, got %d", len(databuf), len(buf))
	}

	for i := 0; i < len(buf); i++ {
		if buf[i] != databuf[i] {
			t.Errorf("incorrect value at index %d: want %c, got %c", i, databuf[i], buf[i])
		}
	}
}
