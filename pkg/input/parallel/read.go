// Package parallel provides functionality for reading from an io.ReaderAt in
// parallel.
package parallel

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

// ReadFull reads the contents of 'r' into 'buf' with 'ncpu' goroutines. The
// length of buf must exactly determine the size of the reader. Returns
// err == nil on success, not err == EOF.
func ReadFull(r io.ReaderAt, buf []byte, ncpu int) (n int, err error) {
	var ntotal int64
	chunksz := len(buf) / ncpu
	errs := make(chan error, ncpu)

	var wg sync.WaitGroup
	wg.Add(ncpu)
	for t := 0; t < ncpu; t++ {
		go func(t int) {
			defer wg.Done()

			start := t * chunksz
			size := chunksz
			if t == ncpu-1 {
				size = len(buf) - start
			}

			rn, rerr := r.ReadAt(buf[start:start+size], int64(start))
			atomic.AddInt64(&ntotal, int64(rn))
			if rerr != nil {
				errs <- rerr
			}
		}(t)
	}
	wg.Wait()
	close(errs)

	for err = range errs {
		if !errors.Is(err, io.EOF) {
			return int(ntotal), err
		}
	}
	return int(ntotal), nil
}
