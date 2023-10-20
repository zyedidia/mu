package shell

import (
	"io"
	"os"
	"os/exec"
)

func Run(cmd string) error {
	return RunWith(cmd, os.Stdin, os.Stdout, os.Stderr)
}

func RunWith(cmd string, stdin io.Reader, stdout, stderr io.Writer) error {
	c := exec.Command("sh", "-c", cmd)
	c.Stdin = stdin
	c.Stdout = stdout
	c.Stderr = stderr
	return c.Run()
}
