package text

import (
	"bytes"

	"golang.org/x/text/transform"
)

// LineEnding represents a line ending encoding.
type LineEnding byte

const (
	LF   LineEnding = iota
	CRLF LineEnding = iota
)

func (t LineEnding) String() string {
	switch t {
	case LF:
		return "LF"
	default:
		return "CRLF"
	}
}

// DetectLineEnding detects whether a slice of bytes uses LF or CRLF line
// endings. It works by searching for the first LF and checking if there is a
// CR immediately before it.
func DetectLineEnding(b []byte) LineEnding {
	idx := bytes.Index(b, []byte{'\n'})
	if idx == -1 || idx == 0 {
		return LF
	}
	if b[idx-1] == '\r' {
		return CRLF
	}
	return LF
}

// ToLF converts CRLF or CR endings to LF.
type ToLF struct {
	prev byte
}

func (lf *ToLF) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nDst < len(dst) && nSrc < len(src) {
		c := src[nSrc]
		switch c {
		case '\r':
			dst[nDst] = '\n'
		case '\n':
			if lf.prev == '\r' {
				nSrc++
				lf.prev = c
				continue
			}
			dst[nDst] = '\n'
		default:
			dst[nDst] = c
		}
		lf.prev = c
		nDst++
		nSrc++
	}
	if nSrc < len(src) {
		err = transform.ErrShortDst
	}
	return
}

func (lf *ToLF) Reset() {
	lf.prev = 0
}

// ToCRLF converts LF line endings to CRLF line endings.
type ToCRLF struct{}

func (ToCRLF) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nDst < len(dst) && nSrc < len(src) {
		if c := src[nSrc]; c == '\n' {
			if nDst+1 == len(dst) {
				break
			}
			dst[nDst] = '\r'
			dst[nDst+1] = '\n'
			nSrc++
			nDst += 2
		} else {
			dst[nDst] = c
			nSrc++
			nDst++
		}
	}
	if nSrc < len(src) {
		err = transform.ErrShortDst
	}
	return
}

func (ToCRLF) Reset() {}
