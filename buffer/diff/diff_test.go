package diff

import (
	"testing"
)

func bequal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalsSesElemArray(ses1, ses2 []Edit) bool {
	m, n := len(ses1), len(ses2)
	if m != n {
		return true
	}
	for i := 0; i < m; i++ {
		if !bequal(ses1[i].Text, ses2[i].Text) || ses1[i].Kind != ses2[i].Kind || ses1[i].Length != ses2[i].Length {
			return false
		}
	}
	return true
}

func assert(t *testing.T, b bool) {
	if !b {
		t.Fail()
	}
}

type indexer struct {
	slc []byte
}

func (i *indexer) At(pos int) byte {
	return i.slc[pos]
}

func (i *indexer) Len() int {
	return len(i.slc)
}

func newbytes(s string) *indexer {
	return &indexer{
		slc: []byte(s),
	}
}

func TestDiff1(t *testing.T) {
	sesActual := Diff(newbytes("abc"), newbytes("abd"))
	sesExpected := []Edit{
		{Text: nil, Kind: OpEqual, Length: 2},
		{Text: nil, Kind: OpDelete, Length: 1},
		{Text: []byte("d"), Kind: OpInsert, Length: 1},
	}
	assert(t, equalsSesElemArray(sesActual, sesExpected))
}

func TestDiff2(t *testing.T) {
	sesActual := Diff(newbytes("abcdef"), newbytes("dacfea"))
	sesExpected := []Edit{
		{Text: []byte("d"), Kind: OpInsert, Length: 1},
		{Text: nil, Kind: OpEqual, Length: 1},
		{Text: nil, Kind: OpDelete, Length: 1},
		{Text: nil, Kind: OpEqual, Length: 1},
		{Text: nil, Kind: OpDelete, Length: 2},
		{Text: nil, Kind: OpEqual, Length: 1},
		{Text: []byte("ea"), Kind: OpInsert, Length: 2},
	}
	assert(t, equalsSesElemArray(sesActual, sesExpected))
}

func TestDiff3(t *testing.T) {
	sesActual := Diff(newbytes("acbdeacbed"), newbytes("acebdabbabed"))
	sesExpected := []Edit{
		{Text: nil, Kind: OpEqual, Length: 2},
		{Text: []byte("e"), Kind: OpInsert, Length: 1},
		{Text: nil, Kind: OpEqual, Length: 2},
		{Text: nil, Kind: OpDelete, Length: 1},
		{Text: nil, Kind: OpEqual, Length: 1},
		{Text: nil, Kind: OpDelete, Length: 1},
		{Text: nil, Kind: OpEqual, Length: 1},
		{Text: []byte("bab"), Kind: OpInsert, Length: 3},
		{Text: nil, Kind: OpEqual, Length: 2},
	}
	assert(t, equalsSesElemArray(sesActual, sesExpected))
}

func TestDiff4(t *testing.T) {
	sesActual := Diff(newbytes("abcbda"), newbytes("bdcaba"))
	sesExpected := []Edit{
		{Text: nil, Kind: OpDelete, Length: 1},
		{Text: nil, Kind: OpEqual, Length: 1},
		{Text: []byte("d"), Kind: OpInsert, Length: 1},
		{Text: nil, Kind: OpEqual, Length: 1},
		{Text: []byte("a"), Kind: OpInsert, Length: 1},
		{Text: nil, Kind: OpEqual, Length: 1},
		{Text: nil, Kind: OpDelete, Length: 1},
		{Text: nil, Kind: OpEqual, Length: 1},
	}
	assert(t, equalsSesElemArray(sesActual, sesExpected))
}

func TestDiff5(t *testing.T) {
	sesActual := Diff(newbytes("bokko"), newbytes("bokkko"))
	sesExpected := []Edit{
		{Text: nil, Kind: OpEqual, Length: 4},
		{Text: []byte("k"), Kind: OpInsert, Length: 1},
		{Text: nil, Kind: OpEqual, Length: 1},
	}
	assert(t, equalsSesElemArray(sesActual, sesExpected))
}

func TestDiffEmptyString1(t *testing.T) {
	sesActual := Diff(newbytes(""), newbytes(""))
	sesExpected := []Edit{}
	assert(t, equalsSesElemArray(sesActual, sesExpected))
}

func TestDiffEmptyString2(t *testing.T) {
	sesActual := Diff(newbytes("a"), newbytes(""))
	sesExpected := []Edit{
		{Text: nil, Kind: OpDelete, Length: 1},
	}
	assert(t, equalsSesElemArray(sesActual, sesExpected))
}

func TestDiffEmptyString3(t *testing.T) {
	sesActual := Diff(newbytes(""), newbytes("b"))
	sesExpected := []Edit{
		{Text: []byte("b"), Kind: OpInsert, Length: 1},
	}
	assert(t, equalsSesElemArray(sesActual, sesExpected))
}
