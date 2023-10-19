// +build darwin,amd64 linux,386 linux,amd64 freebsd,amd64

package input

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"
	"unsafe"

	"github.com/zyedidia/mu/pkg/cpu"
	"github.com/zyedidia/mu/pkg/gommap"
	"github.com/zyedidia/mu/pkg/input/parallel"
)

// An MMapReadFile is a file that is read using mmap instead of read. The
// entire file is read and copied into memory using mmap. This is fully safe
// (unlike the MMapFile, which is entirely accessed via mmap, and thus has
// safety problems detailed below), but is likely no faster than a standard
// read. Nonetheless, this version is actually usable and correct.
type MMapReadFile struct {
	Path string
}

func (mrf *MMapReadFile) Read() ([]byte, error) {
	f, err := os.Open(mrf.Path)
	if errors.Is(err, os.ErrNotExist) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	b, err := gommap.Map(f.Fd(), gommap.PROT_READ, gommap.MAP_PRIVATE)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("mmap: %w", err)
	}
	return mmapToMem(b), nil
}

func (mrf *MMapReadFile) ModTime() (time.Time, error) {
	fi, err := os.Stat(mrf.Path)
	if err != nil {
		return time.Now(), err
	}
	return fi.ModTime(), nil
}

func (mrf *MMapReadFile) Name() string {
	return mrf.Path
}

// An MMapFile is a file that is accessed via memory mapping. We have support
// for mmapping files but there are various problems with the implementation
// because mmap is not a good alternative for read.
//
// The primary issue is that once we mmap a file, if it is modified, then those
// modifications may be reflected in the mapped memory. If we load a file via
// mmap and another process truncates it, then the editor may crash if it loads
// memory that used to correspond to data in the file. When we try to save, we
// first truncate the file, and then write the in-memory buffer into the file.
// With mmap this causes a fault because the in-memory buffer no longer exists
// after truncation.
//
// The other issue with mmap is unmapping. Currently we never unmap because
// Read returns a byte slice, which has no mechanism for a custom "free"
// function. We could return an interface with a Free or Close function and a
// function to fetch the bytes, but this will complicate the code base. It
// seems not worth it, especially given the problems with mmap detailed above.
// In summary, using mmap is not worth the hassle, and likely not better than a
// normal read.
type MMapFile struct {
	Path string
}

func (mf *MMapFile) Read() ([]byte, error) {
	f, err := os.Open(mf.Path)
	if errors.Is(err, os.ErrNotExist) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, err
	}

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// files with size 0 cannot be mmapped
	if fi.Size() == 0 {
		return []byte{}, nil
	}

	b, err := gommap.Map(f.Fd(), gommap.PROT_READ, gommap.MAP_PRIVATE)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("mmap: %w", err)
	}

	// We must call this function now for safety and correctness.
	// Unfortunately, calling this function essentially erases all gains made
	// by using mmap in the first place. Though the OS will report that our
	// application is using no memory for the file (good!), the OS will have
	// allocated the memory for the temp file so the overall computer's memory
	// usage is not actually decreased at all from using mmap.
	return allowModifications(b), nil
}

func (mf *MMapFile) ModTime() (time.Time, error) {
	fi, err := os.Stat(mf.Path)
	if err != nil {
		return time.Now(), err
	}
	return fi.ModTime(), nil
}

func (mf *MMapFile) Name() string {
	return mf.Path
}

// allowModifications ensures that the mmapped data backing the input file
// cannot be modified by changes to the underlying file. This is important
// before saving, since that will modify the underlying file. This is achieved
// by writing the mapped data to a temporary file, unmapping the data, mapping
// the temporary file at the same address the old data was mapped at, and then
// removing the temporary file so that it cannot be changed by another process.
// If creating the temporary file fails, the mapped data is copied entirely
// into memory.
//
// It is forbidden to modify the underlying file that is backing a buffer with
// mmap before this function has been called.
func allowModifications(mmap []byte) []byte {
	tmp, err := ioutil.TempFile("", "micro")
	if err != nil {
		return mmapToMem(mmap)
	}

	_, err = tmp.Write(mmap)
	if err != nil {
		return mmapToMem(mmap)
	}

	// unsafe because we have to remap at the exact pointer we are about to unmap.
	slcptr := uintptr(unsafe.Pointer(&mmap[0]))
	slclen := len(mmap)

	unmap(mmap)

	_, err = gommap.MapAt(slcptr, tmp.Fd(), 0, int64(slclen), gommap.PROT_READ, gommap.MAP_PRIVATE)
	if err != nil {
		// this should really never happen... if the map fails though we can just read
		// back from the temp file into memory
		newb := make([]byte, slclen)
		n, _ := parallel.ReadFull(tmp, newb, cpu.NumCores())
		return newb[:n]
	}
	tmp.Close()
	os.Remove(tmp.Name())

	return mmap
}

func unmap(mmap []byte) {
	m := gommap.MMap(mmap)
	m.UnsafeUnmap()
}

func mmapToMem(b []byte) []byte {
	newb := make([]byte, len(b))
	n, _ := parallel.ReadFull(bytes.NewReader(b), newb, cpu.NumCores())
	unmap(b)
	return newb[:n]
}
