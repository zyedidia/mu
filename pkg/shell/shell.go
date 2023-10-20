package shell

import (
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
)

// Run a shell command connected to stdin/stdout/stderr. The interactive flag
// specifies if interrupts will be caught to kill the process.
func Run(cmd string, interactive bool) error {
	return RunWith(cmd, os.Stdin, os.Stdout, os.Stderr, interactive)
}

// RunWith is the same as Run but specifies custom streams for stdin/stdout/stderr.
func RunWith(command string, stdin io.Reader, stdout, stderr io.Writer, interactive bool) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if interactive {
		// signal handler to catch a possible interrupt while the user is
		// interacting with the password prompt.
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			log.Println("waiting for interrupt")
			s := <-c
			log.Println("interrupt signal received")
			// this channel might have closed, so make sure the received value is
			// the interrupt signal. The process might be nil if the interrupt is
			// received before the command is started (very unlikely).
			if cmd.Process != nil && s == os.Interrupt {
				cmd.Process.Kill()
			}
		}()
	}

	return cmd.Run()
}
