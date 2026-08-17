package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// probeTimeout guards the FIFO tests: a regression makes the probe block
// forever, so run it in a goroutine and fail fast instead of hanging.
const probeTimeout = 2 * time.Second

func timedProbe(t *testing.T, what string, fn func() bool) bool {
	t.Helper()
	done := make(chan bool, 1)
	go func() { done <- fn() }()
	select {
	case v := <-done:
		return v
	case <-time.After(probeTimeout):
		t.Fatalf("%s blocked", what)
		return false
	}
}

func TestIsReadonlyRegular(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("x\n"), 0644)

	if isReadonly(path) {
		t.Fatal("writable file reported readonly")
	}
	os.Chmod(path, 0444)
	if os.Getuid() != 0 && !isReadonly(path) {
		t.Fatal("read-only file not reported")
	}
	if isReadonly(filepath.Join(dir, "does-not-exist")) {
		t.Fatal("nonexistent file reported readonly")
	}
}

func TestIsReadonlyFifoDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}

	// A writable FIFO: not readonly, and the probe must not hang waiting
	// for a reader.
	if timedProbe(t, "isReadonly on writable FIFO", func() bool { return isReadonly(fifo) }) {
		t.Fatal("writable FIFO reported readonly")
	}

	// Permission bits decide for special files.
	os.Chmod(fifo, 0444)
	if !timedProbe(t, "isReadonly on read-only FIFO", func() bool { return isReadonly(fifo) }) {
		t.Fatal("read-only FIFO not reported")
	}
}

func TestOpenFifoRefused(t *testing.T) {
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()
	fifo := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(fifo, 0644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}

	errc := make(chan error, 1)
	go func() { errc <- ed.OpenFile(fifo) }()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("opening a FIFO should error")
		}
		if !strings.Contains(err.Error(), "named pipe") {
			t.Fatalf("error = %v, want a named-pipe message", err)
		}
	case <-time.After(probeTimeout):
		t.Fatal("opening a FIFO blocked")
	}

	// The editor is still usable.
	b := ed.ks.Buf()
	b.text.Insert(0, []byte("ok\n"))
	feedKeys(ed.ks, "x")
	if bufText(ed.ks) != "k\n" {
		t.Fatalf("editor state after refused open: %q", bufText(ed.ks))
	}
}
