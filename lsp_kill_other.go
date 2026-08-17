//go:build !linux

package main

import "syscall"

// lspSysProcAttr is the non-Linux stub: there is no parent-death signal, so
// cleanup relies on Shutdown and on servers watching the initialize
// ProcessID.
func lspSysProcAttr() *syscall.SysProcAttr {
	return nil
}
