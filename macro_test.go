package main

import (
	"testing"
)

// edKeys feeds keys through the editor's full key routing (infobar
// included), the way real keystrokes and macro replays travel.
func edKeys(ed *Editor, keys ...string) {
	for _, k := range keys {
		for _, ch := range splitKeys(k) {
			ed.dispatchKey(ch)
		}
	}
}

func TestMacroRecordReplay(t *testing.T) {
	ks := newVimState("abcdef\n")

	feedKeys(ks, "qaxq") // record: delete one char
	if bufText(ks) != "bcdef\n" {
		t.Fatalf("after recording: got %q", bufText(ks))
	}
	if got := string(ks.regs.Get(RegisterID('a')).Content); got != "x" {
		t.Fatalf("register a = %q, want %q", got, "x")
	}
	feedKeys(ks, "@a")
	if bufText(ks) != "cdef\n" {
		t.Fatalf("@a: got %q", bufText(ks))
	}
}

func TestMacroCount(t *testing.T) {
	ks := newVimState("abcdef\n")
	ks.regs.Set(RegisterID('a'), []byte("x"), false)

	feedKeys(ks, "3@a")
	if bufText(ks) != "def\n" {
		t.Fatalf("3@a: got %q", bufText(ks))
	}
}

func TestMacroAtAt(t *testing.T) {
	ks := newVimState("abcdef\n")
	ks.regs.Set(RegisterID('a'), []byte("x"), false)

	feedKeys(ks, "@a@@")
	if bufText(ks) != "cdef\n" {
		t.Fatalf("@a @@: got %q", bufText(ks))
	}
}

func TestMacroInsertKeys(t *testing.T) {
	// Special keys record in key notation and replay through insert mode.
	ks := newVimState("a\nb\n")

	feedKeys(ks, "qa")
	feedKeys(ks, "A!")
	feedSpecial(ks, KeyEscape)
	feedKeys(ks, "q")
	if got := string(ks.regs.Get(RegisterID('a')).Content); got != "A!<Esc>" {
		t.Fatalf("register a = %q, want %q", got, "A!<Esc>")
	}
	feedKeys(ks, "j@a")
	if bufText(ks) != "a!\nb!\n" {
		t.Fatalf("replayed insert macro: got %q", bufText(ks))
	}
}

func TestMacroUppercaseAppends(t *testing.T) {
	ks := newVimState("abcdef\n")

	feedKeys(ks, "qaxq") // a = "x"
	feedKeys(ks, "qAxq") // append: a = "xx"
	if got := string(ks.regs.Get(RegisterID('a')).Content); got != "xx" {
		t.Fatalf("register a = %q, want %q", got, "xx")
	}
	feedKeys(ks, "@a")
	if bufText(ks) != "ef\n" { // two deletes recorded + two replayed
		t.Fatalf("@a after append: got %q", bufText(ks))
	}
}

func TestMacroRecursionTerminates(t *testing.T) {
	ks := newVimState("aaaaaaaa\n")
	ks.regs.Set(RegisterID('a'), []byte("x@a"), false)

	feedKeys(ks, "@a") // must terminate via the depth limit
	if ks.ModeID() != ModeNormal {
		t.Fatal("should end in normal mode")
	}
}

func TestMacroDotRepeatsInnerChange(t *testing.T) {
	// . after @a repeats the macro's last change, as in vim.
	ks := newVimState("abcdef\n")
	ks.regs.Set(RegisterID('a'), []byte("x"), false)

	feedKeys(ks, "@a.")
	if bufText(ks) != "cdef\n" {
		t.Fatalf("@a .: got %q", bufText(ks))
	}
}

func TestMacroPasteAsText(t *testing.T) {
	// Macros live in ordinary registers: "ap pastes the recorded keys.
	ks := newVimState("\n")

	feedKeys(ks, "qaxq")
	feedKeys(ks, "\"ap")
	if bufText(ks) != "x\n" {
		t.Fatalf("\"ap: got %q", bufText(ks))
	}
}

// --- Visual mode: apply per line ---

func TestMacroVisualPerLine(t *testing.T) {
	ks := newVimState("a\nb\nc\nd\n")
	ks.regs.Set(RegisterID('q'), []byte("A!<Esc>"), false)

	feedDisplay(ks, "V", "jj", "@q")
	if bufText(ks) != "a!\nb!\nc!\nd\n" {
		t.Fatalf("visual @q: got %q", bufText(ks))
	}
	if ks.ModeID() != ModeNormal {
		t.Fatal("should be back in normal mode")
	}
}

func TestMacroVisualCharwise(t *testing.T) {
	// A charwise selection still applies per touched line.
	ks := newVimState("aa\nbb\n")
	ks.regs.Set(RegisterID('q'), []byte("A!<Esc>"), false)

	feedDisplay(ks, "v", "j", "@q")
	if bufText(ks) != "aa!\nbb!\n" {
		t.Fatalf("charwise visual @q: got %q", bufText(ks))
	}
}

