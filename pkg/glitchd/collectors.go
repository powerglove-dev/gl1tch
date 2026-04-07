package glitchd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/collector"
)

// CollectorRun is a snapshot of one collector's most recent run cycle.
// Mirrors collector.RunReport but exposes a wire-friendly shape (ms
// timestamps, error string) for the desktop frontend.
type CollectorRun struct {
	Name         string `json:"name"`
	IndexedCount int    `json:"indexed_count"`
	DurationMs   int64  `json:"duration_ms"`
	AtMs         int64  `json:"at_ms"`
	Error        string `json:"error,omitempty"`
}

// CollectorRuns returns the latest run heartbeat for each collector,
// keyed by collector name. Sourced from the in-process registry that
// every collector calls into after each poll cycle. Used by the
// desktop's brain popover to show "git ran 12s ago, indexed 3" even
// when ES has nothing new for that source.
func CollectorRuns() map[string]CollectorRun {
	snap := collector.Runs.Snapshot()
	out := make(map[string]CollectorRun, len(snap))
	for name, r := range snap {
		var errStr string
		if r.Err != nil {
			errStr = r.Err.Error()
		}
		out[name] = CollectorRun{
			Name:         name,
			IndexedCount: r.IndexedCount,
			DurationMs:   r.Duration.Milliseconds(),
			AtMs:         r.At.UnixMilli(),
			Error:        errStr,
		}
	}
	return out
}

// suppress "imported and not used" if Go ever flags time when the
// helper above is the only consumer.
var _ = time.Now

// CollectorInfo describes one configured collector for the brain
// popover. All collectors are now workspace-bound: the popover asks
// for the active workspace's view and gets a single flat list of
// what's being watched for that workspace. Directories come from the
// workspace row in SQLite; git and github are auto-derived from the
// workspace's directories. Chat sources (claude, copilot) are
// process-wide but shown under every workspace because the brain's
// memory doesn't silo chat context by project.
type CollectorInfo struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	IntervalMs int64  `json:"interval_ms"`
	// Detail is a short, human-readable description of what's being
	// watched (repo count, channel list, …).
	Detail string `json:"detail"`
	// Source is a free-form pointer the frontend can show on hover
	// (config path, repo list, …).
	Source string `json:"source,omitempty"`
}

