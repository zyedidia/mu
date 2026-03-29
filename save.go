package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
)

// Save writes the buffer to its file path. Creates a backup if configured.
func (b *Buffer) Save() error {
	if b.Path == "" {
		return fmt.Errorf("no file name")
	}
	return b.SaveAs(b.Path)
}

// SaveAs writes the buffer to the given path.
func (b *Buffer) SaveAs(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	if isReadonly(absPath) {
		return fmt.Errorf("%s is read-only", path)
	}

	// Create backup of existing file.
	if fileExists(absPath) {
		if err := createBackup(absPath); err != nil {
			// Non-fatal: log but continue saving.
			_ = err
		}
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("save: mkdir: %w", err)
	}

	// Write to file.
	if err := b.writeToFile(absPath); err != nil {
		return err
	}

	b.Path = path
	b.markUnmodified()
	b.updateModTime()
	return nil
}

// writeToFile creates/truncates the file and writes the buffer contents,
// converting back to the original line endings and charset.
func (b *Buffer) writeToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}
	defer f.Close()

	bw := bufio.NewWriter(f)
	if _, err := b.text.WriteTo(bw); err != nil {
		return fmt.Errorf("save: write: %w", err)
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("save: flush: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("save: sync: %w", err)
	}
	return nil
}

// createBackup copies the existing file to ~/.config/mu/backup/, using
// the full path with slashes replaced to avoid collisions.
func createBackup(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	backupDir := filepath.Join(configDir(), "backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	// Encode the full path as a filename: replace path separators.
	encoded := strings.ReplaceAll(absPath, string(filepath.Separator), "%")
	if encoded[0] == '%' {
		encoded = encoded[1:]
	}
	backupPath := filepath.Join(backupDir, encoded)

	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

// isReadonly returns true if the file exists and is not writable.
func isReadonly(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		// File doesn't exist yet — not readonly.
		if os.IsNotExist(err) {
			return false
		}
		// Permission denied or other error — treat as readonly.
		return true
	}
	f.Close()
	return false
}

// fileExists returns true if the path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// saveWithSudo writes the buffer via sudo for files the user can't write
// directly. It writes to a temp file, suspends the screen (so sudo can
// prompt for a password), then copies the temp file to the target path.
func (e *Editor) saveWithSudo(b *Buffer, path string) error {
	sudoCmd := findSudoCmd()
	if sudoCmd == "" {
		return fmt.Errorf("neither sudo nor doas found")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	// Write buffer to a temp file.
	tmp, err := os.CreateTemp("", "mu-save-*")
	if err != nil {
		return fmt.Errorf("sudo save: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	bw := bufio.NewWriter(tmp)
	if _, err := b.text.WriteTo(bw); err != nil {
		tmp.Close()
		return fmt.Errorf("sudo save: write temp: %w", err)
	}
	bw.Flush()
	tmp.Close()

	// Preserve original file permissions if the file exists.
	if fi, err := os.Stat(absPath); err == nil {
		os.Chmod(tmpPath, fi.Mode())
	}

	// Catch SIGINT so Ctrl-C during sudo kills only the subprocess.
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt)

	// Suspend the screen so sudo can use the terminal for password input.
	if e.screen != nil {
		e.screen.Suspend()
	}

	cmd := exec.Command(sudoCmd, "cp", tmpPath, absPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()

	// Stop intercepting signals and resume the screen.
	signal.Stop(sigch)
	if e.screen != nil {
		e.screen.Resume()
	}

	if err != nil {
		return fmt.Errorf("sudo save: %w", err)
	}

	b.Path = path
	b.markUnmodified()
	return nil
}

// findSudoCmd returns "sudo" or "doas", whichever is available.
func findSudoCmd() string {
	if _, err := exec.LookPath("sudo"); err == nil {
		return "sudo"
	}
	if _, err := exec.LookPath("doas"); err == nil {
		return "doas"
	}
	return ""
}
