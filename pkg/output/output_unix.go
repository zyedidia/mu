// Atomic file saving is not possible on Windows.
//go:build !windows
// +build !windows

package output

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/google/renameio"
	"github.com/zyedidia/mu/pkg/shell"
)

const (
	HasAtomicFile = false
	HasRootFile   = false
)

// An AtomicFile is similar to a File but uses os.Rename to atomically write
// the file (this way the file doesn't get truncated and partially written if
// there isn't enough disk space). The AtomicFile is a little brittle since OS
// support for transactions in general is limited. It won't work well for
// symlinks or hardlinks.
type AtomicFile struct {
	Path string
}

// Open uses the renameio library to create a new temporary file that can be
// written to. The file is also wrapped with a Close function that performs the
// atomic replacement and any cleanup.
func (afo *AtomicFile) Open() (io.Writer, error) {
	pf, err := renameio.TempFile("", afo.Path)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(afo.Path)
	if err == nil {
		// restore permissions before writing in case of sensitive data.
		if err := pf.Chmod(fi.Mode()); err != nil {
			return nil, err
		}
	}
	return &WriterCloser{
		Wr: pf,
		CloseFn: func() error {
			defer pf.Cleanup()
			return pf.CloseAtomicallyReplace()
		},
	}, nil
}

func (afo *AtomicFile) Name() string {
	return afo.Path
}

func (afo *AtomicFile) FullName() string {
	p, _ := filepath.Abs(afo.Path)
	return p
}

// RootFile saves a file with sudo (or a "sudo"-like command such as "doas") by
// invoking the external process "dd" with root privileges. If the user does
// not currently have root privileges, calling Open may open a password prompt,
// so make sure the screen is available.
type RootFile struct {
	RootCmd string
	Path    string
	Suspend chan func()
	Resume  chan struct{}
}

// Open starts the 'dd' process writing to the output path with root
// privileges. Since this may open an interactive prompt for the user to enter
// their password, it also sets up a signal handler to receive an interrupt (if
// Ctrl-C is pressed). In that case, the subprocess will be killed rather than
// the parent process.
func (rf *RootFile) Open() (io.Writer, error) {
	_, err := exec.LookPath("dd")
	if err != nil {
		return nil, err
	}

	// initialize the 'dd' process with 'sudo' (or whatever the value of 'RootCmd' is).
	cmd := exec.Command(rf.RootCmd, "dd", "bs=4k", "of="+rf.Path)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	// get dd's stdin to write the buffer contents to.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	started := make(chan struct{})

	rf.Suspend <- func() {
		// start the command
		err = cmd.Start()
		started <- struct{}{}
		if err != nil {
			fmt.Println("error saving RootFile:", err)
			shell.EnterToContinue()
		}
	}

	<-started

	// we wrap stdin with a special close function that closes the interrupt
	// signal channel so that the next interrupt is not accidentally caught
	// after 'dd' has completed.
	return &WriterCloser{
		Wr: stdin,
		CloseFn: func() error {
			stdin.Close()
			err = cmd.Wait()
			rf.Resume <- struct{}{}
			return err
		},
	}, nil
}

func (rf *RootFile) Name() string {
	return rf.Path
}

func (rf *RootFile) FullName() string {
	p, _ := filepath.Abs(rf.Path)
	return p
}
