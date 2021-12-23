package undo_test

import (
	"encoding/gob"
	"fmt"
	"testing"

	"github.com/zyedidia/ned/buffer/undo"
)

func check(got, want int, t *testing.T) {
	if got != want {
		t.Errorf("want %d, got %d", want, got)
	}
}

type Add struct {
	Amt int
}

func (a Add) Undo(base interface{}) {
	*(base.(*int)) -= a.Amt
}

func (a Add) Do(base interface{}) {
	*(base.(*int)) += a.Amt
}

func TestUndo(t *testing.T) {
	base := 42
	u := undo.NewTree(&base, undo.NoCutoff)
	u.Barrier()
	u.Apply(Add{Amt: 5})
	check(base, 47, t)
	u.Barrier()
	u.Apply(Add{Amt: -2})
	check(base, 45, t)
	u.Undo()
	check(base, 47, t)
	u.Barrier()
	u.Apply(Add{Amt: 5})
	check(base, 52, t)
	u.Undo()
	check(base, 47, t)
	u.RedoMostRecent()
	check(base, 52, t)
	u.Undo()
	u.Redo(u.RedoChoices()[0])
	check(base, 45, t)
}

func TestCoalesce(t *testing.T) {
	base := 42
	u := undo.NewTree(&base, undo.NoCutoff)
	u.Apply(Add{Amt: 5})
	u.Apply(Add{Amt: 5})
	u.Apply(Add{Amt: 5})
	u.Apply(Add{Amt: 5})
	u.Undo()
	check(base, 42, t)
}

func TestSerialize(t *testing.T) {
	gob.Register(Add{})

	base := 42
	u := undo.NewTree(&base, undo.NoCutoff)
	u.Barrier()
	u.Apply(Add{Amt: 5})
	u.Barrier()
	u.Apply(Add{Amt: -2})
	u.Undo()
	check(base, 47, t)

	b, err := u.ToBytes()
	if err != nil {
		t.Fatal(err)
	}
	u, err = undo.FromBytes(b, &base, undo.NoCutoff)
	if err != nil {
		t.Fatal(err)
	}

	u.RedoMostRecent()
	check(base, 45, t)
	u.Undo()
	check(base, 47, t)
	u.Undo()
	check(base, 42, t)
}

func TestCutoff(t *testing.T) {
	base := 42
	u := undo.NewTree(&base, 1000)
	for i := 0; i < 10000; i++ {
		u.Barrier()
		u.Apply(Add{Amt: 5})
	}
	b, err := u.ToBytes()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("size after 10,000 edits with cutoff of 1,000: %d\n", len(b))
}
