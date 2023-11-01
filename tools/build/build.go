package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func run(dir, cmd string, args ...string) string {
	c := exec.Command(cmd, args...)
	c.Dir = dir
	b := &bytes.Buffer{}
	c.Stdout = b
	c.Stderr = os.Stderr
	fmt.Println(c)
	err := c.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return strings.TrimSpace(b.String())
}

func main() {
	version := run("tools/version", "go", "run", "version.go")
	date := run("tools/date", "go", "run", "date.go")
	commit := run(".", "git", "rev-parse", "--short", "HEAD")
	run(".", "go", "build", "-trimpath", "-ldflags",
		fmt.Sprintf("-s -w -X 'github.com/zyedidia/mu/build.Version=%s' -X 'github.com/zyedidia/mu/build.CompileDate=%s' -X 'github.com/zyedidia/mu/build.CommitHash=%s'", version, date, commit),
		"-tags", "flare_custom,ftdetect_custom", "./cmd/mu",
	)
}
