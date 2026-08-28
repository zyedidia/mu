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
		mainq:   make(chan func(), 128),
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
		{"w!", "write!"},
		{"w! foo.txt", "write! foo.txt"},
		{"wq!", "write!; quit"},
		{"x!", "write!; quit"},
		{"e!", "edit!"},
		{"bd!", "bdelete!"},
		{"tabe foo.go", "tabnew foo.go"},
		{"tabedit foo.go", "tabnew foo.go"},
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

func TestWcCommand(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("hello world\n  foo\tbar baz\n\nx\n"))

	ed.RunCommand("wc")
	want := "4 lines, 6 words, 29 bytes"
	if ed.infobar.message != want {
		t.Fatalf("wc: %q, want %q", ed.infobar.message, want)
	}
}

// --- ~ expansion ---

func TestExpandTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := []struct{ in, want string }{
		{"~", home},
		{"~/", home},
		{"~/notes.txt", filepath.Join(home, "notes.txt")},
		{"~/a/b/c.txt", filepath.Join(home, "a/b/c.txt")},
		{"~root/x", "~root/x"}, // ~user is not expanded
		{"~notatilde", "~notatilde"},
		{"/abs/~/x", "/abs/~/x"}, // only a leading tilde counts
		{"rel/~", "rel/~"},
		{"", ""},
		{"plain.txt", "plain.txt"},
	}
	for _, tt := range tests {
		if got := expandTilde(tt.in); got != tt.want {
			t.Errorf("expandTilde(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// :e ~/file opens the file under the home directory, not a literal "~" one.
func TestCmdEditExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	target := filepath.Join(home, "XXX")
	if err := os.WriteFile(target, []byte("from home\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ed := newTestEditor()
	ed.RunCommand("e ~/XXX")

	b := ed.ActiveView().buf
	if b.Path != target {
		t.Fatalf("path = %q, want %q (infobar: %q)", b.Path, target, ed.infobar.message)
	}
	if got := string(b.text.Slice(0, b.Len())); got != "from home\n" {
		t.Errorf("contents = %q, want %q", got, "from home\n")
	}
}

// The expansion has to happen before :e decides between reloading the
// current file and opening another one, or ":e ~/current" would re-show the
// buffer instead of rereading it from disk.
func TestCmdEditTildeRereadsCurrentFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	target := filepath.Join(home, "XXX")
	os.WriteFile(target, []byte("one\n"), 0644)

	ed := newTestEditor()
	ed.RunCommand("e ~/XXX")
	before := len(ed.buffers)

	os.WriteFile(target, []byte("two\n"), 0644)
	ed.RunCommand("e ~/XXX")

	if len(ed.buffers) != before {
		t.Errorf("buffer count %d -> %d: reopened instead of reloading", before, len(ed.buffers))
	}
	b := ed.ActiveView().buf
	if got := string(b.text.Slice(0, b.Len())); got != "two\n" {
		t.Errorf("contents = %q, want the reloaded %q", got, "two\n")
	}
}

func TestCmdWriteExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("saved\n"))
	ed.RunCommand("w ~/out.txt")

	data, err := os.ReadFile(filepath.Join(home, "out.txt"))
	if err != nil {
		t.Fatalf("write to ~/out.txt: %v (infobar: %q)", err, ed.infobar.message)
	}
	if string(data) != "saved\n" {
		t.Errorf("wrote %q, want %q", data, "saved\n")
	}
}

// The other commands that take a file name expand it too.
func TestTildeExpandsForSplitTabAndBuffer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	target := filepath.Join(home, "XXX")
	os.WriteFile(target, []byte("x\n"), 0644)

	for _, cmd := range []string{"vs ~/XXX", "sp ~/XXX", "tabe ~/XXX"} {
		ed := newTestEditor()
		ed.RunCommand(cmd)
		if got := ed.ActiveView().buf.Path; got != target {
			t.Errorf("%q opened %q, want %q (infobar: %q)", cmd, got, target, ed.infobar.message)
		}
	}

	// :b matches an open buffer by path, which also has to be expanded.
	ed := newTestEditor()
	ed.RunCommand("e ~/XXX")
	ed.RunCommand("e other.txt")
	ed.RunCommand("b ~/XXX")
	if got := ed.ActiveView().buf.Path; got != target {
		t.Errorf(":b ~/XXX showed %q, want %q (infobar: %q)", got, target, ed.infobar.message)
	}
}

// Completion offers ~ paths in the form they were typed.
func TestCompleteFilePathTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.WriteFile(filepath.Join(home, "zzz-note.txt"), nil, 0644)
	os.MkdirAll(filepath.Join(home, "zzz-dir"), 0755)

	got := completeFilePath("~/zzz-")
	want := []string{"~/zzz-dir" + string(filepath.Separator), "~/zzz-note.txt"}
	if len(got) != len(want) {
		t.Fatalf("completions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("completions = %v, want %v", got, want)
		}
	}
}
