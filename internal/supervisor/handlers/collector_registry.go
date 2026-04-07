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

	if len(cfg.Git.Repos) > 0 {
		sup.RegisterService(NewCollectorService(&collector.GitCollector{
			Repos:    cfg.Git.Repos,
			Interval: cfg.Git.Interval,
		}))
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

	if len(cfg.GitHub.Repos) > 0 {
		sup.RegisterService(NewCollectorService(&collector.GitHubCollector{
			Repos:    cfg.GitHub.Repos,
			Interval: cfg.GitHub.Interval,
		}))
	}

	// Always register the directories collector — even with no paths
	// configured up front. The collector's tick body re-reads
	// observer.yaml every cycle (see currentDirs in directory.go), so
	// directories the user adds later via the desktop's "Add directory"
	// button get picked up without a restart. Without this, an empty
	// directories.paths at startup meant the collector goroutine
	// never even started, defeating the dynamic re-read.
	sup.RegisterService(NewCollectorService(&collector.DirectoryCollector{
		Dirs:     cfg.Directories.Paths,
		Interval: cfg.Directories.Interval,
	}))

	// PipelineIndexer is registered without a store — it opens its own.
	sup.RegisterService(NewCollectorService(&collector.PipelineIndexer{}))
}
