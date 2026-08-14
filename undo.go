package main

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"io"
	"sort"
	"time"
)

// A Delta can perform and undo some event on the base structure.
type Delta[T, S any] interface {
	Do(base T)
	Undo(base T)
	State() S
}

// EventPtr is a reference to an event. We use an event pool and index into it
// rather than system pointers so that the tree can be serialized.
type EventPtr int

// Event is a single undo event, potentially containing multiple coalesced deltas.
type Event[T, S any] struct {
	Deltas []Delta[T, S]
	Time   time.Time
	Count  int // distance from the root event

	Next []EventPtr
	Prev EventPtr
}

// UndoTree stores undo/redo history as a tree so that no history is ever lost.
// When the tree branches (due to undoing and then making new edits), the user
// can select which branch to redo.
type UndoTree[T, S any] struct {
	Root    EventPtr
	Current EventPtr
	Events  []Event[T, S]

	cutoff      int
	base        T
	barrier     bool
	BarrierOnly bool // when true, only split undo events on explicit barriers (ignore time)
}

const NoCutoff = -1

const coalesceTime = time.Second

// NewUndoTree creates a new undo tree with the given base and cutoff. If
// cutoff is NoCutoff, the history will never be truncated.
func NewUndoTree[T, S any](base T, cutoff int) *UndoTree[T, S] {
	u := &UndoTree[T, S]{
		base:        base,
		cutoff:      cutoff,
		BarrierOnly: true,
		Events:      make([]Event[T, S], 0),
	}
	root := u.newEvent(Event[T, S]{
		Time: time.Now(),
		Prev: -1,
	})
	u.Root = root
	u.Current = root
	return u
}

// Serialize compresses and writes the file mtime and content hash followed
// by the tree.
func (u *UndoTree[T, S]) Serialize(w io.Writer, mtime time.Time, hash []byte) error {
	fz := gzip.NewWriter(w)
	enc := gob.NewEncoder(fz)
	if err := enc.Encode(mtime); err != nil {
		fz.Close()
		return err
	}
	if err := enc.Encode(hash); err != nil {
		fz.Close()
		return err
	}
	err := enc.Encode(u)
	fz.Close()
	return err
}

// FromReader loads an undo tree from a serialized byte stream. If the stored
// mtime or content hash doesn't match the current file, returns nil (stale
// history): applying deltas from a tree whose current state doesn't match the
// buffer contents would corrupt the buffer or panic on out-of-range offsets.
func FromReader[T, S any](r io.Reader, base T, cutoff int, currentMtime time.Time, currentHash []byte) (*UndoTree[T, S], error) {
	fz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer fz.Close()
	dec := gob.NewDecoder(fz)
	var savedMtime time.Time
	if err := dec.Decode(&savedMtime); err != nil {
		return nil, fmt.Errorf("decode mtime: %w", err)
	}
	var savedHash []byte
	if err := dec.Decode(&savedHash); err != nil {
		return nil, fmt.Errorf("decode hash: %w", err)
	}
	if !savedMtime.Equal(currentMtime) || !bytes.Equal(savedHash, currentHash) {
		return nil, nil
	}
	u := UndoTree[T, S]{
		base:   base,
		cutoff: cutoff,
	}
	if err := dec.Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// ToBytes serializes the tree to a byte slice.
func (u *UndoTree[T, S]) ToBytes(mtime time.Time, hash []byte) ([]byte, error) {
	var buf bytes.Buffer
	err := u.Serialize(&buf, mtime, hash)
	return buf.Bytes(), err
}

// FromBytes loads an undo tree from a byte slice.
func FromBytes[T, S any](b []byte, base T, cutoff int, mtime time.Time, hash []byte) (*UndoTree[T, S], error) {
	return FromReader[T, S](bytes.NewReader(b), base, cutoff, mtime, hash)
}

// AdjustSize ensures the cutoff is respected by advancing the root.
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

// Barrier prevents coalescing on the next Apply call.
func (u *UndoTree[T, S]) Barrier() {
	u.barrier = true
}

// Apply a delta to the tree at the current position.
func (u *UndoTree[T, S]) Apply(d Delta[T, S]) {
	d.Do(u.base)

	// Coalesce into the current event if no barrier is set and either
	// BarrierOnly mode is on or we are within the coalesce time window.
	if u.current().Prev != -1 && !u.barrier && (u.BarrierOnly || time.Since(u.current().Time) < coalesceTime) {
		u.current().Deltas = append(u.current().Deltas, d)
		u.barrier = false
		return
	}

	u.barrier = false

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

// Undo reverts the current event.
func (u *UndoTree[T, S]) Undo() {
	if u.current().Prev != -1 {
		deltas := u.current().Deltas
		for i := len(deltas) - 1; i >= 0; i-- {
			deltas[i].Undo(u.base)
		}
		u.Current = u.current().Prev
	}
}

// Redo applies the given event. The pointer must refer to one of the current
// event's children (from RedoChoices).
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

	for _, d := range u.current().Deltas {
		d.Do(u.base)
	}
}

// RedoChoices returns the events that could be redone from here, sorted from
// least recent to most recent.
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

// MostRecent returns the most recently created redo choice, if any.
func (u *UndoTree[T, S]) MostRecent() (EventPtr, bool) {
	if len(u.current().Next) <= 0 {
		return 0, false
	}
	return u.current().Next[len(u.current().Next)-1], true
}

// PrevState returns the cursor state from the current event (before undo).
func (u *UndoTree[T, S]) PrevState() (s S, ok bool) {
	if u.current().Prev == -1 {
		return s, false
	}
	return u.current().Deltas[0].State(), true
}

// NextState returns the cursor state from the given event (for redo).
func (u *UndoTree[T, S]) NextState(ep EventPtr) (s S) {
	return u.event(ep).Deltas[0].State()
}

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