// ListCollectorsForWorkspace returns the flat set of collectors
// scoped to a single workspace. Directories come from the workspace
// row in SQLite, and git/github/claude/copilot intervals + enablement
// come from that workspace's per-workspace collectors.yaml.
//
// workspaceID == "" falls back to the global observer.yaml view so
// the function is still usable before the desktop picks an active
// workspace (e.g. during startup).
//
// Every collector row is always included even when disabled so the
// popover can show "you have 0 dirs, add one to start watching git"
// on a fresh workspace instead of a blank list.
func ListCollectorsForWorkspace(ctx context.Context, workspaceID string) ([]CollectorInfo, error) {
	// Load the per-workspace config when a workspace is specified;
	// otherwise the global one. This is what makes "configure
	// collectors" actually edit per-workspace files now — the
	// brain popover and the editor see the same source of truth.
	var cfg *collector.Config
	var err error
	if workspaceID != "" {
		cfg, err = collector.LoadWorkspaceConfig(workspaceID)
	} else {
		cfg, err = collector.LoadConfig()
	}
	if err != nil {
		return nil, err
	}

	// Resolve the workspace's directories. Fall back to the loaded
	// config's directories.paths when no workspace is specified.
	var dirs []string
	if workspaceID != "" {
		st, err := OpenStore()
		if err == nil {
			if ws, err := st.GetWorkspace(ctx, workspaceID); err == nil {
				dirs = append(dirs, ws.Directories...)
			}
		}
	}
	if len(dirs) == 0 {
		dirs = append(dirs, cfg.Directories.Paths...)
	}

	// Apply the same do-what-I-mean overlay the pod manager uses
	// when actually running collectors. This guarantees the brain
	// popover and the running pod show the same state — without it
	// the popover could promise "git: 2 repos" while the pod was
	// actually running zero git collectors because the workspace
	// config didn't list them.
	collector.AutoDetectFromWorkspace(cfg, dirs)

	gitRepos := append([]string{}, cfg.Git.Repos...)
	githubRepos := append([]string{}, cfg.GitHub.Repos...)

	var out []CollectorInfo

	// Directories — the workspace's filesystem scanner. The
	// directories collector picks up these paths from observer.yaml
	// on its next tick (see internal/collector/directory.go).
	out = append(out, CollectorInfo{
		Name:       "directories",
		Enabled:    len(dirs) > 0,
		IntervalMs: cfg.Directories.Interval.Milliseconds(),
		Detail:     fmt.Sprintf("%d path(s)", len(dirs)),
		Source:     joinShort(dirs),
	})

	out = append(out, CollectorInfo{
		Name:       "git",
		Enabled:    len(gitRepos) > 0,
		IntervalMs: cfg.Git.Interval.Milliseconds(),
		Detail:     fmt.Sprintf("%d repo(s)", len(gitRepos)),
		Source:     joinShort(gitRepos),
	})

	out = append(out, CollectorInfo{
		Name:       "github",
		Enabled:    len(githubRepos) > 0,
		IntervalMs: cfg.GitHub.Interval.Milliseconds(),
		Detail:     fmt.Sprintf("%d repo(s)", len(githubRepos)),
		Source:     joinShort(githubRepos),
	})

	// Chat sources — claude / copilot. These are tied to the user's
	// machine rather than any one workspace, but we surface them
	// here because the brain's memory for any given workspace
	// includes "what the user was discussing in claude when they
	// were in this project". Enabled state comes from observer.yaml.

	out = append(out, CollectorInfo{
		Name:       "claude",
		Enabled:    cfg.Claude.Enabled,
		IntervalMs: cfg.Claude.Interval.Milliseconds(),
		Detail:     "claude code sessions",
	})

	out = append(out, CollectorInfo{
		Name:       "copilot",
		Enabled:    cfg.Copilot.Enabled,
		IntervalMs: cfg.Copilot.Interval.Milliseconds(),
		Detail:     "copilot CLI history",
	})

	// Code-index — semantic embedding pass over the workspace's
	// source files. Off by default because the first pass against a
	// large tree can take minutes; the user opts in via the modal,
	// after which AutoDetectFromWorkspace fills in the paths from
	// the workspace's directories so they don't have to type them.
	//
	// Detail line summarizes the embedder so users immediately know
	// "ollama nomic-embed-text" vs "voyage voyage-code-3" without
	// opening the modal.
	ciDetail := fmt.Sprintf("%d path(s)", len(cfg.CodeIndex.Paths))
	if cfg.CodeIndex.Enabled {
		provider := cfg.CodeIndex.EmbedProvider
		if provider == "" {
			provider = "ollama"
		}
		model := cfg.CodeIndex.EmbedModel
		if model == "" {
			switch provider {
			case "ollama":
				model = "nomic-embed-text"
			case "openai":
				model = "text-embedding-3-small"
			case "voyage":
				model = "voyage-code-3"
			}
		}
		ciDetail = fmt.Sprintf("%d path(s) · %s %s",
			len(cfg.CodeIndex.Paths), provider, model)
	}
	out = append(out, CollectorInfo{
		Name:       "code-index",
		Enabled:    cfg.CodeIndex.Enabled && len(cfg.CodeIndex.Paths) > 0,
		IntervalMs: cfg.CodeIndex.Interval.Milliseconds(),
		Detail:     ciDetail,
		Source:     joinShort(cfg.CodeIndex.Paths),
	})

	return out, nil
}

// ListCollectors is the legacy observer.yaml-only entry point, kept
// for callers that don't have a workspace ID handy. New code should
// use ListCollectorsForWorkspace.
func ListCollectors() ([]CollectorInfo, error) {
	return ListCollectorsForWorkspace(context.Background(), "")
}

// joinShort returns up to 3 entries joined by ", " followed by an
// ellipsis-style suffix if there are more.
func joinShort(items []string) string {
	if len(items) == 0 {
		return ""
	}
	const max = 3
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(" +%d more", len(items)-max)
}
