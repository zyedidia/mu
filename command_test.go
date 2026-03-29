package main

import (
	"testing"
)

// newTestEditor creates an Editor without a tcell screen, suitable for
// testing commands and TCL integration.
func newTestEditor() *Editor {
	cfg, _ := LoadConfig()
	ed := &Editor{
		config:  cfg,
		theme:   DefaultTheme,
		ks:      NewKeyState(nil, NewRegisterSet()),
		regs:    NewRegisterSet(),
		infobar: NewInfoBar(),
		running: true,
		w:       80,
		h:       24,
	}
	SetupBindings(ed.ks)
	ed.initTCL()
	ed.registerEditorBindings()
	ed.registerSearchBindings()

	// Open an empty buffer.
	buf := NewEmptyBuffer()
	buf.Path = "test.txt"
	v := NewView(buf, 4)
	v.Resize(80, 22)
	ed.views = append(ed.views, v)
	ed.active = 0
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
	initial := len(ed.views)
	ed.RunCommand("e command_test.go")
	if len(ed.views) != initial+1 {
		t.Fatalf("edit should open new view: got %d views, want %d", len(ed.views), initial+1)
	}
}

func TestCmdUnknown(t *testing.T) {
	ed := newTestEditor()
	ed.RunCommand("nonexistent")
	if !ed.infobar.msgErr {
		t.Fatal("unknown command should show error")
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
	// The wq alias expands to "write; quit". Since write is a TODO stub,
	// it won't fail but quit should still execute.
	ed.RunCommand("wq")
	// write shows a TODO message but doesn't error; quit runs next.
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
