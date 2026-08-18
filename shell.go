package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
)

// runShellCommand implements :!cmd — suspend the screen, run cmdline
// through the shell with the terminal attached, wait for enter, resume.
// SIGINT is swallowed for the whole suspended stretch: Ctrl-C goes to the
// foreground process group, so it interrupts the command while the editor
// (and the press-enter prompt) survive it.
func (e *Editor) runShellCommand(cmdline string) {
	cmdline = strings.TrimSpace(cmdline)
	if cmdline == "" {
		return
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt)
	defer signal.Stop(sigch)

	if e.screen != nil {
		e.screen.Suspend()
	}

	cmd := exec.Command(shell, "-c", cmdline)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()

	if e.screen != nil {
		// Report the outcome and hold the plain screen until enter, so
		// the command's output stays readable before the alternate
		// screen returns.
		if ee, ok := err.(*exec.ExitError); ok {
			if code := ee.ExitCode(); code > 0 {
				fmt.Printf("\nshell returned %d\n", code)
			} else {
				fmt.Printf("\ninterrupted\n")
			}
		} else if err != nil {
			fmt.Printf("\n%v\n", err)
		}
		fmt.Print("Press enter to continue")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		e.screen.Resume()
		e.screen.Sync()
	}
}
