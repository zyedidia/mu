package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// newTestEditor creates an Editor without a tcell screen, suitable for
// testing commands and TCL integration.
func newTestEditor() *Editor {
	cfg, _ := LoadConfig()
	regs := NewRegisterSet()
	ed := &Editor{
		config:  cfg,
		theme:   DefaultTheme,
		ks:      NewKeyState(nil, regs),
		regs:    regs,
		infobar: NewInfoBar(),
		running: true,
		w:       80,
		h:       24,
	}
	SetupBindings(ed.ks)
	ed.ks.activeView = func() *View {
		return ed.ActiveView()
	}
	ed.ks.dispatch = ed.dispatchKey
	ed.ks.recordJump = ed.pushJump
	ed.comments = cfg.LoadComments()
	ed.ks.commentPrefix = func(b *Buffer) string {
		return ed.comments[b.Filetype]
	}
	ed.initTCL()
	ed.registerEditorBindings()
	ed.registerSearchBindings()

	// Open an empty buffer in a tab. Use a temp path so tests that write
	// (e.g. :wq) don't drop files into the working directory.
	buf := NewEmptyBuffer()
	buf.Path = filepath.Join(os.TempDir(), fmt.Sprintf("mu-test-%d.txt", os.Getpid()))
	v := NewView(buf, 4)
	v.Resize(80, 22)
	ed.NewTabWithView(v)
	ed.ks.SetBuffer(buf)

	return ed
}

// --- Alias expansion ---

