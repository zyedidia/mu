package main

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
)

type Stats struct {
	Redraw []time.Duration
	Event  []time.Duration
	Alloc  []uint64
	lock   sync.Mutex
}

func (s *Stats) AddEventTime(t time.Duration) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Event = append(s.Event, t)
}

func (s *Stats) AddRedrawTime(t time.Duration) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Redraw = append(s.Redraw, t)
}

func (s *Stats) SampleAlloc() {
	s.lock.Lock()
	defer s.lock.Unlock()
	var mstats runtime.MemStats
	runtime.ReadMemStats(&mstats)
	s.Alloc = append(s.Alloc, mstats.Alloc)
}

func (s *Stats) String() string {
	var totredraw int64
	for _, t := range s.Redraw {
		totredraw += t.Microseconds()
	}
	var totevent int64
	for _, t := range s.Event {
		totevent += t.Microseconds()
	}
	var totalloc uint64
	for _, t := range s.Alloc {
		totalloc += t
	}

	b := &bytes.Buffer{}
	fmt.Fprintf(b, "avg redraw time: %.3f us\n", float64(totredraw)/float64(len(s.Redraw)))
	fmt.Fprintf(b, "avg event time: %.3f us\n", float64(totevent)/float64(len(s.Event)))
	fmt.Fprintf(b, "avg alloc: %s\n", humanize.Bytes(uint64(float64(totalloc)/float64(len(s.Alloc)))))
	return b.String()
}
