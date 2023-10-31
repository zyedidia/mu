package buffer

import (
	"bufio"
	"io"
	"regexp"
)

// FindDown finds the first match of r in the buffer starting from off and
// searching down from there. It returns the index of the match and the
// submatch indices.
func (b *Buffer) FindDown(r *regexp.Regexp, off int) []int {
	sr := io.NewSectionReader(b.Buffer, int64(off), int64(b.Len()-off))
	br := bufio.NewReader(sr)

	loc := r.FindReaderSubmatchIndex(br)
	if loc == nil && off != 0 {
		return b.FindDown(r, 0)
	}
	for i := range loc {
		loc[i] += off
	}
	return loc
}

// FindUp finds the first match of r in the buffer starting from off and
// searching up from there. It returns the index of the match and the submatch
// indices.
func (b *Buffer) FindUp(r *regexp.Regexp, off int) []int {
	sr := io.NewSectionReader(b.Buffer, 0, int64(off))
	br := bufio.NewReader(sr)
	var last []int
	var start int
	for {
		match := r.FindReaderSubmatchIndex(br)
		if match != nil {
			sr = io.NewSectionReader(b.Buffer, int64(start+match[1]), int64(off))
			if start+match[1] >= off {
				break
			}
			br = bufio.NewReader(sr)
			if last == nil {
				last = make([]int, 2)
			}
			last[0] = start + match[0]
			last[1] = start + match[1]
			start = start + match[1]
		} else {
			break
		}
	}
	if last == nil && off != b.Buffer.Len() {
		return b.FindUp(r, b.Buffer.Len())
	}
	return last
}

// Replace the match from loc with repl. Before replacement, 're.Expand' is
// performed on the replacement slice, which expands submatch identifiers like
// $1. Use $$ for a literal dollar sign. Returns the number of bytes inserted
// for replacement.
func (b *Buffer) Replace(re *regexp.Regexp, loc []int, repl []byte) int {
	if len(loc) < 2 {
		return 0
	}

	dst := expand(re, nil, string(repl), b, loc)
	b.Edit(&Edit{
		Start: loc[0],
		End:   loc[1],
		Text:  dst,
	})
	return len(dst)
}

// ReplaceFunc replaces the match from loc by passing the matched slice through
// a replacement function. Expand is not performed. Returns the number of bytes
// inserted for replacement.
func (b *Buffer) ReplaceFunc(loc []int, repl func([]byte) []byte) int {
	if len(loc) < 2 {
		return 0
	}

	slc := b.Slice(loc[0], loc[1])
	replb := repl(slc)
	b.Edit(&Edit{
		Start: loc[0],
		End:   loc[1],
		Text:  replb,
	})
	return len(replb)
}

// ReplaceLiteral replaces the match from loc with 'repl'. Expand is not
// performed. Returns the number of bytes inserted for replacement.
func (b *Buffer) ReplaceLiteral(loc []int, repl []byte) int {
	if len(loc) < 2 {
		return 0
	}

	b.Edit(&Edit{
		Start: loc[0],
		End:   loc[1],
		Text:  repl,
	})
	return len(repl)
}