func TestMacroVisualLineDelta(t *testing.T) {
	// A macro that deletes lines: the range tracks the shrinking buffer
	// (matching vim's :'<,'>normal quirk of skipping shifted lines).
	ks := newVimState("a\nb\nc\nd\n")
	ks.regs.Set(RegisterID('q'), []byte("dd"), false)

	feedDisplay(ks, "V", "jj", "@q")
	if bufText(ks) != "b\nd\n" {
		t.Fatalf("visual @q with dd: got %q", bufText(ks))
	}
}

// --- Editor integration ---

func TestMacroWithSearch(t *testing.T) {
	// A macro containing a '/' search records the prompt keys and replays
	// them through the same routing.
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()
	b := ed.ks.Buf()
	b.text.Insert(0, []byte("Xbaz\nYbaz\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	edKeys(ed, "qa", "/baz", "<CR>", "x", "q")
	if got := string(ed.regs.Get(RegisterID('a')).Content); got != "/baz<CR>x" {
		t.Fatalf("register a = %q, want %q", got, "/baz<CR>x")
	}
	if bufText(ed.ks) != "Xaz\nYbaz\n" {
		t.Fatalf("after recording: got %q", bufText(ed.ks))
	}
	edKeys(ed, "@a")
	if bufText(ed.ks) != "Xaz\nYaz\n" {
		t.Fatalf("@a with search: got %q", bufText(ed.ks))
	}
}

func TestMacroSpaceBinding(t *testing.T) {
	// The default init.tcl binds space in visual mode to @q: record with
	// qq...q, select lines, press space.
	configDirOverride = t.TempDir()
	dataDirOverride = t.TempDir()
	t.Cleanup(func() {
		configDirOverride = ""
		dataDirOverride = ""
	})
	ed := newTestEditor()
	ed.RunInitScript()
	b := ed.ks.Buf()
	b.text.Insert(0, []byte("a\nb\nc\n"))
	*b.Cursor() = b.Cursor().MoveTo(0)

	// Record @q: append "!" to the line.
	edKeys(ed, "qq", "A!", "<Esc>", "q")
	if bufText(ed.ks) != "a!\nb\nc\n" {
		t.Fatalf("after recording: got %q", bufText(ed.ks))
	}

	// Select the remaining lines and press space.
	edKeys(ed, "j", "V", "j", " ")
	if bufText(ed.ks) != "a!\nb!\nc!\n" {
		t.Fatalf("space over selection: got %q", bufText(ed.ks))
	}
}

// --- Mappings and replay ---

func TestMacroRemapThroughMapping(t *testing.T) {
	// A macro started from inside a mapping expansion (vmap <Space> @q)
	// must still apply mappings to the keys it plays back: with the 0/^
	// swap, the macro's 0 goes to the first non-blank.
	ed := newMapTestEditor(t, "  abc\n  def\n")
	evalOrFatal(t, ed, "map 0 ^; map ^ 0; vmap <Space> @q")
	ed.regs.Set(RegisterID('q'), []byte("0x"), false)

	edKeys(ed, "V", "j", " ")
	if bufText(ed.ks) != "  bc\n  ef\n" {
		t.Fatalf("macro 0 not remapped through <Space>: got %q", bufText(ed.ks))
	}
}

func TestMacroRemapDirect(t *testing.T) {
	// The same applies to a directly typed @a.
	ed := newMapTestEditor(t, "  abc\n")
	evalOrFatal(t, ed, "map 0 ^; map ^ 0")
	ed.regs.Set(RegisterID('a'), []byte("0x"), false)

	edKeys(ed, "$", "@a")
	if bufText(ed.ks) != "  bc\n" {
		t.Fatalf("macro 0 not remapped: got %q", bufText(ed.ks))
	}
}

func TestDotRepeatThroughMapping(t *testing.T) {
	// . invoked via a mapping replays the recorded raw keys with mappings
	// applied again.
	ed := newMapTestEditor(t, "  abcd\n  efgh\n")
	evalOrFatal(t, ed, "map 0 ^; map ^ 0; nmap Q .")

	feedDisplay(ed.ks, "$", "d0") // deletes back to the first non-blank
	if bufText(ed.ks) != "  d\n  efgh\n" {
		t.Fatalf("d0 with swap: got %q", bufText(ed.ks))
	}
	feedDisplay(ed.ks, "j$", "Q")
	if bufText(ed.ks) != "  d\n  h\n" {
		t.Fatalf(". through mapping: got %q", bufText(ed.ks))
	}
}

func TestMacroRecordDeadSequence(t *testing.T) {
	// A mapping that shadows an operator prefix (nmap dx ...) makes "dw"
	// resolve through the dead-sequence fallback; the re-fed key must not
	// be recorded twice.
	ed := newMapTestEditor(t, "hello world\n")
	evalOrFatal(t, ed, "nmap dx x")

	feedKeys(ed.ks, "qa")
	feedKeys(ed.ks, "dw")
	feedKeys(ed.ks, "q")
	if got := string(ed.regs.Get(RegisterID('a')).Content); got != "dw" {
		t.Fatalf("register a = %q, want %q", got, "dw")
	}
	if bufText(ed.ks) != "world\n" {
		t.Fatalf("after recording: got %q", bufText(ed.ks))
	}
	feedKeys(ed.ks, "@a")
	if bufText(ed.ks) != "" {
		t.Fatalf("@a: got %q", bufText(ed.ks))
	}
}
