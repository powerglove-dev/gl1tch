package bootstrap

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
)

// startCapabilityRunner builds the unified capability registry, registers
// every Go-implemented capability that has been ported, loads any user-authored
// skill markdown files from ~/.config/glitch/capabilities/, and starts the
// runner. The hook bridges to capability.RecordRun so the brain popover keeps
// showing per-source heartbeats during the migration period when some
// collectors live on the old code path and others on the new one.
//
// Returns the started runner so the caller can Stop() it on shutdown. The
// caller is also responsible for ensuring this is only invoked when the
// global collector path is active (i.e. when the workspace pod manager is
// not handling claude per-workspace) — otherwise both paths will index the
// same Claude history file and produce duplicates.
func startCapabilityRunner(ctx context.Context, cfgDir string, esAddr string) (*capability.Runner, error) {
	es, err := esearch.New(esAddr)
	if err != nil {
		return nil, err
	}

	reg := capability.NewRegistry()

	cfg, _ := capability.LoadConfig()

	// Built-in: every Go-implemented capability that has been ported
	// from internal/collector/. Each replaces the corresponding legacy
	// collector in suphandlers.RegisterCollectors. Empty WorkspaceID/Dirs
	// matches the legacy global behaviour ("index everything").
	builtins := []capability.Capability{
		&capability.ClaudeHistoryCapability{},
		&capability.ClaudeProjectsCapability{},
		&capability.CopilotCapability{},
		&capability.PipelineRunsCapability{},
	}
	if cfg != nil {
		// Honour observer.yaml enable flags so users who explicitly
		// disabled a source in the legacy config still see that source
		// disabled in the new runner. The pipeline indexer has no enable
		// flag and is always on, matching the legacy collector_registry.
		filtered := builtins[:0]
		for _, c := range builtins {
			switch c.Manifest().Name {
			case "claude", "claude-projects":
				if cfg.Claude.Enabled {
					filtered = append(filtered, c)
				}
			case "copilot":
				if cfg.Copilot.Enabled {
					filtered = append(filtered, c)
				}
			default:
				filtered = append(filtered, c)
			}
		}
		builtins = filtered
	}
	for _, c := range builtins {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}

	// User-authored skill capabilities live alongside other glitch config.
	// A missing directory is fine — the user just hasn't authored any yet.
	skillDir := filepath.Join(cfgDir, "capabilities")
	if caps, errs := capability.LoadSkillsFromDir(skillDir); len(caps) > 0 || len(errs) > 0 {
		for _, c := range caps {
			if err := reg.Register(c); err != nil {
				slog.Warn("capability: register skill failed", "err", err)
			}
		}
		for _, e := range errs {
			slog.Warn("capability: load skill failed", "err", e)
		}
		slog.Info("capability: loaded skills", "dir", skillDir, "count", len(caps), "errors", len(errs))
	}

	runner := capability.NewRunner(reg, es)

	// Bridge new-runner heartbeats to the existing capability.Runs registry
	// so the brain popover renders "claude ran 12s ago, indexed 3" with no
	// changes to the popover code path. Once the migration is complete and
	// capability.Runs is replaced by a capability-native heartbeat store,
	// this bridge goes away.
	runner.SetAfterInvoke(func(name string, dur time.Duration, indexed int, err error) {
		// RecordRun records (start, indexed, err); reconstruct start
		// from the supplied duration so "ran X seconds ago" stays
		// accurate against the popover's wall-clock display.
		capability.RecordRun(name, time.Now().Add(-dur), indexed, err)
	})

	runner.Start(ctx)
	slog.Info("capability: runner started", "registered", len(reg.Names()))
	return runner, nil
}
