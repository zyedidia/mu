package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newClipState is a KeyState whose register set uses in-memory clipboard
// hooks backed by the returned store.
func newClipState(text string) (*KeyState, *[]byte) {
	ks := newVimState(text)
	store := &[]byte{}
	ks.regs.WriteClip = func(data []byte) bool {
		*store = append([]byte(nil), data...)
		return true
	}
	ks.regs.ReadClip = func() ([]byte, bool) {
		return *store, true
	}
	return ks, store
}

func TestClipboardYankWrites(t *testing.T) {
	ks, store := newClipState("hello\nworld\n")

	feedKeys(ks, "\"+yy")
	if string(*store) != "hello\n" {
		t.Fatalf("clipboard after \"+yy: got %q", *store)
	}
}

func TestClipboardPasteForeignCharwise(t *testing.T) {
	ks, store := newClipState("abc\n")
	*store = []byte("XY") // placed by another program

	feedKeys(ks, "\"+p")
	if bufText(ks) != "aXYbc\n" {
		t.Fatalf("\"+p foreign: got %q", bufText(ks))
	}
}

func TestClipboardPasteForeignLinewise(t *testing.T) {
	// Foreign content ending in a newline pastes linewise, as in vim.
	ks, store := newClipState("abc\n")
	*store = []byte("new line\n")

	feedKeys(ks, "\"+p")
	if bufText(ks) != "abc\nnew line\n" {
		t.Fatalf("\"+p foreign linewise: got %q", bufText(ks))
	}
}

func TestClipboardRoundTripKeepsType(t *testing.T) {
	// When the system clipboard still holds what mu wrote, the stored
	// register type (blockwise here) is preserved on paste.
	ks, store := newClipState("ab\ncd\n")

	feedDisplay(ks, "\"+", "<C-v>", "j", "y")
	if string(*store) != "a\nc" {
		t.Fatalf("block yank to clipboard: got %q", *store)
	}
	feedDisplay(ks, "l", "\"+p")
	if bufText(ks) != "aba\ncdc\n" {
		t.Fatalf("blockwise clipboard paste: got %q", bufText(ks))
	}
}

func TestClipboardStarAlias(t *testing.T) {
	// "* and "+ are the same clipboard register.
	ks, store := newClipState("hello\n")

	feedKeys(ks, "\"*yy")
	if string(*store) != "hello\n" {
		t.Fatalf("clipboard after \"*yy: got %q", *store)
	}
	*store = []byte("ext\n")
	feedKeys(ks, "\"+p")
	if bufText(ks) != "hello\next\n" {
		t.Fatalf("\"+p after \"*yy: got %q", bufText(ks))
	}
}

// --- External tool detection and round trip ---

// setupFakeClipTool installs a fake xclip on PATH that stores the clipboard
// in a file, and disables the wayland path.
func setupFakeClipTool(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	clip := filepath.Join(t.TempDir(), "clip")
	// The test replaces PATH, so the script restores a standard one for cat.
	script := "#!/bin/sh\nPATH=/usr/bin:/bin\nif [ \"$1\" = \"-o\" ]; then\n  cat \"$CLIP\"\nelse\n  cat > \"$CLIP\"\nfi\n"
	if err := os.WriteFile(filepath.Join(dir, "xclip"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("CLIP", clip)
	t.Setenv("WAYLAND_DISPLAY", "")
	return clip
}

func TestDetectClipboardCmds(t *testing.T) {
	setupFakeClipTool(t)
	cmds := detectClipboardCmds()
	if cmds == nil {
		t.Fatal("fake xclip not detected")
	}
	if !cmds.write([]byte("round trip")) {
		t.Fatal("clipboard write failed")
	}
	data, ok := cmds.read()
	if !ok || string(data) != "round trip" {
		t.Fatalf("clipboard read: got %q, %v", data, ok)
	}

	t.Setenv("PATH", t.TempDir()) // nothing on PATH
	if detectClipboardCmds() != nil {
		t.Fatal("detection should fail with an empty PATH")
	}
}

// --- Editor integration ---

func TestClipboardExternalEditor(t *testing.T) {
	clip := setupFakeClipTool(t)
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()
	if err := ed.initClipboard(); err != nil {
		t.Fatal(err)
	}
	b := ed.ks.Buf()
	b.text.Insert(0, []byte("hello\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	// Yank to the system clipboard.
	feedKeys(ed.ks, "\"+yy")
	if data, _ := os.ReadFile(clip); string(data) != "hello\n" {
		t.Fatalf("clip file after \"+yy: got %q", data)
	}

	// Another program replaces the clipboard; paste picks it up.
	os.WriteFile(clip, []byte("external\n"), 0644)
	feedKeys(ed.ks, "\"+p")
	if got := bufText(ed.ks); got != "hello\nexternal\n" {
		t.Fatalf("\"+p external: got %q", got)
	}
}

func TestClipboardSetOption(t *testing.T) {
	setupFakeClipTool(t)
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()

	ed.RunCommand("set clipboard internal")
	if ed.regs.ReadClip != nil || ed.regs.WriteClip != nil {
		t.Fatal("internal mode should disconnect the clipboard hooks")
	}

	ed.RunCommand("set clipboard external")
	if ed.regs.ReadClip == nil || ed.regs.WriteClip == nil {
		t.Fatal("external mode should connect the clipboard hooks")
	}

	// With no tool available, :set clipboard external reports an error and
	// stays internal.
	t.Setenv("PATH", t.TempDir())
	ed.RunCommand("set clipboard internal")
	ed.infobar.Clear()
	ed.RunCommand("set clipboard external")
	if !strings.Contains(ed.infobar.message, "clipboard") {
		t.Fatalf("expected clipboard error, got %q", ed.infobar.message)
	}
	if ed.regs.WriteClip != nil {
		t.Fatal("hooks must stay nil when no tool is found")
	}
}

func TestClipboardTerminalMode(t *testing.T) {
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()
	if err := ed.config.SetGlobalOpt("clipboard", "terminal"); err != nil {
		t.Fatal(err)
	}
	if err := ed.initClipboard(); err != nil {
		t.Fatal(err)
	}
	b := ed.ks.Buf()
	b.text.Insert(0, []byte("hello\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	// Without a terminal answering reads, the register still round-trips
	// internally.
	feedKeys(ed.ks, "\"+yy")
	feedKeys(ed.ks, "\"+p")
	if got := bufText(ed.ks); got != "hello\nhello\n" {
		t.Fatalf("terminal mode yank/paste: got %q", got)
	}

	// A terminal OSC 52 response refreshes the register.
	ed.regs.storeClipboard([]byte("from terminal\n"))
	feedKeys(ed.ks, "\"+p")
	if !strings.Contains(bufText(ed.ks), "from terminal\n") {
		t.Fatalf("OSC52 response paste: got %q", bufText(ed.ks))
	}
}
