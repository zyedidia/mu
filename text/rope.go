package text

import (
	"bytes"
	"io"
	"runtime"
	"sync"
)

// DefaultRopeOptions provides sensible defaults for rope construction.
var DefaultRopeOptions = RopeOptions{
	SplitLen:       4096,
	JoinLen:        2048,
	RebalanceRatio: 1.2,
	LineSep:        []byte{'\n'},
}

// RopeOptions configures the behavior of the rope data structure.
type RopeOptions struct {
	// SplitLen is the threshold above which slices will be split into separate
	// nodes.
	SplitLen int
	// JoinLen is the threshold below which nodes will be merged into slices.
	JoinLen int
	// RebalanceRatio is the threshold used to trigger a rebuild during a
	// rebalance operation.
	RebalanceRatio float64
	// LineSep is the newline byte sequence (usually '\n').
	LineSep []byte
}

type nodeType byte

const (
	tLeaf nodeType = iota
	tNode
)

// A Rope is a node in the rope tree structure. If the kind is tLeaf, only the
// value and length are valid, and if the kind is tNode, only length, left,
// right are valid.
type Rope struct {
	kind        nodeType
	value       []byte
	length      int
	llength     loc
	left, right *Rope
	opts        RopeOptions
}

// NewRope returns a new rope from the given byte slice. The underlying data is
// not copied so the user should ensure that it is okay to insert and delete
// from the input slice.
func NewRope(b []byte, opts RopeOptions) *Rope {
	// We build the tree from the bottom up for extra efficiency. This avoids
	// counting duplicate newlines a logarithmic number of times (for each
	// level of the tree).
	//
	// We make the chunk size equal to SplitLength which means a node will be
	// split when the first edit is made. Since most nodes will never be
	// edited, it makes sense to fill them all up to avoid wasting space, even
	// if it means inserting will require a split the first time a node is
	// edited.
	chunksz := opts.SplitLen
	nchunks := len(b) / chunksz
	nodes := make([]*Rope, nchunks, nchunks+1)

	// For even better performance, we load the chunks in parallel. Chunk
	// loading is distributed among the cores available on the machine.
	var nthreads = runtime.NumCPU()
	var wg sync.WaitGroup
	wg.Add(nthreads)
	for t := 0; t < nthreads; t++ {
		go func(t int) {
			start := t * (nchunks / nthreads)
			end := t*(nchunks/nthreads) + (nchunks / nthreads)
			if t == nthreads-1 {
				end = nchunks
			}
			for i := start; i < end; i++ {
				j := i * chunksz
				// triple index slice notation allows a sort of copy-on-write
				// behavior which is extremely beneficial to us because it's
				// likely that this slice is backed by a memory-mapped file.
				slc := b[j : j+chunksz : j+chunksz]
				nodes[i] = &Rope{
					kind:    tLeaf,
					value:   slc,
					length:  len(slc),
					llength: llen(slc, opts.LineSep),
					opts:    opts,
				}
			}
			wg.Done()
		}(t)
	}
	wg.Wait()
	// load any extra bytes
	slc := b[nchunks*chunksz : len(b) : len(b)]
	nodes = append(nodes, &Rope{
		kind:    tLeaf,
		value:   slc,
		length:  len(slc),
		llength: llen(slc, opts.LineSep),
		opts:    opts,
	})
	return buildTree(nodes)
}

// recursively creates parent nodes
func buildTree(nodes []*Rope) *Rope {
	if len(nodes) == 1 {
		return nodes[0]
	}
	if len(nodes)%2 != 0 {
		l := len(nodes)
		nodes[l-2] = joinRope(nodes[l-2], nodes[l-1])
		nodes = nodes[:l-1]
	}

	newnodes := make([]*Rope, 0, len(nodes)/2+1)
	for i := 0; i < len(nodes); i += 2 {
		newnodes = append(newnodes, joinRope(nodes[i], nodes[i+1]))
	}
	return buildTree(newnodes)
}

// Len returns the number of bytes stored in the rope.
func (n *Rope) Len() int {
	return n.length
}