func TestExpandAlias(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"q", "quit"},
		{"q!", "quit!"},
		{"w", "write"},
		{"w foo.txt", "write foo.txt"},
		{"wq", "write; quit"},
		{"wa", "writeall"},
		{"wqa", "writeall; quitall"},
		{"wqa!", "writeall; quitall!"},
		{"xa", "writeall; quitall"},
		{"e foo.go", "edit foo.go"},
		{"unknown", "unknown"},
		{"quit", "quit"},
	}
	for _, tt := range tests {
		got := expandAlias(tt.input)
		if got != tt.want {
			t.Errorf("expandAlias(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- Command execution ---

func TestCmdQuit(t *testing.T) {
	ed := newTestEditor()
	ed.RunCommand("q")
	if ed.running {
		t.Fatal("quit should stop the editor")
	}
}

func TestCmdQuitModified(t *testing.T) {
	ed := newTestEditor()
	ed.ActiveView().buf.Insert(0, []byte("dirty"))
	ed.RunCommand("q")
	if !ed.running {
		t.Fatal("quit should fail on modified buffer")
	}
	if !ed.infobar.msgErr {
		t.Fatal("should show error message")
	}
}

func TestCmdForceQuit(t *testing.T) {
	ed := newTestEditor()
	ed.ActiveView().buf.Insert(0, []byte("dirty"))
	ed.RunCommand("q!")
	if ed.running {
		t.Fatal("q! should force quit even with modifications")
	}
}

func TestCmdEdit(t *testing.T) {
	ed := newTestEditor()
	ed.RunCommand("e command_test.go")
	// edit replaces the current pane, so the buffer path should change.
	v := ed.ActiveView()
	if v == nil || v.buf.Path != "command_test.go" {
		path := ""
		if v != nil {
			path = v.buf.Path
		}
		t.Fatalf("edit should open file: got path %q", path)
	}
}

func TestCmdUnknown(t *testing.T) {
	ed := newTestEditor()
	ed.RunCommand("nonexistent")
	if !ed.infobar.msgErr {
		t.Fatal("unknown command should show error")
	}
}

// --- :wa / :wqa ---

// setupWriteAllTest isolates config/data dirs and returns an editor whose
// default buffer and a second tab's buffer are both modified.
func setupWriteAllTest(t *testing.T) (*Editor, string, string) {
	t.Helper()
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()
	path1 := ed.ActiveView().buf.Path
	os.Remove(path1)
	t.Cleanup(func() { os.Remove(path1) })
	ed.ActiveView().buf.Insert(0, []byte("one\n"))

	path2 := filepath.Join(t.TempDir(), "two.txt")
	os.WriteFile(path2, []byte("old\n"), 0644)
	if err := ed.OpenFileInTab(path2); err != nil {
		t.Fatal(err)
	}
	ed.ActiveView().buf.Insert(0, []byte("two "))
	return ed, path1, path2
}

func TestCmdWriteAll(t *testing.T) {
	ed, path1, path2 := setupWriteAllTest(t)

	ed.RunCommand("wa")
	if ed.infobar.msgErr {
		t.Fatalf("wa error: %s", ed.infobar.message)
	}
	if data, _ := os.ReadFile(path1); string(data) != "one\n" {
		t.Fatalf("path1 = %q, want %q", data, "one\n")
	}
	if data, _ := os.ReadFile(path2); string(data) != "two old\n" {
		t.Fatalf("path2 = %q, want %q", data, "two old\n")
	}
	for _, tab := range ed.tabs {
		for _, v := range tab.panes {
			if v.buf.Modified() {
				t.Fatalf("buffer %q still modified after wa", v.buf.Path)
			}
		}
	}
	if !ed.running {
		t.Fatal("wa must not quit the editor")
	}
	if ed.infobar.message != "2 buffers written" {
		t.Fatalf("message = %q, want %q", ed.infobar.message, "2 buffers written")
	}
}

func TestCmdWqa(t *testing.T) {
	ed, path1, path2 := setupWriteAllTest(t)

	ed.RunCommand("wqa")
	if ed.running {
		t.Fatal("wqa should quit the editor")
	}
	if data, _ := os.ReadFile(path1); string(data) != "one\n" {
		t.Fatalf("path1 = %q, want %q", data, "one\n")
	}
	if data, _ := os.ReadFile(path2); string(data) != "two old\n" {
		t.Fatalf("path2 = %q, want %q", data, "two old\n")
	}
}

func TestCmdXaAlias(t *testing.T) {
	ed, _, _ := setupWriteAllTest(t)

	ed.RunCommand("xa")
	if ed.running {
		t.Fatal("xa should quit the editor")
	}
}

func TestCmdWqaRefusesOnUnnamed(t *testing.T) {
	// A modified buffer without a file name can't be written: wqa writes
	// the others, reports the error, and refuses to quit (as in vim).
	ed, path1, _ := setupWriteAllTest(t)
	ed.RunCommand("tabnew")
	ed.ActiveView().buf.Insert(0, []byte("scratch"))

	ed.RunCommand("wqa")
	if ed.running == false {
		t.Fatal("wqa must not quit while a buffer cannot be written")
	}
	if !ed.infobar.msgErr {
		t.Fatal("wqa should report the unwritable buffer")
	}
	// The writable buffers were still saved.
	if data, _ := os.ReadFile(path1); string(data) != "one\n" {
		t.Fatalf("path1 = %q, want %q (written despite the error)", data, "one\n")
	}
}

func TestCmdWriteAllSkipsUnmodified(t *testing.T) {
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()
	path := ed.ActiveView().buf.Path
	os.Remove(path)

	ed.RunCommand("wa")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("wa should not write unmodified buffers")
	}
	if ed.infobar.msgErr {
		t.Fatalf("wa error: %s", ed.infobar.message)
	}
}

// --- :set ---

func TestCmdSetShow(t *testing.T) {
	ed := newTestEditor()
	ed.RunCommand("set theme")
	if ed.infobar.message == "" {
		t.Fatal("set <name> should display current value")
	}
}

func TestCmdSetGlobalOption(t *testing.T) {
	ed := newTestEditor()
	ed.RunCommand("set theme monokai")
	if ed.config.GlobalStrOpt("theme") != "monokai" {
		t.Fatalf("theme should be monokai, got %q", ed.config.GlobalStrOpt("theme"))
	}
}

func TestCmdSetBoolOption(t *testing.T) {
	ed := newTestEditor()
	// tabstospaces is a bool option from defaults.
	ed.RunCommand("set tabstospaces false")
	// Check the message confirms it was set.
	if ed.infobar.msgErr {
		t.Fatalf("set bool: unexpected error: %s", ed.infobar.message)
	}
}

// --- TCL integration ---

func TestTCLEvalQuit(t *testing.T) {
	ed := newTestEditor()
	err := ed.EvalTCL("quit!")
	if err != nil {
		t.Fatal(err)
	}
	if ed.running {
		t.Fatal("TCL quit! should stop the editor")
	}
}

func TestTCLEvalCompound(t *testing.T) {
	ed := newTestEditor()
	// The wq alias expands to "write; quit": the write goes to the test
	// buffer's temp path, then quit runs.
	defer os.Remove(ed.ActiveView().buf.Path)
	ed.RunCommand("wq")
	if ed.running {
		t.Fatal("wq should quit the editor")
	}
}

func TestTCLEvalError(t *testing.T) {
	ed := newTestEditor()
	err := ed.EvalTCL("this-command-does-not-exist")
	if err == nil {
		t.Fatal("unknown TCL command should return error")
	}
}

func TestTCLSetCmd(t *testing.T) {
	ed := newTestEditor()
	err := ed.EvalTCL("set theme monokai")
	if err != nil {
		t.Fatal(err)
	}
	if ed.config.GlobalStrOpt("theme") != "monokai" {
		t.Fatal("set should update the option")
	}
}

// --- Colon binding integration ---

func TestColonPrompt(t *testing.T) {
	ed := newTestEditor()

	// Simulate typing ":q<CR>" via KeyState.
	ed.ks.HandleKey(":")

	if !ed.infobar.IsActive() {
		t.Fatal("colon should activate infobar prompt")
	}

	ed.infobar.HandleKey("q")
	ed.infobar.HandleKey(KeyEnter)

	if ed.infobar.IsActive() {
		t.Fatal("prompt should close after enter")
	}
	if ed.running {
		t.Fatal("q command should quit")
	}
}
