package text_test

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/zyedidia/mu/text"
)

func check(r *text.Rope, b *basicText, t *testing.T) {
	if !bytes.Equal(r.Value(), b.value()) {
		t.Errorf("incorrect bytes: %s %s", string(r.Value()), string(b.value()))
	}
	if r.Len() != b.length() {
		t.Errorf("incorrect length: %d %d", r.Len(), b.length())
	}
	if r.NumLines() != b.NumLines() {
		t.Errorf("incorrect line count: %d %d", r.NumLines(), b.NumLines())
	}

	const ncheck = 100
	for i := 0; i < ncheck; i++ {
		pos := rand.Intn(r.Len())
		rline, rcol := r.LineColAt(pos)
		bline, bcol := b.lineColAt(pos)
		if rline != bline || rcol != bcol {
			t.Errorf("incorrect offset conversion: %d, want (%d, %d), got (%d, %d)", pos, bline, bcol, rline, rcol)
		}

		off := r.OffsetAt(rline, rcol)
		if off != pos {
			t.Errorf("incorrect line/col conversion: (%d, %d), want %d, got %d", rline, rcol, pos, off)
		}
	}
}

const datasz = 5000

func data() (*text.Rope, *basicText) {
	data := randbytes(datasz)
	r := text.NewRope(data, text.RopeOptions{
		SplitLen:       4,
		JoinLen:        2,
		RebalanceRatio: 1.2,
		LineSep:        []byte{'\n'},
	})
	b := newBasicText(data)
	return r, b
}

func randrange(high int) (int, int) {
	i1 := rand.Intn(high)
	i2 := rand.Intn(high)
	return min(i1, i2), max(i1, i2)
}

var letters = []byte("\nabcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randbytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return b
}

func TestConstruction(t *testing.T) {
	r, b := data()
	check(r, b, t)
}

func TestInsertRemove(t *testing.T) {
	r, b := data()

	const nedit = 100
	const strlen = 20
	for i := 0; i < nedit; i++ {
		low, high := randrange(r.Len())
		r.Remove(low, high)
		b.remove(low, high)
		check(r, b, t)
		bstr := randbytes(strlen)
		r.Insert(low, bstr)
		b.insert(low, bstr)
		check(r, b, t)
	}
	check(r, b, t)
}

func TestReadAt(t *testing.T) {
	r, b := data()

	const nslice = 100
	length := r.Len()
	for i := 0; i < nslice; i++ {
		low, high := randrange(length)

		rb := make([]byte, high-low)
		r.ReadAt(rb, int64(low))
		bb := b.slice(low, high)
		if !bytes.Equal(rb, bb) {
			t.Errorf("slice not equal: %s %s", string(rb), string(bb))
		}
	}
}

func TestSplit(t *testing.T) {
	r, b := data()

	const nsplit = 10
	for i := 0; i < nsplit; i++ {
		splitidx := rand.Intn(r.Len())
		left, right := r.SplitAt(splitidx)

		lb := b.slice(0, splitidx)
		rb := b.slice(splitidx, b.length())
		if !bytes.Equal(left.Value(), lb) {
			t.Errorf("%d: left slice not equal: %s %s", splitidx, string(left.Value()), string(lb))
		}
		if !bytes.Equal(right.Value(), rb) {
			t.Errorf("%d: right slice not equal: %s %s", splitidx, string(right.Value()), string(rb))
		}
		r = text.Join(left, right)
		check(r, b, t)
	}
}

type basicText struct {
	data []byte
}

func newBasicText(b []byte) *basicText {
	data := make([]byte, len(b))
	copy(data, b)
	return &basicText{
		data: data,
	}
}

func (b *basicText) length() int {
	return len(b.data)
}

func (b *basicText) value() []byte {
	return b.data
}

func (b *basicText) remove(start, end int) {
	b.data = append(b.data[:start], b.data[end:]...)
}

func (b *basicText) insert(pos int, val []byte) {
	b.data = testInsert(b.data, pos, val)
}

func (b *basicText) slice(start, end int) []byte {
	return b.data[start:end]
}

func (b *basicText) lineColAt(pos int) (line, col int) {
	var last int
	for i, c := range b.data {
		if c == '\n' {
			if i >= pos {
				return line, pos - last
			}
			last = i + 1
			line++
		}
	}
	return line, pos - last
}

func (b *basicText) NumLines() int {
	return bytes.Count(b.data, []byte{'\n'})
}

