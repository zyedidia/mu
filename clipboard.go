package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// System clipboard integration for the '+' and '*' registers, selected by
// the clipboard option:
//
//	internal  the registers stay ordinary internal registers
//	external  a system clipboard tool is used (wl-clipboard, pbcopy,
//	          xclip, xsel) — full read/write
//	terminal  OSC 52 escape sequences through the terminal (works over
//	          SSH). Writes are immediate; reads depend on the terminal
//	          allowing them and refresh the register asynchronously.

// clipboardCmds holds the detected external clipboard commands.
type clipboardCmds struct {
	copyCmd  []string
	pasteCmd []string
}

// detectClipboardCmds finds a usable external clipboard tool.
func detectClipboardCmds() *clipboardCmds {
	type tool struct {
		copyCmd  []string
		pasteCmd []string
		when     func() bool
	}
	tools := []tool{
		{
			copyCmd:  []string{"wl-copy"},
			pasteCmd: []string{"wl-paste", "-n"},
			when:     func() bool { return os.Getenv("WAYLAND_DISPLAY") != "" },
		},
		{
			copyCmd:  []string{"pbcopy"},
			pasteCmd: []string{"pbpaste"},
			when:     func() bool { return runtime.GOOS == "darwin" },
		},
		{
			copyCmd:  []string{"xclip", "-i", "-selection", "clipboard"},
			pasteCmd: []string{"xclip", "-o", "-selection", "clipboard"},
		},
		{
			copyCmd:  []string{"xsel", "--input", "--clipboard"},
			pasteCmd: []string{"xsel", "--output", "--clipboard"},
		},
	}
	for _, t := range tools {
		if t.when != nil && !t.when() {
			continue
		}
		if _, err := exec.LookPath(t.copyCmd[0]); err != nil {
			continue
		}
		if _, err := exec.LookPath(t.pasteCmd[0]); err != nil {
			continue
		}
		return &clipboardCmds{copyCmd: t.copyCmd, pasteCmd: t.pasteCmd}
	}
	return nil
}

// clipboardTimeout bounds external tool runs so a wedged clipboard helper
// can't freeze the editor.
const clipboardTimeout = 3 * time.Second

func (c *clipboardCmds) write(data []byte) bool {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.copyCmd[0], c.copyCmd[1:]...)
	cmd.Stdin = bytes.NewReader(data)
	if err := cmd.Run(); err != nil {
		log.Printf("clipboard: %s: %v", c.copyCmd[0], err)
		return false
	}
	return true
}

func (c *clipboardCmds) read() ([]byte, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, c.pasteCmd[0], c.pasteCmd[1:]...).Output()
	if err != nil {
		log.Printf("clipboard: %s: %v", c.pasteCmd[0], err)
		return nil, false
	}
	return out, true
}

// initClipboard connects the '+'/'*' registers to the system clipboard
// according to the clipboard option. On failure the registers stay
// internal and an error is returned for the caller to report.
func (e *Editor) initClipboard() error {
	e.regs.ReadClip = nil
	e.regs.WriteClip = nil
	switch e.config.GlobalStrOpt("clipboard") {
	case "external":
		cmds := detectClipboardCmds()
		if cmds == nil {
			return fmt.Errorf("clipboard: no clipboard tool found (wl-clipboard, pbcopy, xclip, xsel); '+' register is internal")
		}
		e.regs.WriteClip = cmds.write
		e.regs.ReadClip = cmds.read
	case "terminal":
		e.regs.WriteClip = func(data []byte) bool {
			if e.screen == nil {
				return false
			}
			e.screen.SetClipboard(data)
			return true
		}
		// A read requests the clipboard via OSC 52; the terminal's answer
		// (if it allows reads) arrives as an EventClipboard and refreshes
		// the register. The current paste uses the register's stored
		// content.
		e.regs.ReadClip = func() ([]byte, bool) {
			if e.screen != nil {
				e.screen.GetClipboard()
			}
			return nil, false
		}
	}
	return nil
}
