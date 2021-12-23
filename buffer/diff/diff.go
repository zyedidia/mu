// Package diff provides an function for creating diffs between two string-like
// interface structures. It is adapted from the gonp package
// github.com/cubicdaiya/gonp for a more general interface.
package diff

type OpKind byte

const (
	OpInsert OpKind = iota
	OpDelete
	OpEqual
)

// An Edit made from a diff. If the Kind is OpDelete or OpEqual, the Text will
// be empty and the Length will be the size of the deleted text or equivalent
// text. If the Kind is OpInsert, the Text will contain the text to be
// inserted.
type Edit struct {
	Kind   OpKind
	Text   []byte
	Length int
}

type Indexer interface {
	At(pos int) byte
	Len() int
}

// PointWithRoute is coordinate in edit graph attached route
type PointWithRoute struct {
	x, y, r int
}

// Point is coordinate in edit graph
type Point struct {
	x, y int
}

// Diff returns a list of edits necessary to transform 'from' to 'to'.
func Diff(from, to Indexer) []Edit {
	m, n := from.Len(), to.Len()

	reverse := m >= n
	if m >= n {
		from, to = to, from
		m, n = n, m
	}

	fp := make([]int, m+n+3)
	path := make([]int, m+n+3)
	pointWithRoute := make([]PointWithRoute, 0)

	for i := range fp {
		fp[i] = -1
		path[i] = -1
	}

	offset := m + 1
	delta := n - m

	snake := func(k, p, pp, offset int) int {
		r := 0
		if p > pp {
			r = path[k-1+offset]
		} else {
			r = path[k+1+offset]
		}

		y := max(p, pp)
		x := y - k

		for x < m && y < n && from.At(x) == to.At(y) {
			x++
			y++
		}

		path[k+offset] = len(pointWithRoute)
		pointWithRoute = append(pointWithRoute, PointWithRoute{x: x, y: y, r: r})

		return y
	}

	for p := 0; ; p++ {
		for k := -p; k <= delta-1; k++ {
			fp[k+offset] = snake(k, fp[k-1+offset]+1, fp[k+1+offset], offset)
		}

		for k := delta + p; k >= delta+1; k-- {
			fp[k+offset] = snake(k, fp[k-1+offset]+1, fp[k+1+offset], offset)
		}

		fp[delta+offset] = snake(delta, fp[delta-1+offset]+1, fp[delta+1+offset], offset)

		if fp[delta+offset] >= n {
			break
		}
	}

	r := path[delta+offset]
	epc := make([]Point, 0)
	for r != -1 {
		epc = append(epc, Point{x: pointWithRoute[r].x, y: pointWithRoute[r].y})
		r = pointWithRoute[r].r
	}
	return recordSeq(epc, reverse, from, to)
}

func recordSeq(epc []Point, reverse bool, from, to Indexer) []Edit {
	ses := make([]Edit, 0)

	x, y := 1, 1
	px, py := 0, 0

	var cur Edit
	var init bool
	for i := len(epc) - 1; i >= 0; i-- {
		for (px < epc[i].x) || (py < epc[i].y) {
			var op OpKind
			var char byte
			if (epc[i].y - epc[i].x) > (py - px) {
				if reverse {
					op = OpDelete
				} else {
					op = OpInsert
				}
				char = to.At(py)
				y++
				py++
			} else if epc[i].y-epc[i].x < py-px {
				if reverse {
					op = OpInsert
				} else {
					op = OpDelete
				}
				char = from.At(px)
				x++
				px++
			} else {
				op = OpEqual
				char = from.At(px)
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
				if op == OpInsert {
					cur.Text = append(cur.Text, char)
					cur.Length++
				} else {
					cur.Length++
				}
			} else {
				ses = append(ses, cur)
				var txt []byte = nil
				if op == OpInsert {
					txt = []byte{char}
				}
				cur = Edit{
					Kind:   op,
					Text:   txt,
					Length: 1,
				}
			}
		}
	}
	return append(ses, cur)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
