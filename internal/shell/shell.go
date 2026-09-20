// Package shell validates and re-sources alias files.
package shell

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Check parses path with the shell's syntax checker without executing it.
func Check(shellName, path string) error {
	bin, err := resolve(shellName)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-n", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("syntax error: %s", msg)
	}
	return nil
}

// Source sources path in a throwaway subshell. It cannot change the calling
// shell, but it proves the file loads cleanly and reports what it would fail
// on. The `am` wrapper function is what actually reloads the live shell.
func Source(shellName, path string) error {
	bin, err := resolve(shellName)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-c", fmt.Sprintf("source %s", quote(path)))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("source failed: %s", msg)
	}
	return nil
}

func resolve(shellName string) (string, error) {
	if shellName == "" {
		shellName = "bash"
	}
	bin, err := exec.LookPath(shellName)
	if err != nil {
		return "", fmt.Errorf("%s not found on PATH", shellName)
	}
	return bin, nil
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Wrapper returns the shell function the user should add to their rc file so
// that saves take effect in the current shell immediately.
func Wrapper(binary, sourceFile string) string {
	return fmt.Sprintf(`am() {
  command %s "$@"
  [ -f %s ] && . %s
}`, binary, quote(sourceFile), quote(sourceFile))
}
