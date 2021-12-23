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
// TODO: when Go gets generics, make the base a generic type instead of an
// interface.
type Delta interface {
	Do(base interface{})
	Undo(base interface{})
}

// EventPtr is a reference to an event. Since we want to support serializing
// the undo tree we don't use system pointers, but instead use an event pool
// and index into the pool (essentially pointers).
type EventPtr int

type Event struct {
	Deltas []Delta
	Time   time.Time // time this edit was made
	Count  int       // distance from the first event

	Next []EventPtr
	Prev EventPtr
}

type UndoTree struct {
	Root    EventPtr // root of tree
	Current EventPtr // current state in tree
	Events  []Event  // pool of events that have been applied

	// If the count distance between the root and current state exceeds the
	// cutoff, the root will be advanced until this is no longer the case
	// (meaning undo history will be deleted. If the cutoff is NoCutoff, the
	// undo history will never be deleted.
	cutoff  int
	base    interface{}
	barrier bool
}

const NoCutoff = -1

func NewTree(base interface{}, cutoff int) *UndoTree {
	u := &UndoTree{
		base:   base,
		cutoff: cutoff,
		Events: make([]Event, 0),
	}
	root := u.newEvent(Event{
		Time: time.Now(),
		Prev: -1,
	})
	u.Root = root
	u.Current = root
	return u
}

// ToBytes serializes and compresses the tree into a byte stream that can be
// saved to disk.
func (u *UndoTree) ToBytes() ([]byte, error) {
	var buf bytes.Buffer
	fz := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(fz)
	err := enc.Encode(u)
	fz.Close()
	return buf.Bytes(), err
}

// FromBytes loads the undo tree from a serialized version.
func FromBytes(b []byte, base interface{}, cutoff int) (*UndoTree, error) {
	u := UndoTree{
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
func (u *UndoTree) AdjustSize() {
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
func (u *UndoTree) Barrier() {
	u.barrier = true
}

// Apply a delta to the tree at the current position.
func (u *UndoTree) Apply(d Delta) {
	d.Do(u.base)

	// If it hasn't been enough time since the last event, coalesce this event
	// into that one. Never coalesce into the root node.
	if u.current().Prev != -1 && !u.barrier && time.Since(u.current().Time) < coalesceTime {
		u.current().Deltas = append(u.current().Deltas, d)
		u.barrier = false
		return
	}

	// otherwise create a new event
	e := u.newEvent(Event{
		Deltas: []Delta{d},
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
func (u *UndoTree) Undo() {
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
func (u *UndoTree) Redo(e EventPtr) {
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
func (u *UndoTree) RedoChoices() []EventPtr {
	return u.current().Next
}

// RedoMostRecent applies redo to the most recent available event.
func (u *UndoTree) RedoMostRecent() {
	if len(u.current().Next) <= 0 {
		return
	}

	e := u.current().Next[len(u.current().Next)-1]
	u.Redo(e)
}

// Below are some utility functions for using the custom pointer system.
func (u *UndoTree) newEvent(ev Event) EventPtr {
	u.Events = append(u.Events, ev)
	return u.Root + EventPtr(len(u.Events)-1)
}

func (u *UndoTree) event(p EventPtr) *Event {
	return &u.Events[p-u.Root]
}

func (u *UndoTree) current() *Event {
	return u.event(u.Current)
}

func (u *UndoTree) root() *Event {
	return u.event(u.Root)
}