// LLen returns the line/col location one byte beyond the last position.
func (n *Rope) LLen() (lines, cols int) {
	return n.llength.line, n.llength.col
}

// NumLines returns the number of lines in the rope.
func (n *Rope) NumLines() int {
	return n.llength.line
}

func (n *Rope) adjust() {
	switch n.kind {
	case tLeaf:
		if n.length > n.opts.SplitLen {
			divide := n.length / 2
			n.left = NewRope(n.value[:divide], n.opts)
			n.right = NewRope(n.value[divide:], n.opts)
			n.value = nil
			n.kind = tNode
			n.length = n.left.length + n.right.length
			n.llength = addlocs(n.left.llength, n.right.llength)
		}
	default: // case tNode
		if n.length < n.opts.JoinLen {
			n.value = n.Value()
			n.left = nil
			n.right = nil
			n.kind = tLeaf
			n.length = len(n.value)
			n.llength = llen(n.value, n.opts.LineSep)
		}
	}
}

// Value returns the elements of this node concatenated into a slice. May
// return the underlying slice without copying, so do not modify the returned
// slice.
func (n *Rope) Value() []byte {
	switch n.kind {
	case tLeaf:
		return n.value
	default: // case tNode
		return sliceConcat(n.left.Value(), n.right.Value())
	}
}

// Remove deletes the range [start:end) (exclusive bound) from the rope.
func (n *Rope) Remove(start, end int) {
	switch n.kind {
	case tLeaf:
		// slice tricks delete
		n.value = sliceRemove(n.value, start, end)
		n.length = len(n.value)
		n.llength = llen(n.value, n.opts.LineSep)
	default: // case tNode
		leftLength := n.left.length
		leftStart := min(start, leftLength)
		leftEnd := min(end, leftLength)
		rightLength := n.right.length
		rightStart := max(0, min(start-leftLength, rightLength))
		rightEnd := max(0, min(end-leftLength, rightLength))
		if leftStart < leftLength {
			n.left.Remove(leftStart, leftEnd)
		}
		if rightEnd > 0 {
			n.right.Remove(rightStart, rightEnd)
		}
		n.length = n.left.length + n.right.length
		n.llength = addlocs(n.left.llength, n.right.llength)
	}
	n.adjust()
}

// Insert inserts the given value at pos.
func (n *Rope) Insert(pos int, value []byte) {
	switch n.kind {
	case tLeaf:
		// slice tricks insert
		n.value = sliceInsert(n.value, pos, value)
		n.length = len(n.value)
		n.llength = llen(n.value, n.opts.LineSep)
	default: // case tNode
		leftLength := n.left.length
		if pos < leftLength {
			n.left.Insert(pos, value)
		} else {
			n.right.Insert(pos-leftLength, value)
		}
		n.length = n.left.length + n.right.length
		n.llength = addlocs(n.left.llength, n.right.llength)
	}
	n.adjust()
}

// Slice returns the range of the rope from [start:end).
func (n *Rope) Slice(start, end int) []byte {
	if start >= end {
		return []byte{}
	}

	switch n.kind {
	case tLeaf:
		return n.value[start:end]
	default: // case tNode
		leftLength := n.left.length
		leftStart := min(start, leftLength)
		leftEnd := min(end, leftLength)
		rightLength := n.right.length
		rightStart := max(0, min(start-leftLength, rightLength))
		rightEnd := max(0, min(end-leftLength, rightLength))

		if leftStart != leftEnd {
			if rightStart != rightEnd {
				return sliceConcat(n.left.Slice(leftStart, leftEnd), n.right.Slice(rightStart, rightEnd))
			} else {
				return n.left.Slice(leftStart, leftEnd)
			}
		} else {
			if rightStart != rightEnd {
				return n.right.Slice(rightStart, rightEnd)
			} else {
				return []byte{}
			}
		}
	}
}

