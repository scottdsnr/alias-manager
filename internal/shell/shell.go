// Package shell validates and re-sources alias files.
package shell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// WriteUnaliases records the alias names that must be removed from the live
// shell before the alias file is re-sourced. Sourcing alone can only add or
// overwrite aliases, so renames, deletes and disables would otherwise leave
// the old definition active until a new shell is started. The wrapper sources
// this file first, then the alias file, then clears it.
func WriteUnaliases(path string, names []string) error {
	return WriteCleanup(path, names, nil)
}

// WriteCleanup is WriteUnaliases plus the function names that must be unset,
// for functions that were renamed, deleted or disabled.
func WriteCleanup(path string, aliases, funcs []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, n := range aliases {
		if strings.TrimSpace(n) == "" {
			continue
		}
		fmt.Fprintf(&b, "unalias -- %s 2>/dev/null\n", quote(n))
	}
	for _, n := range funcs {
		if strings.TrimSpace(n) == "" {
			continue
		}
		fmt.Fprintf(&b, "unset -f %s 2>/dev/null\n", quote(n))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// Wrapper returns the shell function the user should add to their rc file so
// that saves take effect in the current shell immediately.
func Wrapper(binary, sourceFile, unaliasFile string) string {
	return fmt.Sprintf(`am() {
  command %s "$@"
  if [ -s %s ]; then . %s; : > %s; fi
  if [ -f %s ]; then . %s; fi
}`, binary, quote(unaliasFile), quote(unaliasFile), quote(unaliasFile), quote(sourceFile), quote(sourceFile))
}
