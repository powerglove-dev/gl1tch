package handlers

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// NewNotifyService creates a ProcessService for the gl1tch-notify macOS app.
// Returns nil on non-Darwin platforms.
func NewNotifyService() *ProcessService {
	if runtime.GOOS != "darwin" {
		return nil
	}

	home, _ := os.UserHomeDir()
	appBundle := filepath.Join(home, ".local", "Applications", "glitch-notify.app")
	binary := filepath.Join(appBundle, "Contents", "MacOS", "glitch-notify")

	return &ProcessService{
		ServiceName: "notify",
		Command:     binary,
		Display:     "systray",
		BuildFn: func() error {
			return buildNotifyIfNeeded(binary)
		},
	}
}

// buildNotifyIfNeeded builds the Swift notify plugin if the binary doesn't exist.
func buildNotifyIfNeeded(binary string) error {
	if _, err := os.Stat(binary); err == nil {
		return nil // already built
	}

	// Find the plugins/notify source directory relative to the glitch binary.
	srcDir := findNotifySource()
	if srcDir == "" {
		slog.Info("notify: source not found, skipping build")
		return nil
	}

	slog.Info("notify: building from source", "dir", srcDir)
	cmd := exec.Command("make", "install")
	cmd.Dir = srcDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findNotifySource locates the plugins/notify directory.
func findNotifySource() string {
	// Check relative to the running binary.
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(filepath.Dir(exe)) // go up from bin/
		candidate := filepath.Join(dir, "plugins", "notify")
		if _, err := os.Stat(filepath.Join(candidate, "Package.swift")); err == nil {
			return candidate
		}
	}

	// Check common development locations.
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, "Projects", "gl1tch", "plugins", "notify"),
	} {
		if _, err := os.Stat(filepath.Join(p, "Package.swift")); err == nil {
			return p
		}
	}

	return ""
}
