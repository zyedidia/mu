package main

// Diff computes the edits needed to transform 'from' into 'to'. It operates
// on the byte level using the Indexer interface, so it works directly on
// buffer data structures without copying.

// DiffOpKind identifies the type of diff edit.
type DiffOpKind byte

const (
	DiffInsert DiffOpKind = iota
	DiffDelete
	DiffEqual
)

// DiffEdit is one step in a diff result.
type DiffEdit struct {
	Kind   DiffOpKind
	Text   []byte // non-nil only for Insert
	Length int
}

// Indexer is the interface for byte-level access needed by the diff.
type Indexer interface {
	ByteAt(pos int) byte
	Len() int
}

type diffPoint struct{ x, y int }
type diffPWR struct{ x, y, r int }

// Diff returns the edits to transform 'from' into 'to'. Uses an O(ND)
// algorithm adapted from the gonp package.
func Diff(from, to Indexer) []DiffEdit {
	edits, _ := DiffBounded(from, to, -1)
	return edits
}

// DiffBounded is Diff with a budget on the search nodes explored. The
// O(ND) algorithm allocates one node per diagonal snake, so the node count
// grows with (delta + 2p)·p; a budget bounds both its time and memory.
// When the budget is exhausted it returns ok=false and callers should fall
// back to wholesale replacement. Cheap cases like large pure appends
// (delta large, p = 0) stay within small budgets. budget < 0 means
// unlimited.
func DiffBounded(from, to Indexer, budget int) (edits []DiffEdit, ok bool) {
	m, n := from.Len(), to.Len()

	reverse := m >= n
	if reverse {
		from, to = to, from
		m, n = n, m
	}

	fp := make([]int, m+n+3)
	path := make([]int, m+n+3)
	pwr := make([]diffPWR, 0)

	for i := range fp {
		fp[i] = -1
		path[i] = -1
	}

	offset := m + 1
	delta := n - m

	snake := func(k, p, pp int) int {
		r := 0
		if p > pp {
			r = path[k-1+offset]
		} else {
			r = path[k+1+offset]
		}
		y := max(p, pp)
		x := y - k
		for x < m && y < n && from.ByteAt(x) == to.ByteAt(y) {
			x++
			y++
		}
		path[k+offset] = len(pwr)
		pwr = append(pwr, diffPWR{x, y, r})
		return y
	}

	for p := 0; ; p++ {
		// Each round adds delta+2p+1 nodes; give up when the budget is
		// exhausted.
		if budget >= 0 && len(pwr)+delta+2*p+1 > budget {
			return nil, false
		}
		for k := -p; k <= delta-1; k++ {
			fp[k+offset] = snake(k, fp[k-1+offset]+1, fp[k+1+offset])
		}
		for k := delta + p; k >= delta+1; k-- {
			fp[k+offset] = snake(k, fp[k-1+offset]+1, fp[k+1+offset])
		}
		fp[delta+offset] = snake(delta, fp[delta-1+offset]+1, fp[delta+1+offset])
		if fp[delta+offset] >= n {
			break
		}
	}

	r := path[delta+offset]
	epc := make([]diffPoint, 0)
	for r != -1 {
		epc = append(epc, diffPoint{pwr[r].x, pwr[r].y})
		r = pwr[r].r
	}
	return recordSeq(epc, reverse, from, to), true
}

func recordSeq(epc []diffPoint, reverse bool, from, to Indexer) []DiffEdit {
	ses := make([]DiffEdit, 0)
	x, y := 1, 1
	px, py := 0, 0

	var cur DiffEdit
	var init bool
	for i := len(epc) - 1; i >= 0; i-- {
		for px < epc[i].x || py < epc[i].y {
			var op DiffOpKind
			var ch byte
			if (epc[i].y - epc[i].x) > (py - px) {
				if reverse {
					op = DiffDelete
				} else {
					op = DiffInsert
				}
				ch = to.ByteAt(py)
				y++
				py++
			} else if (epc[i].y - epc[i].x) < (py - px) {
				if reverse {
					op = DiffInsert
				} else {
					op = DiffDelete
				}
				ch = from.ByteAt(px)
				x++
				px++
			} else {
				op = DiffEqual
				ch = from.ByteAt(px)
				x++
				y++
				px++
				py++
			}
			if !init {
				cur.Kind = op
				init = true
			}
			if cur.Kind == op {
				if op == DiffInsert {
					cur.Text = append(cur.Text, ch)
				}
				cur.Length++
			} else {
				ses = append(ses, cur)
				var txt []byte
				if op == DiffInsert {
					txt = []byte{ch}
				}
				cur = DiffEdit{Kind: op, Text: txt, Length: 1}
			}
		}
	}
	if init {
		ses = append(ses, cur)
	}
	return ses
}
