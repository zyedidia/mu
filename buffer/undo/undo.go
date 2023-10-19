// Package undo provides a data structure for undoing and redoing events on
// some base data. It stores the events in a tree so that no undo/redo history
// is every lost. When the tree starts branch out (due to undoing and then
// applying new events), the user can select which events they want for
// performing redo. The undo data structure also supports serialization to a
// byte slice and a cutoff to prevent it from getting too large.
package undo

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"sort"
	"time"
)

// A Delta can perform and undo some event on the base structure provided to
// the tree.
type Delta[T, S any] interface {
	Do(base T)
	Undo(base T)
	State() S
}

// EventPtr is a reference to an event. Since we want to support serializing
// the undo tree we don't use system pointers, but instead use an event pool
// and index into the pool (essentially pointers).
type EventPtr int

type Event[T, S any] struct {
	Deltas []Delta[T, S]
	Time   time.Time // time this edit was made
	Count  int       // distance from the first event

	Next []EventPtr
	Prev EventPtr
}

type UndoTree[T, S any] struct {
	Root    EventPtr      // root of tree
	Current EventPtr      // current state in tree
	Events  []Event[T, S] // pool of events that have been applied

	// If the count distance between the root and current state exceeds the
	// cutoff, the root will be advanced until this is no longer the case
	// (meaning undo history will be deleted. If the cutoff is NoCutoff, the
	// undo history will never be deleted.
	cutoff  int
	base    T
	barrier bool
}

const NoCutoff = -1

func NewTree[T, S any](base T, cutoff int) *UndoTree[T, S] {
	u := &UndoTree[T, S]{
		base:   base,
		cutoff: cutoff,
		Events: make([]Event[T, S], 0),
	}
	root := u.newEvent(Event[T, S]{
		Time: time.Now(),
		Prev: -1,
	})
	u.Root = root
	u.Current = root
	return u
}

// ToBytes serializes and compresses the tree into a byte stream that can be
// saved to disk.
func (u *UndoTree[T, S]) ToBytes() ([]byte, error) {
	var buf bytes.Buffer
	fz := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(fz)
	err := enc.Encode(u)
	fz.Close()
	return buf.Bytes(), err
}

// FromBytes loads the undo tree from a serialized version.
func FromBytes[T, S any](b []byte, base T, cutoff int) (*UndoTree[T, S], error) {
	u := UndoTree[T, S]{
		base:   base,
		cutoff: cutoff,
	}
	fz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	dec := gob.NewDecoder(fz)
	err = dec.Decode(&u)
	fz.Close()
	return &u, err
}

// AdjustSize ensures that the cutoff is respected.
func (u *UndoTree[T, S]) AdjustSize() {
	if u.cutoff != NoCutoff {
		for u.current().Count-u.root().Count > u.cutoff && len(u.root().Next) > 0 {
			next := u.root().Next[0]
			if next < u.Current {
				u.Events = u.Events[next-u.Root:]
				u.Root = next
			}
		}
	}
}

const coalesceTime = time.Second

// Barrier prevents coalescing on the next event application.
func (u *UndoTree[T, S]) Barrier() {
	u.barrier = true
}

// Apply a delta to the tree at the current position.
func (u *UndoTree[T, S]) Apply(d Delta[T, S]) {
	d.Do(u.base)

	// If it hasn't been enough time since the last event, coalesce this event
	// into that one. Never coalesce into the root node.
	if u.current().Prev != -1 && !u.barrier && time.Since(u.current().Time) < coalesceTime {
		u.current().Deltas = append(u.current().Deltas, d)
		u.barrier = false
		return
	}

	// otherwise create a new event
	e := u.newEvent(Event[T, S]{
		Deltas: []Delta[T, S]{d},
		Time:   time.Now(),
		Count:  u.current().Count + 1,
		Next:   nil,
		Prev:   -1,
	})

	u.current().Next = append(u.current().Next, e)
	sort.Slice(u.current().Next, func(i, j int) bool {
		nexti := u.event(u.current().Next[i])
		nextj := u.event(u.current().Next[j])
		return nexti.Time.Before(nextj.Time)
	})

	u.event(e).Prev = u.Current
	u.Current = e

	u.AdjustSize()
}

// Undo from the current position.
func (u *UndoTree[T, S]) Undo() {
	if u.current().Prev != -1 {
		// undo all events in reverse order
		deltas := u.current().Deltas
		for i := len(deltas) - 1; i >= 0; i-- {
			deltas[i].Undo(u.base)
		}
		u.Current = u.current().Prev
	}
}

// Redo the given event from the current position. The pointer must refer to an
// event in the current event's set of possible next steps (returned by
// RedoChoices). If that is not the case nothing will happen.
func (u *UndoTree[T, S]) Redo(e EventPtr) {
	var found bool
	for _, ne := range u.current().Next {
		if ne == e {
			found = true
			break
		}
	}
	if !found {
		return
	}

	u.Current = e

	// do all events in order
	deltas := u.current().Deltas
	for _, d := range deltas {
		d.Do(u.base)
	}
}

// RedoChoices returns the events that could be redone from here (each
// corresponding to a separate undo branch in the tree. The list of events
// returned is sorted from least recent to most recent.
func (u *UndoTree[T, S]) RedoChoices() []EventPtr {
	return u.current().Next
}

// RedoMostRecent applies redo to the most recent available event.
func (u *UndoTree[T, S]) RedoMostRecent() {
	e, ok := u.MostRecent()
	if !ok {
		return
	}
	u.Redo(e)
}

func (u *UndoTree[T, S]) MostRecent() (EventPtr, bool) {
	if len(u.current().Next) <= 0 {
		return 0, false
	}

	e := u.current().Next[len(u.current().Next)-1]
	return e, true
}

func (u *UndoTree[T, S]) PrevState() (s S, ok bool) {
	if u.current().Prev == -1 {
		return s, false
	}
	return u.current().Deltas[0].State(), true
}

// use with caution, does not check that ep is valid
func (u *UndoTree[T, S]) NextState(ep EventPtr) (s S) {
	e := u.event(ep)
	return e.Deltas[0].State()
}

// Below are some utility functions for using the custom pointer system.
func (u *UndoTree[T, S]) newEvent(ev Event[T, S]) EventPtr {
	u.Events = append(u.Events, ev)
	return u.Root + EventPtr(len(u.Events)-1)
}

func (u *UndoTree[T, S]) event(p EventPtr) *Event[T, S] {
	return &u.Events[p-u.Root]
}

func (u *UndoTree[T, S]) current() *Event[T, S] {
	return u.event(u.Current)
}

func (u *UndoTree[T, S]) root() *Event[T, S] {
	return u.event(u.Root)
}
