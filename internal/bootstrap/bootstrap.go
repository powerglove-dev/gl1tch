package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/console"
	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/supervisor"
	suphandlers "github.com/8op-org/gl1tch/internal/supervisor/handlers"
	"github.com/8op-org/gl1tch/internal/systemprompts"
)

// ErrReload is returned by Run when a reload was requested (marker file present).
var ErrReload = errors.New("reload requested")

const (
	SessionName  = "glitch"
	configSubdir = ".config/glitch"
)

func reloadMarkerPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configSubdir, ".reload"), nil
}

// WriteReloadMarker creates the reload marker file.
func WriteReloadMarker() error {
	path, err := reloadMarkerPath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, nil, 0o644)
}

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

// Run is the main entrypoint: sets up config, starts background services
// under the supervisor, and runs the BubbleTea TUI.
func Run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home dir: %w", err)
	}
	cfgDir := filepath.Join(home, configSubdir)

	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	for _, sub := range []string{"providers", "widgets", "themes"} {
		if err := os.MkdirAll(filepath.Join(cfgDir, sub), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "glitch: warning: could not create %s dir: %v\n", sub, err)
		}
	}

	if err := systemprompts.EnsureInstalled(cfgDir); err != nil {
		fmt.Fprintf(os.Stderr, "glitch: warning: install system prompts: %v\n", err)
	}

	_ = collector.EnsureDefaultConfig()

	// ── BUSD event bus ─────────────────────────────────────────────────────
	busdSrv := busd.New()
	if err := busdSrv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "glitch: warning: could not start busd: %v\n", err)
	} else {
		defer busdSrv.Stop()
	}

	// ── Supervisor: single lifecycle manager for ALL background services ───
	supCtx, supCancel := context.WithCancel(context.Background())
	defer supCancel()

	execMgr := executor.NewManager()
	_ = execMgr.LoadWrappersFromDir(filepath.Join(cfgDir, "wrappers"))

	sup := supervisor.New(cfgDir, execMgr)

	// Event-driven handlers.
	sockPath, _ := busd.SocketPath()
	pub := suphandlers.NewBusPublisher(sockPath)
	sup.RegisterHandler(suphandlers.NewDiagnosisHandler(execMgr, pub))

	wrappersDir := filepath.Join(cfgDir, "wrappers")
	for _, alCfg := range suphandlers.ScanAgentLoopSidecars(wrappersDir) {
		sup.RegisterHandler(suphandlers.NewAgentLoopHandler(alCfg, execMgr, pub))
	}

	// ── Services ───────────────────────────────────────────────────────────

	// Observer (ES connection + query engine).
	obsSvc := suphandlers.NewObserverService()
	sup.RegisterService(obsSvc)

	// Individual collectors (each manages its own ES client).
	suphandlers.RegisterCollectors(sup)

	// Cron scheduler.
	sup.RegisterService(&suphandlers.CronService{})

	// gl1tch-notify (macOS only).
	if runtime.GOOS == "darwin" {
		if notifySvc := suphandlers.NewNotifyService(); notifySvc != nil {
			sup.RegisterService(notifySvc)
		}
	}

	// ── Start supervisor ───────────────────────────────────────────────────
	go func() {
		if err := sup.Start(supCtx); err != nil {
			slog.Warn("supervisor exited", "err", err)
		}
	}()
	defer sup.Stop()

	// Catch signals for clean shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		<-sigCh
		supCancel()
		os.Exit(0)
	}()

	// Give services a moment to connect to ES before the TUI starts.
	time.Sleep(500 * time.Millisecond)

	if err := checkReload(); err != nil {
		return err
	}

	// Run the TUI with the observer query engine.
	console.RunWithObserver(obsSvc.Engine)
	return nil
}