// OffsetAt returns the absolute byte offset of a line/col position.
func (n *Rope) OffsetAt(line, col int) int {
	if line < 0 {
		return 0
	}

	pos := loc{line, col}
	switch n.kind {
	case tLeaf:
		idx := indexN(n.value, n.opts.LineSep, line)
		if idx == -1 {
			if line > 0 {
				return n.length
			}
			return col
		}
		return idx + len(n.opts.LineSep) + col
	default: // case tNode
		leftLength := n.left.llength
		if pos.cmp(leftLength) < 0 {
			return n.left.OffsetAt(line, col)
		} else {
			l := sublocs(pos, leftLength)
			return n.left.length + n.right.OffsetAt(l.line, l.col)
		}
	}
}

// LineColAt returns the line/col position of an absolute byte offset.
func (n *Rope) LineColAt(pos int) (line, col int) {
	l := n.lineColAt(pos)
	return l.line, l.col
}

func (n *Rope) lineColAt(pos int) loc {
	switch n.kind {
	case tLeaf:
		return lineCol(n.value, n.opts.LineSep, pos)
	default: // case tNode
		leftLength := n.left.length
		if pos < leftLength {
			return n.left.lineColAt(pos)
		} else {
			return addlocs(n.left.llength, n.right.lineColAt(pos-leftLength))
		}
	}
}

// SliceLC is the same as Slice but uses line/col positions for start and end.
func (n *Rope) SliceLC(startl, startc, endl, endc int) []byte {
	return n.sliceLC(loc{startl, startc}, loc{endl, endc})
}

func (n *Rope) sliceLC(start, end loc) []byte {
	if start.cmp(end) >= 0 {
		return []byte{}
	}

	switch n.kind {
	case tLeaf:
		return sliceloc(n.value, n.opts.LineSep, start, end)
	default: // case tNode
		leftLength := n.left.llength
		leftStart := minloc(start, leftLength)
		leftEnd := minloc(end, leftLength)
		rightLength := n.right.llength
		rightStart := maxloc(lzero, minloc(sublocs(start, leftLength), rightLength))
		rightEnd := maxloc(lzero, minloc(sublocs(end, leftLength), rightLength))

		if leftStart != leftEnd {
			if rightStart != rightEnd {
				return sliceConcat(n.left.sliceLC(leftStart, leftEnd), n.right.sliceLC(rightStart, rightEnd))
			} else {
				return n.left.sliceLC(leftStart, leftEnd)
			}
		} else {
			if rightStart != rightEnd {
				return n.right.sliceLC(rightStart, rightEnd)
			} else {
				return []byte{}
			}
		}
	}
}

// At returns the byte at the given position.
func (n *Rope) At(pos int) byte {
	s := n.Slice(pos, pos+1)
	return s[0]
}

// SplitAt splits the rope at the given index and returns two new ropes
// corresponding to the left and right portions of the split.
func (n *Rope) SplitAt(i int) (*Rope, *Rope) {
	switch n.kind {
	case tLeaf:
		return NewRope(n.value[:i], n.opts), NewRope(n.value[i:], n.opts)
	default: // case tNode
		m := n.left.length
		if i == m {
			return n.left, n.right
		} else if i < m {
			l, r := n.left.SplitAt(i)
			return l, joinRope(r, n.right)
		}
		l, r := n.right.SplitAt(i - m)
		return joinRope(n.left, l), r
	}
}

func joinRope(l, r *Rope) *Rope {
	n := &Rope{
		left:    l,
		right:   r,
		length:  l.length + r.length,
		llength: addlocs(l.llength, r.llength),
		kind:    tNode,
		opts:    l.opts,
	}
	n.adjust()
	return n
}

// Join merges all the given ropes together into one rope.
func Join(a, b *Rope, more ...*Rope) *Rope {
	s := joinRope(a, b)
	for _, n := range more {
		s = joinRope(s, n)
	}
	return s
}

// Rebuild rebuilds the entire rope structure, resulting in a balanced tree.
func (n *Rope) Rebuild() {
	switch n.kind {
	case tNode:
		n.value = sliceConcat(n.left.Value(), n.right.Value())
		n.left = nil
		n.right = nil
		n.kind = tLeaf
		n.length = len(n.value)
		n.llength = llen(n.value, n.opts.LineSep)
		n.adjust()
	}
}