func TestSlice(t *testing.T) {
	r, b := data()

	const nslice = 100
	for i := 0; i < nslice; i++ {
		low, high := randrange(r.Len())
		rs := r.Slice(low, high)
		bs := b.slice(low, high)
		if !bytes.Equal(rs, bs) {
			t.Errorf("Slice(%d,%d): mismatch", low, high)
		}
	}

	// empty slice
	if got := r.Slice(5, 5); len(got) != 0 {
		t.Errorf("Slice(5,5): got %q, want empty", got)
	}
}

func TestSliceLC(t *testing.T) {
	r, b := data()

	const ncheck = 100
	for i := 0; i < ncheck; i++ {
		low, high := randrange(r.Len())
		sl, sc := r.LineColAt(low)
		el, ec := r.LineColAt(high)
		got := r.SliceLC(sl, sc, el, ec)
		want := b.slice(low, high)
		if !bytes.Equal(got, want) {
			t.Errorf("SliceLC(%d,%d,%d,%d): got %q, want %q", sl, sc, el, ec, got, want)
		}
	}

	// empty range
	l, c := r.LineColAt(0)
	if got := r.SliceLC(l, c, l, c); len(got) != 0 {
		t.Errorf("SliceLC empty: got %q", got)
	}
}

func TestJoin(t *testing.T) {
	d1 := randbytes(100)
	d2 := randbytes(100)
	d3 := randbytes(100)
	opts := text.RopeOptions{
		SplitLen:       4,
		JoinLen:        2,
		RebalanceRatio: 1.2,
		LineSep:        []byte{'\n'},
	}
	r1 := text.NewRope(d1, opts)
	r2 := text.NewRope(d2, opts)
	r3 := text.NewRope(d3, opts)

	joined := text.Join(r1, r2, r3)
	want := make([]byte, 0, 300)
	want = append(want, d1...)
	want = append(want, d2...)
	want = append(want, d3...)

	if !bytes.Equal(joined.Value(), want) {
		t.Fatal("Join: value mismatch")
	}
	if joined.Len() != len(want) {
		t.Fatalf("Join: Len got %d, want %d", joined.Len(), len(want))
	}
	wantLines := bytes.Count(want, []byte{'\n'})
	if joined.NumLines() != wantLines {
		t.Fatalf("Join: NumLines got %d, want %d", joined.NumLines(), wantLines)
	}
}

func TestWriteTo(t *testing.T) {
	r, _ := data()
	var buf bytes.Buffer
	n, err := r.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo error: %v", err)
	}
	if int(n) != r.Len() {
		t.Fatalf("WriteTo: wrote %d bytes, want %d", n, r.Len())
	}
	if !bytes.Equal(buf.Bytes(), r.Value()) {
		t.Fatal("WriteTo: data mismatch")
	}
}

func TestIndexAllFunc(t *testing.T) {
	content := []byte("a\nb\nc\nd\n")
	opts := text.RopeOptions{
		SplitLen:       4,
		JoinLen:        2,
		RebalanceRatio: 1.2,
		LineSep:        []byte{'\n'},
	}
	r := text.NewRope(content, opts)

	var indices []int
	r.IndexAllFunc(0, r.Len(), []byte{'\n'}, func(idx int) bool {
		indices = append(indices, idx)
		return false
	})

	want := []int{1, 3, 5, 7}
	if len(indices) != len(want) {
		t.Fatalf("IndexAllFunc: got %v, want %v", indices, want)
	}
	for i, idx := range indices {
		if idx != want[i] {
			t.Errorf("IndexAllFunc[%d]: got %d, want %d", i, idx, want[i])
		}
	}
}

func TestIndexAllFuncSubrange(t *testing.T) {
	content := []byte("a\nb\nc\nd\n")
	opts := text.RopeOptions{
		SplitLen:       4,
		JoinLen:        2,
		RebalanceRatio: 1.2,
		LineSep:        []byte{'\n'},
	}
	r := text.NewRope(content, opts)

	var indices []int
	r.IndexAllFunc(2, 6, []byte{'\n'}, func(idx int) bool {
		indices = append(indices, idx)
		return false
	})

	want := []int{3, 5}
	if len(indices) != len(want) {
		t.Fatalf("IndexAllFunc subrange: got %v, want %v", indices, want)
	}
	for i, idx := range indices {
		if idx != want[i] {
			t.Errorf("IndexAllFunc subrange[%d]: got %d, want %d", i, idx, want[i])
		}
	}
}

func TestIndexAllFuncAbort(t *testing.T) {
	content := []byte("a\nb\nc\n")
	opts := text.RopeOptions{
		SplitLen:       4,
		JoinLen:        2,
		RebalanceRatio: 1.2,
		LineSep:        []byte{'\n'},
	}
	r := text.NewRope(content, opts)

	var indices []int
	r.IndexAllFunc(0, r.Len(), []byte{'\n'}, func(idx int) bool {
		indices = append(indices, idx)
		return true // abort after first
	})

	if len(indices) != 1 {
		t.Fatalf("IndexAllFunc abort: got %d results, want 1", len(indices))
	}
}

