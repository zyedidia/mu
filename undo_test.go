package main

import (
	"encoding/gob"
	"testing"
	"time"
)

type testAdd struct {
	Amt int
}

func (a testAdd) Do(base *int)   { *base += a.Amt }
func (a testAdd) Undo(base *int) { *base -= a.Amt }
func (a testAdd) State() bool    { return false }

func checkVal(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("want %d, got %d", want, got)
	}
}

func TestUndoBasic(t *testing.T) {
	base := 42
	u := NewUndoTree[*int, bool](&base, NoCutoff)
	u.Barrier()
	u.Apply(testAdd{Amt: 5})
	checkVal(t, base, 47)
	u.Barrier()
	u.Apply(testAdd{Amt: -2})
	checkVal(t, base, 45)
	u.Undo()
	checkVal(t, base, 47)
	u.Barrier()
	u.Apply(testAdd{Amt: 5})
	checkVal(t, base, 52)
	u.Undo()
	checkVal(t, base, 47)
	u.RedoMostRecent()
	checkVal(t, base, 52)
	// Undo back to the branch point and take the other branch.
	u.Undo()
	u.Redo(u.RedoChoices()[0])
	checkVal(t, base, 45)
}

func TestUndoCoalesce(t *testing.T) {
	base := 42
	u := NewUndoTree[*int, bool](&base, NoCutoff)
	u.Apply(testAdd{Amt: 5})
	u.Apply(testAdd{Amt: 5})
	u.Apply(testAdd{Amt: 5})
	u.Apply(testAdd{Amt: 5})
	// All four should coalesce into one event.
	u.Undo()
	checkVal(t, base, 42)
}

func TestUndoSerialize(t *testing.T) {
	gob.Register(testAdd{})

	base := 42
	u := NewUndoTree[*int, bool](&base, NoCutoff)
	u.Barrier()
	u.Apply(testAdd{Amt: 5})
	u.Barrier()
	u.Apply(testAdd{Amt: -2})
	u.Undo()
	checkVal(t, base, 47)

	mtime := time.Now()
	hash := []byte{1, 2, 3}
	b, err := u.ToBytes(mtime, hash)
	if err != nil {
		t.Fatal(err)
	}
	u, err = FromBytes[*int, bool](b, &base, NoCutoff, mtime, hash)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		t.Fatal("matching mtime and hash should load")
	}

	// A mismatched content hash must discard the history.
	stale, err := FromBytes[*int, bool](b, &base, NoCutoff, mtime, []byte{9, 9, 9})
	if err != nil {
		t.Fatal(err)
	}
	if stale != nil {
		t.Fatal("mismatched hash should discard history")
	}

	u.RedoMostRecent()
	checkVal(t, base, 45)
	u.Undo()
	checkVal(t, base, 47)
	u.Undo()
	checkVal(t, base, 42)
}
