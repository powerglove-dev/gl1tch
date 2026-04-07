package handlers

import (
	"log/slog"

	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/supervisor"
)

// RegisterCollectors reads the observer config and registers each enabled
// collector as an independent supervisor service.
func RegisterCollectors(sup *supervisor.Supervisor) {
	cfg, err := collector.LoadConfig()
	if err != nil {
		slog.Warn("collectors: config error", "err", err)
		return
	}

	if cfg.Claude.Enabled {
		sup.RegisterService(NewCollectorService(&collector.ClaudeCollector{
			Interval: cfg.Claude.Interval,
		}))
		sup.RegisterService(NewCollectorService(&collector.ClaudeProjectCollector{
			Interval: cfg.Claude.Interval,
		}))
	}

	if cfg.Copilot.Enabled {
		sup.RegisterService(NewCollectorService(&collector.CopilotCollector{
			Interval: cfg.Copilot.Interval,
		}))
	}

	// Headless `glitch serve` path: register one unified workspace
	// collector for the directories listed in observer.yaml. The
	// per-workspace pod manager owns the multi-workspace path; this
	// branch only fires for users running gl1tch as a background
	// daemon without the desktop app, where there's no concept of
	// per-workspace dirs and the global YAML is the only source of
	// truth.
	if len(cfg.Directories.Paths) > 0 {
		sup.RegisterService(NewCollectorService(&collector.WorkspaceCollector{
			Dirs:     cfg.Directories.Paths,
			Interval: cfg.Directories.Interval,
		}))
	}

	// PipelineIndexer is registered without a store — it opens its own.
	sup.RegisterService(NewCollectorService(&collector.PipelineIndexer{}))
}
