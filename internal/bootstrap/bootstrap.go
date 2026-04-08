package bootstrap

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/8op-org/gl1tch/internal/brain"
	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/supervisor"
	suphandlers "github.com/8op-org/gl1tch/internal/supervisor/handlers"
	"github.com/8op-org/gl1tch/internal/systemprompts"
)

// ErrReload is returned when a reload was requested (marker file present).
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

// LoadDotenv reads a .env file and sets any variables not already in the
// environment. Supports KEY=VALUE, KEY="VALUE", and # comments.
func LoadDotenv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}

// Options configures RunHeadless. Zero value matches the legacy
// "headless CLI" behavior so existing call sites (cmd/serve.go) keep
// working unchanged.
type Options struct {
	// SkipGlobalCollectors disables registration of the legacy
	// global-supervisor collector services. Set this when the caller
	// owns collector lifecycle through the workspace pod manager
	// (i.e. the desktop app), so the same collectors don't run twice
	// and stamp half their docs with workspace_id="" (the bug that
	// made the brain popover render TOTAL INDEXED 0 even though the
	// global path had ingested 366k events). Headless `glitch serve`
	// has no pod manager and must leave this false.
	SkipGlobalCollectors bool
}

// RunHeadless starts all background services (busd, supervisor, collectors,
// cron, notify) without the TUI. Blocks until ctx is cancelled or a signal
// is received. Used by `glitch serve` (pure-headless) and historically also
// by the desktop GUI's RunBackend wrapper.
//
// Defaults match the headless CLI: every collector is registered as a
// global supervisor service. Use RunHeadlessWithOptions to opt out of
// the global collector registration.
func RunHeadless(ctx context.Context) error {
	return RunHeadlessWithOptions(ctx, Options{})
}

// RunHeadlessWithOptions is the option-taking variant of RunHeadless.
// The desktop app (which manages collectors via the pod manager) calls
// this with SkipGlobalCollectors:true so collectors only run once,
// scoped to a workspace, with a non-empty workspace_id on every doc.
func RunHeadlessWithOptions(ctx context.Context, opts Options) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home dir: %w", err)
	}

	LoadDotenv(filepath.Join(home, configSubdir, ".env"))
	LoadDotenv(".env")
	cfgDir := filepath.Join(home, configSubdir)

	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	for _, sub := range []string{"providers", "widgets", "themes"} {
		os.MkdirAll(filepath.Join(cfgDir, sub), 0o755) //nolint:errcheck
	}

	if err := systemprompts.EnsureInstalled(cfgDir); err != nil {
		fmt.Fprintf(os.Stderr, "glitch: warning: install system prompts: %v\n", err)
	}

	_ = capability.EnsureDefaultConfig()

	// ── BUSD event bus ─────────────────────────────────────────────────────
	busdSrv := busd.New()
	if err := busdSrv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "glitch: warning: could not start busd: %v\n", err)
	} else {
		defer busdSrv.Stop()
	}

	// ── Supervisor ─────────────────────────────────────────────────────────
	supCtx, supCancel := context.WithCancel(ctx)
	defer supCancel()

	execMgr := executor.NewManager()
	_ = execMgr.LoadWrappersFromDir(filepath.Join(cfgDir, "wrappers"))

	sup := supervisor.New(cfgDir, execMgr)

	sockPath, _ := busd.SocketPath()
	pub := suphandlers.NewBusPublisher(sockPath)
	sup.RegisterHandler(suphandlers.NewDiagnosisHandler(execMgr, pub))

	wrappersDir := filepath.Join(cfgDir, "wrappers")
	for _, alCfg := range suphandlers.ScanAgentLoopSidecars(wrappersDir) {
		sup.RegisterHandler(suphandlers.NewAgentLoopHandler(alCfg, execMgr, pub))
	}

	// ── Services ───────────────────────────────────────────────────────────
	obsSvc := suphandlers.NewObserverService()
	sup.RegisterService(obsSvc)
	sup.RegisterService(&suphandlers.CronService{})

	// Capability runner — unified registry for the new-style capabilities.
	// Owns claude, claude-projects, copilot, and pipeline. The matching
	// legacy registrations in suphandlers.RegisterCollectors are gone so
	// the same source is not indexed twice. Only runs on the global path;
	// in desktop mode the workspace pod manager will get its own
	// per-workspace runner once that side of the migration lands.
	var capRunner *capability.Runner
	if !opts.SkipGlobalCollectors {
		cfg, cfgErr := capability.LoadConfig()
		esAddr := ""
		if cfgErr == nil {
			esAddr = cfg.Elasticsearch.Address
		} else {
			slog.Warn("capability: load config failed", "err", cfgErr)
		}
		r, err := startCapabilityRunner(supCtx, cfgDir, esAddr)
		if err != nil {
			slog.Warn("capability: runner start failed", "err", err)
		} else {
			capRunner = r
		}
	}

	// Brain — autonomous self-improvement loop.
	sup.RegisterService(&brain.Service{})

	// ── Start ──────────────────────────────────────────────────────────────
	go func() {
		if err := sup.Start(supCtx); err != nil {
			slog.Warn("supervisor exited", "err", err)
		}
	}()
	defer sup.Stop()
	defer func() {
		if capRunner != nil {
			capRunner.Stop()
		}
	}()

	slog.Info("glitch backend running (headless mode)")

	// Block until context cancelled or signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-ctx.Done():
	case <-sigCh:
	}

	return nil
}