func TestEachLeaf(t *testing.T) {
	r, _ := data()

	// Concatenating all leaves should equal Value()
	var result []byte
	r.EachLeaf(func(n *text.Rope) bool {
		result = append(result, n.Value()...)
		return false
	})
	if !bytes.Equal(result, r.Value()) {
		t.Fatal("EachLeaf: concatenated leaves != Value()")
	}
}

func TestEachLeafAbort(t *testing.T) {
	r, _ := data()

	count := 0
	r.EachLeaf(func(n *text.Rope) bool {
		count++
		return true // abort after first
	})
	if count != 1 {
		t.Fatalf("EachLeaf abort: visited %d nodes, want 1", count)
	}
}

func TestEach(t *testing.T) {
	r, _ := data()

	count := 0
	r.Each(func(n *text.Rope) {
		count++
	})
	if count == 0 {
		t.Fatal("Each: visited 0 nodes")
	}
}

func TestRebalance(t *testing.T) {
	r, b := data()

	// Do many insertions to potentially unbalance the tree
	for i := 0; i < 50; i++ {
		pos := rand.Intn(r.Len())
		bs := randbytes(10)
		r.Insert(pos, bs)
		b.insert(pos, bs)
	}

	r.Rebalance()
	check(r, b, t)
}

func TestRebuild(t *testing.T) {
	r, b := data()

	// Do some edits then rebuild
	for i := 0; i < 20; i++ {
		low, high := randrange(r.Len())
		r.Remove(low, high)
		b.remove(low, high)
		bs := randbytes(15)
		r.Insert(low, bs)
		b.insert(low, bs)
	}

	r.Rebuild()
	check(r, b, t)
}

func TestEmptyRope(t *testing.T) {
	opts := text.RopeOptions{
		SplitLen:       4,
		JoinLen:        2,
		RebalanceRatio: 1.2,
		LineSep:        []byte{'\n'},
	}
	r := text.NewRope([]byte{}, opts)

	if r.Len() != 0 {
		t.Fatalf("empty rope Len: got %d", r.Len())
	}
	if r.NumLines() != 0 {
		t.Fatalf("empty rope NumLines: got %d", r.NumLines())
	}
	if len(r.Value()) != 0 {
		t.Fatalf("empty rope Value: got %q", r.Value())
	}
}

func TestSingleByte(t *testing.T) {
	opts := text.RopeOptions{
		SplitLen:       4,
		JoinLen:        2,
		RebalanceRatio: 1.2,
		LineSep:        []byte{'\n'},
	}
	r := text.NewRope([]byte("x"), opts)

	if r.Len() != 1 {
		t.Fatalf("single byte Len: got %d", r.Len())
	}
	if r.At(0) != 'x' {
		t.Fatalf("single byte At(0): got %c", r.At(0))
	}
	line, col := r.LineColAt(0)
	if line != 0 || col != 0 {
		t.Fatalf("single byte LineColAt(0): got (%d,%d)", line, col)
	}
}

func TestNewlineOnlyRope(t *testing.T) {
	opts := text.RopeOptions{
		SplitLen:       4,
		JoinLen:        2,
		RebalanceRatio: 1.2,
		LineSep:        []byte{'\n'},
	}
	r := text.NewRope([]byte("\n\n\n"), opts)

	if r.NumLines() != 3 {
		t.Fatalf("newline-only NumLines: got %d, want 3", r.NumLines())
	}
	if r.Len() != 3 {
		t.Fatalf("newline-only Len: got %d, want 3", r.Len())
	}
}

func TestLLen(t *testing.T) {
	opts := text.RopeOptions{
		SplitLen:       4,
		JoinLen:        2,
		RebalanceRatio: 1.2,
		LineSep:        []byte{'\n'},
	}

	tests := []struct {
		input     string
		wantLines int
		wantCols  int
	}{
		{"", 0, 0},
		{"abc", 0, 3},
		{"a\nb", 1, 1},
		{"a\nb\n", 2, 0},
		{"\n\n", 2, 0},
	}

	for _, tt := range tests {
		r := text.NewRope([]byte(tt.input), opts)
		lines, cols := r.LLen()
		if lines != tt.wantLines || cols != tt.wantCols {
			t.Errorf("LLen(%q): got (%d,%d), want (%d,%d)", tt.input, lines, cols, tt.wantLines, tt.wantCols)
		}
	}
}

// from slice tricks
func testInsert(s []byte, k int, vs []byte) []byte {
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