// Rebalance finds unbalanced nodes and rebuilds them.
func (n *Rope) Rebalance() {
	switch n.kind {
	case tNode:
		lratio := float64(n.left.length) / float64(n.right.length)
		rratio := float64(n.right.length) / float64(n.left.length)
		if lratio > n.opts.RebalanceRatio || rratio > n.opts.RebalanceRatio {
			n.Rebuild()
		} else {
			n.left.Rebalance()
			n.right.Rebalance()
		}
	}
}

func (n *Rope) indexAllFunc(off, start, end int, sep []byte, fn func(idx int) bool) {
	if n.kind == tNode && end < off+n.left.length {
		// [start,end) is in left node
		n.left.indexAllFunc(off, start, end, sep, fn)
	} else if n.kind == tNode && start >= off+n.left.length {
		// [start,end) is in right node
		n.right.indexAllFunc(off+n.left.length, start, end, sep, fn)
	} else {
		var total int
		n.EachLeaf(func(it *Rope) bool {
			val := it.Value()
			var acc int
			for {
				idx := bytes.Index(val[acc:], sep)
				if idx == -1 {
					acc += len(val[acc:])
					break
				}

				fullidx := off + total + acc + idx
				if fullidx >= start && fullidx < end {
					if fn(fullidx) {
						return true
					}
				}

				acc += idx + 1
			}
			total += acc
			return false
		})
	}
}

// IndexAllFunc iterates through all occurrences of 'sep' in the range
// [start:end) and calls fn each time with the index of the occurrence. If 'fn'
// returns 'true' iteration is aborted and fn will no longer be called.
func (n *Rope) IndexAllFunc(start, end int, sep []byte, fn func(idx int) bool) {
	n.indexAllFunc(0, start, end, sep, fn)
}

// Each applies the given function to every node in the rope.
func (n *Rope) Each(fn func(n *Rope)) {
	fn(n)
	if n.kind == tNode {
		n.left.Each(fn)
		n.right.Each(fn)
	}
}

// EachLeaf applies the given function to every leaf node in order.
func (n *Rope) EachLeaf(fn func(n *Rope) bool) bool {
	switch n.kind {
	case tLeaf:
		return fn(n)
	default: // case tNode
		if n.left.EachLeaf(fn) {
			return true
		}
		return n.right.EachLeaf(fn)
	}
}

// ReadAt implements the io.ReaderAt interface.
func (n *Rope) ReadAt(p []byte, off int64) (nread int, err error) {
	if off > int64(n.length) {
		return 0, io.EOF
	}

	end := off + int64(len(p))
	if end >= int64(n.length) {
		end = int64(n.length)
		err = io.EOF
	}
	b := n.Slice(int(off), int(end))
	nread = copy(p, b)
	return nread, err
}

// WriteTo implements the io.WriterTo interface.
func (n *Rope) WriteTo(w io.Writer) (int64, error) {
	var err error
	var ntotal int64
	n.EachLeaf(func(it *Rope) bool {
		var nwritten int
		nwritten, err = w.Write(it.Value())
		ntotal += int64(nwritten)
		return err != nil
	})
	return ntotal, err
}

// from slice tricks
func sliceInsert(s []byte, k int, vs []byte) []byte {
	if n := len(s) + len(vs); n <= cap(s) {
		s2 := s[:n]
		copy(s2[k+len(vs):], s[k:])
		copy(s2[k:], vs)
		return s2
	}
	s2 := make([]byte, len(s)+len(vs))
	copy(s2, s[:k])
	copy(s2[k:], vs)
	copy(s2[k+len(vs):], s[k:])
	return s2
}

func sliceConcat(a, b []byte) []byte {
	c := make([]byte, 0, len(a)+len(b))
	c = append(c, a...)
	c = append(c, b...)
	return c
}

func sliceRemove(s []byte, start, end int) []byte {
	if len(s) == cap(s) {
		// "copy-on-write" for slices where len == cap.
		ns := make([]byte, len(s)-(end-start), cap(s))
		copy(ns, s[:start])
		copy(ns[start:], s[end:])
		return ns
	}
	return append(s[:start], s[end:]...)
}
