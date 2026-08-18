//go:build linux

package main

import "syscall"

// lspSysProcAttr asks the kernel to SIGKILL the language server if the
// editor process dies without running its shutdown path (crash, kill -9).
// Well-behaved servers also watch the ProcessID from the initialize params,
// but a hung server never runs that check. The separate process group keeps
// terminal signals away from the server: the Ctrl-C of a :! command goes to
// the foreground group and must not take the LSP down with it.
func lspSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
		Setpgid:   true,
	}
}
