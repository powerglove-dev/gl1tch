package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/console"
	"github.com/8op-org/gl1tch/internal/daemonwidget"
	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/keybindings"
	"github.com/8op-org/gl1tch/internal/supervisor"
	suphandlers "github.com/8op-org/gl1tch/internal/supervisor/handlers"
	"github.com/8op-org/gl1tch/internal/systemprompts"
)

// ErrReload is returned by Run when a reload was requested (marker file present).
// Callers should re-invoke Run to start a fresh session with the updated binary.
var ErrReload = errors.New("reload requested")

const (
	SessionName  = "glitch"
	configSubdir = ".config/glitch"
)

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

// Run is the main entrypoint: sets up config, starts background services,
// and runs the BubbleTea TUI directly (no tmux required).
func Run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home dir: %w", err)
	}
	cfgDir := filepath.Join(home, configSubdir)

	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	// Ensure user plugin subdirectories exist on first run.
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

	// Apply keybindings configuration.
	applyKeybindings(cfgDir)

	// Start the Unix socket event bus daemon BEFORE any widget binaries are
	// launched so they can connect on startup.
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

	// Catch SIGHUP (terminal closed) and SIGTERM so deferred daemon cleanup runs.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		<-sigCh
		daemons.Stop()
		os.Exit(0)
	}()

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

	// Check for a pending reload before starting the TUI.
	if err := checkReload(); err != nil {
		return err
	}

	// Run the TUI directly — no tmux wrapping needed.
	console.Run()
	return nil
}
