package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/8op-org/gl1tch/internal/assistant"
	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/daemonwidget"
	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/keybindings"
	"github.com/8op-org/gl1tch/internal/layout"
	"github.com/8op-org/gl1tch/internal/supervisor"
	suphandlers "github.com/8op-org/gl1tch/internal/supervisor/handlers"
	"github.com/8op-org/gl1tch/internal/systemprompts"
	"github.com/8op-org/gl1tch/internal/themes"
	"github.com/8op-org/gl1tch/internal/widgetdispatch"
)

// ErrReload is returned by Run when a reload was requested (marker file present).
// Callers should re-invoke Run to start a fresh session with the updated binary.
var ErrReload = errors.New("reload requested")

const (
	SessionName  = "glitch"
	configSubdir = ".config/glitch"
)

// tmuxPalette holds the hex color strings used for tmux status bar styling.
type tmuxPalette struct {
	accent string
	bg     string
	dim    string
	border string
}

// loadTmuxPalette reads the persisted active theme and extracts the colors
// needed for the tmux status bar. Falls back to Nord defaults.
func loadTmuxPalette() tmuxPalette {
	p := tmuxPalette{
		accent: "#88c0d0",
		bg:     "#2e3440",
		dim:    "#4c566a",
		border: "#3b4252",
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	userThemesDir := filepath.Join(home, configSubdir, "themes")
	reg, err := themes.NewRegistry(userThemesDir)
	if err != nil {
		return p
	}
	b := reg.Active()
	if b == nil {
		return p
	}
	if v := b.Palette.Accent; v != "" {
		p.accent = v
	}
	if v := b.Palette.BG; v != "" {
		p.bg = v
	}
	if v := b.Palette.Dim; v != "" {
		p.dim = v
	}
	if v := b.Palette.Border; v != "" {
		p.border = v
	}
	return p
}

func buildTmuxConf(self string) string {
	pal := loadTmuxPalette()
	// Base tmux settings.
	base := "set -g status-position bottom\n" +
		fmt.Sprintf("set -g status-style \"fg=%s,bg=default\"\n", pal.accent) +
		"set -g status-left \"\"\n" +
		"set -g status-left-length 0\n" +
		"set -g status-right \"\"\n" +
		"set -g status-right-length 0\n" +
		"set -g status-justify centre\n" +
		"set -g window-status-format \"\"\n" +
		"set -g window-status-current-format \"" + themes.TmuxStatusCenterFormat(pal.accent, pal.dim) + "\"\n" +
		"set -g mouse on\n" +
		"set -g default-terminal \"screen-256color\"\n" +
		"set -g base-index 0\n" +
		fmt.Sprintf("set -g pane-border-style \"fg=%s\"\n", pal.border) +
		fmt.Sprintf("set -g pane-active-border-style \"fg=%s\"\n", pal.accent)

	// ctrl+space enters the glitch-chord key table.
	// Press ctrl+space again to open the help popup; press a chord key to act directly.
	leaderBinding := "bind-key -n C-Space switch-client -T glitch-chord\n"

	// Chord bindings inside the glitch-chord key table.
	chords := "bind-key -T glitch-chord d     { switch-client -T root ; detach-client }\n" +
		"bind-key -T glitch-chord r     { switch-client -T root ; run-shell \"" + self + " _reload\" }\n" +
		"bind-key -T glitch-chord s     { switch-client -T root ; display-popup -E -w 44 -h 6 \"" + self + " _opsx\" }\n" +
		"unbind-key -T glitch-chord j\n" +
		// Window management
		"bind-key -T glitch-chord c     { switch-client -T root ; new-window }\n" +
		"bind-key -T glitch-chord [     { switch-client -T root ; previous-window }\n" +
		"bind-key -T glitch-chord ]     { switch-client -T root ; next-window }\n" +
		// Pane splitting
		"bind-key -T glitch-chord |     { switch-client -T root ; split-window -h }\n" +
		"bind-key -T glitch-chord -     { switch-client -T root ; split-window -v }\n" +
		// Pane navigation
		"bind-key -T glitch-chord Left  { switch-client -T root ; select-pane -L }\n" +
		"bind-key -T glitch-chord Right { switch-client -T root ; select-pane -R }\n" +
		"bind-key -T glitch-chord Up    { switch-client -T root ; select-pane -U }\n" +
		"bind-key -T glitch-chord Down  { switch-client -T root ; select-pane -D }\n" +
		// Kill pane / window
		"bind-key -T glitch-chord x     { switch-client -T root ; if -F \"#{==:#{window_index},0}\" { display-message \"Cannot kill the GL1TCH window (window 0)\" } { kill-pane } }\n" +
		"bind-key -T glitch-chord X     { switch-client -T root ; if -F \"#{==:#{window_index},0}\" { display-message \"Cannot kill the GL1TCH window (window 0)\" } { kill-window } }\n" +
		"bind-key -T glitch-chord Escape switch-client -T root\n" +
		// Pressing ctrl+space again exits the chord table without action.
		"bind-key -T glitch-chord C-Space switch-client -T root\n" +
		// GL1TCH AI assistant — focus panel in deck (window 0), send A key.
		"bind-key -T glitch-chord a     { switch-client -T root ; switch-client -t glitch ; select-window -t glitch:0 ; send-keys -t glitch:0 A }\n" +
		// Explicitly unbind removed chords so stale sessions don't keep them.
		"unbind-key -T glitch-chord n\n" +
		"unbind-key -T glitch-chord m\n" +
		"bind-key -T glitch-chord o     { switch-client -T root ; select-pane -t :.+ }\n" +
		"unbind-key -T glitch-chord t\n" +
		"unbind-key -T glitch-chord h\n" +
		"unbind-key -T glitch-chord q\n"

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

// applyKeybindings loads keybindings.yaml from cfgDir and applies it.
// Missing file is silently ignored. Errors are logged as warnings.
func applyKeybindings(cfgDir string) {
	kbCfg, err := keybindings.LoadConfig(filepath.Join(cfgDir, "keybindings.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "glitch: warning: load keybindings config: %v\n", err)
	} else if len(kbCfg.Bindings) > 0 {
		if err := keybindings.Apply(kbCfg); err != nil {
			fmt.Fprintf(os.Stderr, "glitch: warning: apply keybindings: %v\n", err)
		}
	}
}

// applyConfigs loads layout.yaml and keybindings.yaml from cfgDir and applies
// them. Missing files are silently ignored (no-op). Errors are logged as
// warnings but do not abort startup. Only call this on re-attach — fresh
// sessions use the hardcoded default layout in Run().
func applyConfigs(cfgDir string) {
	layoutCfg, err := layout.LoadConfig(filepath.Join(cfgDir, "layout.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "glitch: warning: load layout config: %v\n", err)
	} else if len(layoutCfg.Panes) > 0 {
		d := widgetdispatch.DefaultDispatcher{}
		if err := layout.Apply(context.Background(), layoutCfg, d); err != nil {
			fmt.Fprintf(os.Stderr, "glitch: warning: apply layout: %v\n", err)
		}
	}
	applyKeybindings(cfgDir)
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

	// Ensure user plugin subdirectories exist on first run.
	// Note: plugins/ and pipelines/ are created on-demand by discovery.
	for _, sub := range []string{"providers", "widgets", "themes"} {
		if err := os.MkdirAll(filepath.Join(cfgDir, sub), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "glitch: warning: could not create %s dir: %v\n", sub, err)
		}
	}

	// Install system prompt defaults to ~/.config/glitch/prompts/ on first run.
	// Existing files are never overwritten, so user customizations are preserved.
	if err := systemprompts.EnsureInstalled(cfgDir); err != nil {
		fmt.Fprintf(os.Stderr, "glitch: warning: install system prompts: %v\n", err)
	}

	// Fast path: session already running (e.g. after detach) — just reattach.
	if SessionExists(SessionName) {
		applyConfigs(cfgDir)
		cmd := exec.Command("tmux", "-f", confPath, "attach-session", "-t", SessionName)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("attaching to session: %w", err)
		}
		return checkReload()
	}

	// New session: start the Unix socket event bus daemon BEFORE any widget
	// binaries are launched so they can connect on startup.
	busdSrv := busd.New()
	if err := busdSrv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "glitch: warning: could not start busd: %v\n", err)
	} else {
		defer busdSrv.Stop()
	}

	// Start any installed plugins that declare daemon:true in their sidecar YAML.
	// BUSD is already listening so daemons can connect immediately.
	daemons := daemonwidget.StartAll(filepath.Join(cfgDir, "wrappers"))
	defer daemons.Stop()

	// Start the reactive supervisor. It is non-critical — failures are logged
	// but never crash the bootstrap process.
	{
		supCtx, supCancel := context.WithCancel(context.Background())
		defer supCancel()

		execMgr := executor.NewManager()
		_ = execMgr.LoadWrappersFromDir(filepath.Join(cfgDir, "wrappers"))

		sup := supervisor.New(cfgDir, execMgr)

		// Build a bus publisher the handlers can use.
		sockPath, _ := busd.SocketPath()
		pub := suphandlers.NewBusPublisher(sockPath)

		// Register the diagnosis handler (reacts to pipeline/agent failure events).
		sup.RegisterHandler(suphandlers.NewDiagnosisHandler(execMgr, pub))

		// Register agent loop handlers for any sidecar with agent_loop: true.
		wrappersDir := filepath.Join(cfgDir, "wrappers")
		for _, alCfg := range suphandlers.ScanAgentLoopSidecars(wrappersDir) {
			sup.RegisterHandler(suphandlers.NewAgentLoopHandler(alCfg, execMgr, pub))
		}

		go func() {
			if err := sup.Start(supCtx); err != nil {
				slog.Warn("supervisor exited", "err", err)
			}
		}()
		defer sup.Stop()
	}

	run := func(args ...string) error {
		c := exec.Command("tmux", args...)
		c.Stderr = os.Stderr
		return c.Run()
	}

	// Create session running the deck directly in the GLITCH window.
	if err := run("-f", confPath, "new-session", "-d", "-s", SessionName, "-n", "GL1TCH", self); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	run("source-file", confPath) //nolint:errcheck
	// Only apply keybindings on fresh session; layout.yaml is for re-attach customisation.
	applyKeybindings(cfgDir)

	// Kill any stale glitch-cron session so the fresh binary is always used,
	// then start a new glitch-cron session alongside the console.
	exec.Command("tmux", "kill-session", "-t", "glitch-cron").Run() //nolint:errcheck
	if err := exec.Command("tmux", "new-session", "-d", "-s", "glitch-cron",
		"-x", "220", "-y", "50", self+" cron tui").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "glitch: warning: could not start cron session: %v\n", err)
	} else {
		exec.Command("tmux", "set-window-option", "-t", "glitch-cron:0", //nolint:errcheck
			"@glitch-label", "glitch-cron").Run()
	}

	// First-run: open the GL1TCH assistant TUI in a new window before attaching.
	if assistant.IsFirstRun(cfgDir) {
		exec.Command("tmux", "new-window", "-t", SessionName+":", "-n", "glitch-assistant", //nolint:errcheck
			self+" assistant").Run()
	}

	cmd := exec.Command("tmux", "-f", confPath, "attach-session", "-t", SessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("attaching to session: %w", err)
	}
	return checkReload()
}
