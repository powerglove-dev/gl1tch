package pluginmanager

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runCommand runs an external command and returns combined stdout+stderr output.
func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// resolveBinaryPath returns the full path to a binary after `go install`.
// It checks GOBIN, then GOPATH/bin, then the system PATH.
func resolveBinaryPath(binary string) (string, error) {
	// Check GOBIN first.
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		p := filepath.Join(gobin, binary)
		if fileExists(p) {
			return p, nil
		}
	}

	// Check GOPATH/bin.
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}
	p := filepath.Join(gopath, "bin", binary)
	if fileExists(p) {
		return p, nil
	}

	// Fall back to PATH lookup.
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("binary %q not found in GOBIN, GOPATH/bin, or PATH", binary)
	}
	return path, nil
}

// localBinDir returns ~/.local/bin, creating it if necessary.
func localBinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create ~/.local/bin: %w", err)
	}
	// Warn if ~/.local/bin is not on PATH (non-fatal).
	path := os.Getenv("PATH")
	if !strings.Contains(path, dir) {
		fmt.Fprintf(os.Stderr, "warning: %s is not in PATH; add it to use installed plugins\n", dir)
	}
	return dir, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
