// +build !darwin !amd64
// +build !linux !386
// +build !linux !amd64
// +build !freebsd !amd64

package input

// On systems that do not support mmap, we just use the normal file
// implementation.
type MMapReadFile = File
type MMapFile = File
