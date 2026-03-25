package bootstrap

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/adam-stokes/orcai/internal/bus"
	"github.com/adam-stokes/orcai/internal/discovery"
	"github.com/adam-stokes/orcai/internal/host"
)

// ErrReload is returned by Run when a reload was requested (marker file present).
// Callers should re-invoke Run to start a fresh session with the updated binary.
var ErrReload = errors.New("reload requested")

const (
	SessionName  = "orcai"
	configSubdir = ".config/orcai"
)

func buildTmuxConf(self string) string {
	// Base tmux settings.
	base := `set -g status-position bottom
set -g status-style "fg=#bd93f9,bg=#282a36"
set -g window-status-format ""
set -g window-status-current-format ""
set -g status-left "#[fg=#bd93f9,bold] ORCAI #[default]"
set -g status-left-length 20
set -g status-right "#[fg=#6272a4] ^spc n new  ^spc t panel  ^spc p build   %H:%M "
set -g status-right-length 56
set -g mouse on
set -g default-terminal "screen-256color"
set -g base-index 0
set -g pane-border-style "fg=#44475a"
set -g pane-active-border-style "fg=#bd93f9"
`
	// ctrl+space enters the orcai-chord key table.
	// Press ctrl+space again to open the help popup; press a chord key to act directly.
	leaderBinding := "bind-key -n C-Space switch-client -T orcai-chord\n"

	// Chord bindings inside the orcai-chord key table.
	chords := "bind-key -T orcai-chord q     { switch-client -T root ; display-popup -E -w 44 -h 18 \"" + self + " _help quit\" }\n" +
		"bind-key -T orcai-chord d     { switch-client -T root ; display-popup -E -w 44 -h 18 \"" + self + " _help detach\" }\n" +
		"bind-key -T orcai-chord r     { switch-client -T root ; display-popup -E -w 44 -h 18 \"" + self + " _help reload\" }\n" +
		"bind-key -T orcai-chord n     { switch-client -T root ; display-popup -E -w 120 -h 40 \"" + self + " _picker\" }\n" +
		"bind-key -T orcai-chord o     { switch-client -T root ; display-popup -E -w 68 -h 24 \"" + self + " ollama\" }\n" +
		"bind-key -T orcai-chord s     { switch-client -T root ; display-popup -E -w 44 -h 6 \"" + self + " _opsx\" }\n" +
		"bind-key -T orcai-chord p     { switch-client -T root ; new-window -t orcai -n prompt-builder \"" + self + " _promptbuilder\" }\n" +
		"bind-key -T orcai-chord t     { switch-client -T root ; run-shell \"" + self + " _sidebar-toggle\" }\n" +
		"bind-key -T orcai-chord Escape switch-client -T root\n" +
		// Pressing ctrl+space again while in chord table shows help immediately.
		"bind-key -T orcai-chord C-Space { switch-client -T root ; display-popup -E -w 44 -h 18 \"" + self + " _help\" }\n"

	return base + leaderBinding + chords
}

// reloadMarkerPath returns the path to the reload marker file.
func reloadMarkerPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configSubdir, ".reload"), nil
}

// WriteReloadMarker creates the reload marker file so that the next
// bootstrap.Run() call returns ErrReload instead of exiting normally.
func WriteReloadMarker() error {
	path, err := reloadMarkerPath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, nil, 0o644)
}

// checkReload removes the marker file if present and returns ErrReload.
func checkReload() error {
	path, err := reloadMarkerPath()
	if err != nil {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	os.Remove(path) //nolint:errcheck
	return ErrReload
}

// HasTmux reports whether tmux is available in PATH.
func HasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// SessionExists returns true if a tmux session named sessionName is running.
func SessionExists(sessionName string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

// WriteTmuxConf writes tmux.conf to cfgDir and returns the path.
func WriteTmuxConf(cfgDir, self string) (string, error) {
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}
	confPath := filepath.Join(cfgDir, "tmux.conf")
	if err := os.WriteFile(confPath, []byte(buildTmuxConf(self)), 0o644); err != nil {
		return "", fmt.Errorf("writing tmux.conf: %w", err)
	}
	return confPath, nil
}

// Run is the main entrypoint: reconnect to an existing session or create a new one.
func Run() error {
	if !HasTmux() {
		return fmt.Errorf("tmux not found in PATH\nInstall with: brew install tmux")
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home dir: %w", err)
	}
	cfgDir := filepath.Join(home, configSubdir)

	confPath, err := WriteTmuxConf(cfgDir, self)
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Fast path: session already running (e.g. after detach) — just reattach.
	if SessionExists(SessionName) {
		cmd := exec.Command("tmux", "-f", confPath, "attach-session", "-t", SessionName)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("attaching to session: %w", err)
		}
		return checkReload()
	}

	// New session: start the event bus and load plugins.
	busSrv := bus.New()
	busAddr, err := busSrv.Listen("127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting event bus: %w", err)
	}
	defer busSrv.Stop()

	// Persist bus address so sidebar and other out-of-process components can connect.
	if err := os.WriteFile(filepath.Join(cfgDir, "bus.addr"), []byte(busAddr), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "orcai: warning: could not write bus.addr: %v\n", err)
	}

	plugins, err := discovery.Discover(cfgDir)
	if err != nil {
		return fmt.Errorf("discovering plugins: %w", err)
	}
	h := host.New(busAddr)
	defer h.StopAll()
	for _, p := range plugins {
		if err := h.Load(p); err != nil {
			fmt.Fprintf(os.Stderr, "orcai: failed to load plugin %q: %v\n", p.Name, err)
		}
	}

	run := func(args ...string) error {
		c := exec.Command("tmux", args...)
		c.Stderr = os.Stderr
		return c.Run()
	}

	if err := run("-f", confPath, "new-session", "-d", "-s", SessionName, "-n", "ORCAI",
		self, "_welcome"); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	run("source-file", confPath) //nolint:errcheck
	// Sidebar is hidden by default; use ctrl+; t to toggle it.

	cmd := exec.Command("tmux", "-f", confPath, "attach-session", "-t", SessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("attaching to session: %w", err)
	}
	return checkReload()
}
