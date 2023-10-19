// Atomic file saving is not possible on Windows.
//go:build !windows
// +build !windows

package output

import (
	"io"
	"os"
	"os/exec"
	"os/signal"

	"github.com/google/renameio"
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

// RootFile saves a file with sudo (or a "sudo"-like command such as "doas") by
// invoking the external process "dd" with root privileges. If the user does
// not currently have root privileges, calling Open may open a password prompt,
// so make sure the screen is available.
type RootFile struct {
	RootCmd string
	Path    string
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

	// get dd's stdin to write the buffer contents to.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	// signal handler to catch a possible interrupt while the user is
	// interacting with the password prompt.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		s := <-c
		// this channel might have closed, so make sure the received value is
		// the interrupt signal. The process might be nil if the interrupt is
		// received before the command is started (very unlikely).
		if cmd.Process != nil && s == os.Interrupt {
			cmd.Process.Kill()
		}
	}()

	// start the command
	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	// we wrap stdin with a special close function that closes the interrupt
	// signal channel so that the next interrupt is not accidentally caught
	// after 'dd' has completed.
	return &WriterCloser{
		Wr: stdin,
		CloseFn: func() error {
			err = cmd.Wait()
			signal.Stop(c)
			close(c)
			stdin.Close()
			return err
		},
	}, nil
}

func (rf *RootFile) Name() string {
	return rf.Path
}
