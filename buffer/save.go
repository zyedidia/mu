package buffer

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"fmt"
	"io"

	"github.com/zyedidia/mu/pkg/input"
	"github.com/zyedidia/mu/pkg/output"
)

// An Output represents a device that can be opened for writing, and has a
// named identifier.
type Output interface {
	Open() (io.Writer, error)
	Name() string
	FullName() string
}

// Hash performs an md5 hash on the buffer's contents.
func (b *BufferData) Hash() []byte {
	hasher := md5.New()
	b.WriteTo(hasher)
	return hasher.Sum(nil)
}

// Modified returns true if this buffer has been modified since loading. If the
// file is small enough it will hash the buffer to compare to an initial hash.
// If not it will use a boolean (but this can give a false positive).
func (b *Buffer) Modified() bool {
	if b.modhash != nil {
		return !bytes.Equal(b.Hash(), b.modhash)
	}

	return b.modified
}

// SetOutput changes the target output for saving this buffer.
func (b *Buffer) SetOutput(o Output) {
	b.out = o
}

// FileOutput returns nil if the output is not a file.
func (b *Buffer) FileOutput() *output.File {
	if f, ok := b.out.(*output.File); ok {
		return f
	}
	return nil
}

func (b *Buffer) writeout(o Output) (err error) {
	w, err := o.Open()
	if err != nil {
		return fmt.Errorf("save failed: %w", err)
	}
	defer func() {
		if w, ok := w.(io.Closer); ok {
			e := w.Close()
			if e != nil {
				err = fmt.Errorf("save failed: %w", e)
			}
		}
	}()

	bw := bufio.NewWriter(w)
	_, err = b.WriteTo(bw)
	if err != nil {
		return fmt.Errorf("save failed: %w", err)
	}
	return bw.Flush()
}

// Save writes the contents of this buffer to the stored output. If successful
// it resets the modification status and rehashes the file, if possible.
func (b *Buffer) Save() error {
	err := b.writeout(b.out)
	if err != nil {
		return err
	}
	b.unmodified()
	b.SerializeUndo(b.cfg.CacheFS(), input.EscapePath(b.FullName())+".undo")
	return nil
}
