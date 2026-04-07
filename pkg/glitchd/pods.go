// pods.go is the desktop-facing wrapper around collector.PodManager.
//
// The pod manager runs the per-workspace collector goroutines for
// the Phase 4 workspace-scoped collector split. The desktop app
// creates one manager at startup, asks it to start a pod for every
// existing workspace, and then drives pod lifecycle from the
// CreateWorkspace / DeleteWorkspace / WriteCollectorsConfig paths.
//
// The wrapper exists so callers in `pkg/glitchd` and the desktop
// `glitch-desktop/app.go` don't have to import internal/collector
// directly. It also gives us a place to lazily wire the elasticsearch
// client without every call site re-constructing it.
package glitchd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/store"
	"gopkg.in/yaml.v3"
)

// errPodManagerNotInitialized is returned by the package-level
// helpers when the desktop never called InitPodManager. The error
// is logged and swallowed by the helpers themselves so a missing
// init never crashes the app — but tests or programmatic callers
// can check for it explicitly.
var errPodManagerNotInitialized = errors.New("pod manager not initialized — call InitPodManager from app startup")

var (
	podOnce      sync.Once
	podManager   *collector.PodManager
	podManagerEs *esearch.Client
)

// InitPodManager constructs the per-process PodManager singleton
// bound to the given parent context. The first call wins; subsequent
// calls are no-ops so duplicate startup paths (HMR, app restart) don't
// build a second manager and double-up collector goroutines.
//
// The parent context typically comes from the desktop app's startup
// hook. When it cancels (app shutdown) every active pod tears down
// automatically.
//
// EnsureIndices is called synchronously here BEFORE returning. This
// is load-bearing: if a pod starts collectors that index into
// glitch-events before the index has been created with the strict
// mapping from internal/esearch/mappings.go, ES auto-creates the
// index with default dynamic mappings (workspace_id: text instead
// of keyword), which silently breaks every subsequent workspace-
// scoped aggregation. We deliberately do NOT add migration code to
// repair a mis-mapped index — pre-1.0, the contract is "wipe and
// restart" and we keep the startup path simple by guaranteeing the
// fresh-index case is the only one we ever hit.
//
// Returns the manager so the caller can immediately drive it
// (e.g. StartAllWorkspacePods). On repeat calls the existing
// manager is returned unchanged.
func InitPodManager(ctx context.Context) *collector.PodManager {
	podOnce.Do(func() {
		// Build the elasticsearch client from the global config so all
		// pods share one connection pool. If ES is unreachable we
		// still create the manager — collectors fail their first poll
		// and log it, but the desktop UI keeps working and the user
		// can fix ES later via the doctor screen.
		cfg, err := collector.LoadConfig()
		if err != nil {
			slog.Warn("pod manager: load global config failed", "err", err)
			cfg = &collector.Config{}
			cfg.Elasticsearch.Address = "http://localhost:9200"
		}
		es, err := esearch.New(cfg.Elasticsearch.Address)
		if err != nil {
			slog.Warn("pod manager: esearch.New failed", "err", err, "addr", cfg.Elasticsearch.Address)
			// es will be nil; collectors that require it will log on
			// their first poll. The pod manager itself doesn't care.
		}
		// Synchronous EnsureIndices: see the doc comment on this
		// function for why this MUST happen before any pod starts.
		// Best-effort on failure — a missing ES is reported via the
		// doctor screen, and collectors will surface their own
		// "ES unavailable" log lines on first tick.
		if es != nil {
			ensureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := es.EnsureIndices(ensureCtx); err != nil {
				slog.Warn("pod manager: ensure indices failed", "err", err)
			}
			cancel()
		}
		podManagerEs = es
		podManager = collector.NewPodManager(ctx, es, nil)
		// Wire the workspace-dirs resolver so the auto-detect
		// overlay can read each workspace's directories without
		// the collector package having to import store directly.
		podManager.SetWorkspaceDirsResolver(func(workspaceID string) []string {
			st, sErr := OpenStore()
			if sErr != nil {
				return nil
			}
			ws, wErr := st.GetWorkspace(ctx, workspaceID)
			if wErr != nil {
				return nil
			}
			return ws.Directories
		})
	})
	return podManager
}

// PodManager returns the singleton, or nil if InitPodManager hasn't
// been called yet. Most callers should use the package-level
// Start/Stop/Restart helpers instead, which short-circuit gracefully
// when the manager is missing.
func PodManager() *collector.PodManager {
	return podManager
}

// StartWorkspacePod starts the collector pod for the given workspace
// id. No-op (with a warning) when the pod manager hasn't been
// initialized so headless test paths don't crash.
func StartWorkspacePod(workspaceID string) error {
	if podManager == nil {
		slog.Warn("pod manager: skip start", "workspace", workspaceID, "err", errPodManagerNotInitialized)
		return errPodManagerNotInitialized
	}
	// Make sure the workspace has a starter config so the pod has
	// something to load. EnsureWorkspaceConfig is a no-op when the
	// file already exists.
	if err := collector.EnsureWorkspaceConfig(workspaceID); err != nil {
		slog.Warn("pod manager: ensure config failed", "workspace", workspaceID, "err", err)
	}
	return podManager.StartPod(workspaceID)
}

// StopWorkspacePod stops the workspace's pod and waits for every
// collector goroutine to exit. Idempotent.
func StopWorkspacePod(workspaceID string) error {
	if podManager == nil {
		return nil
	}
	return podManager.StopPod(workspaceID)
}

// RestartWorkspacePod stops and immediately re-starts the workspace's
// pod so a freshly written collectors.yaml takes effect. Used by the
// editor popup's save action.
func RestartWorkspacePod(workspaceID string) error {
	if podManager == nil {
		return errPodManagerNotInitialized
	}
	return podManager.RestartPod(workspaceID)
}

// StartToolPod starts the global "tool collectors" pod that owns
// copilot + mattermost. These collectors read shared per-machine
// data sources and run once with workspace_id=collector.WorkspaceIDTools
// so the same source isn't re-indexed under every workspace.
//
// The brain popover OR-includes the tools bucket alongside the active
// workspace's bucket via QueryCollectorActivityScoped, so users still
// see copilot/mattermost rows in the popover with their real (single)
// counts.
//
// Best-effort: returns the pod manager error so the desktop can log
// it, but never propagates a startup failure that would block the rest
// of the app.
func StartToolPod() error {
	if podManager == nil {
		slog.Warn("pod manager: skip start tool pod", "err", errPodManagerNotInitialized)
		return errPodManagerNotInitialized
	}
	return podManager.StartToolPod()
}

// StopToolPod stops the global tool collectors pod. Idempotent.
func StopToolPod() error {
	if podManager == nil {
		return nil
	}
	return podManager.StopToolPod()
}

// WorkspaceIDTools re-exports the sentinel workspace_id for the
// global tool pod so the desktop and query helpers don't have to
// reach into internal/collector. Keep in sync with the underlying
// constant.
const WorkspaceIDTools = collector.WorkspaceIDTools

// StartAllWorkspacePods enumerates every workspace in the store and
// starts a pod for each. Called once at app startup so existing
// workspaces resume collecting without the user having to click
// anything.
//
// Errors per-workspace are logged and don't abort the loop — one
// broken workspace shouldn't prevent the others from collecting.
// Returns the number of pods that were successfully started.
func StartAllWorkspacePods(ctx context.Context) int {
	st, err := OpenStore()
	if err != nil {
		slog.Warn("pod manager: open store failed", "err", err)
		return 0
	}
	wss, err := st.ListWorkspaces(ctx)
	if err != nil {
		slog.Warn("pod manager: list workspaces failed", "err", err)
		return 0
	}
	started := 0
	for _, ws := range wss {
		if err := StartWorkspacePod(ws.ID); err != nil {
			slog.Warn("pod manager: start failed", "workspace", ws.ID, "err", err)
			continue
		}
		started++
	}
	slog.Info("pod manager: started workspace pods", "count", started)
	return started
}

// StopAllWorkspacePods tears down every active pod. Used at app
// shutdown so collector goroutines exit cleanly before the process
// dies.
func StopAllWorkspacePods() {
	if podManager == nil {
		return
	}
	podManager.StopAll()
}

// DeleteWorkspaceCollectorConfig removes a workspace's
// collectors.yaml file from disk. Called by the desktop's
// DeleteWorkspace path so deleted workspaces don't leave residual
// config files in ~/.config/glitch/workspaces/.
func DeleteWorkspaceCollectorConfig(workspaceID string) error {
	return collector.DeleteWorkspaceConfig(workspaceID)
}

// WorkspaceCollectorConfigPath returns the absolute path of a
// workspace's collectors.yaml. Used by the editor popup to show
// "this file lives at <path>" so the user knows what they're
// editing.
func WorkspaceCollectorConfigPath(workspaceID string) (string, error) {
	return collector.WorkspaceConfigPath(workspaceID)
}

// ReadWorkspaceCollectorConfig returns the raw YAML contents of a
// workspace's collectors.yaml. Creates a starter file from defaults
// if none exists, so the editor always opens to a useful starting
// point and the user never sees an empty file.
func ReadWorkspaceCollectorConfig(workspaceID string) (string, error) {
	if err := collector.EnsureWorkspaceConfig(workspaceID); err != nil {
		return "", err
	}
	path, err := collector.WorkspaceConfigPath(workspaceID)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ReadWorkspaceCollectorConfigJSON returns the workspace's collectors
// config parsed into a JSON object whose shape mirrors the typed
// collector.Config struct. The schema-driven config modal uses this
// instead of the raw YAML so it never has to ship a YAML parser.
//
// The returned config is the SAME merged view that the brain popover
// uses (ListCollectorsForWorkspace): per-workspace collectors.yaml
// PLUS workspace SQLite directories PLUS AutoDetectFromWorkspace's
// derived git+github+claude+copilot. Without this merge the modal
// would show empty fields for collectors the brain reports as live,
// because the bulk of workspace state lives in SQLite + autodetect
// rather than in the per-workspace YAML file.
//
// Comments in the YAML are NOT preserved across the round trip — that
// is the explicit trade-off for structured editing. Power users who
// need comments should keep using the raw YAML EditorPopup path.
func ReadWorkspaceCollectorConfigJSON(workspaceID string) (string, error) {
	if err := collector.EnsureWorkspaceConfig(workspaceID); err != nil {
		return "", err
	}
	cfg, err := collector.LoadWorkspaceConfig(workspaceID)
	if err != nil {
		return "", err
	}

	// Pull workspace directories from SQLite and overlay them on the
	// YAML's directories.paths so the modal shows the live list the
	// brain shows. SQLite is authoritative; YAML directories are
	// effectively legacy / power-user fallback.
	var dirs []string
	if st, sErr := OpenStore(); sErr == nil {
		if ws, wErr := st.GetWorkspace(context.Background(), workspaceID); wErr == nil {
			dirs = append(dirs, ws.Directories...)
		}
	}
	if len(dirs) > 0 {
		cfg.Directories.Paths = dirs
	}

	// Apply the same auto-detection overlay the pod manager and brain
	// popover use, so git/github/claude/copilot enablement reflect
	// what's actually running rather than the bare YAML state.
	collector.AutoDetectFromWorkspace(cfg, dirs)

	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteWorkspaceCollectorConfigJSON parses jsonBody into the typed
// collector.Config and persists it across two stores so the read-side
// merge stays consistent:
//
//  1. cfg.Directories.Paths is diffed against the workspace's SQLite
//     directory list and the differences are applied via the store's
//     AddWorkspaceDirectory / RemoveWorkspaceDirectory primitives.
//     Directories live in SQLite, not YAML.
//  2. cfg.Directories.Paths / cfg.Git.Repos / cfg.GitHub.Repos are
//     cleared from the YAML we re-marshal because those fields are
//     derived at read time from the SQLite list via AutoDetect.
//     Persisting them would freeze auto-detected entries into the
//     YAML and break the dynamic behavior.
//  3. Everything else (claude/copilot/mattermost/code_index/intervals
//     /enabled flags) round-trips through the YAML normally.
//
// Triggers the same pod restart on success.
//
// Returns nil on success, or the parse/validation/IO error so the
// modal can render it inline.
func WriteWorkspaceCollectorConfigJSON(workspaceID, jsonBody string) error {
	var cfg collector.Config
	if err := json.Unmarshal([]byte(jsonBody), &cfg); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}

	// ── Directories: diff against SQLite ─────────────────────────────
	incomingDirs := append([]string{}, cfg.Directories.Paths...)
	if st, err := OpenStore(); err == nil {
		ctx := context.Background()
		current, gErr := st.GetWorkspace(ctx, workspaceID)
		var existing []string
		if gErr == nil {
			existing = current.Directories
		}
		existingSet := make(map[string]bool, len(existing))
		for _, d := range existing {
			existingSet[d] = true
		}
		incomingSet := make(map[string]bool, len(incomingDirs))
		for _, d := range incomingDirs {
			incomingSet[d] = true
		}
		// Add new entries.
		for _, d := range incomingDirs {
			if d == "" || existingSet[d] {
				continue
			}
			if err := st.AddWorkspaceDirectory(ctx, workspaceID, d); err != nil {
				slog.Warn("write workspace config: add dir", "workspace", workspaceID, "dir", d, "err", err)
			}
		}
		// Remove deleted entries.
		for _, d := range existing {
			if incomingSet[d] {
				continue
			}
			if err := st.RemoveWorkspaceDirectory(ctx, workspaceID, d); err != nil {
				slog.Warn("write workspace config: remove dir", "workspace", workspaceID, "dir", d, "err", err)
			}
		}
	}

	// ── Strip derived fields before marshaling YAML ──────────────────
	// directories.paths lives in SQLite. git.repos / github.repos /
	// code_index.paths are all auto-detected from those directories
	// at read time via AutoDetectFromWorkspace. Persisting any of
	// them would override the live SQLite state on the next read and
	// freeze the autodetect result — e.g. removing a workspace dir
	// later wouldn't drop its code_index entry because the YAML
	// would still hold the stale absolute path.
	cfg.Directories.Paths = nil
	cfg.Git.Repos = nil
	cfg.GitHub.Repos = nil
	cfg.CodeIndex.Paths = nil

	yamlBytes, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("re-marshal yaml: %w", err)
	}
	return WriteWorkspaceCollectorConfig(workspaceID, string(yamlBytes))
}

// WriteWorkspaceCollectorConfig validates and writes new content to
// a workspace's collectors.yaml. On a successful write, the
// workspace's collector pod is restarted in the background so the
// new config takes effect without blocking the caller.
//
// The restart is fire-and-forget by design: RestartPod calls
// stopPodLocked → wg.Wait, which can block for many seconds while
// in-flight collector work (Ollama embeddings, slow HTTP polls)
// drains. The Wails save handler that calls into here MUST return
// quickly or the desktop UI freezes — and worse, every other Wails
// call (CreateWorkspace, ListWorkspaces, …) queues behind the hung
// save because Wails serializes calls. So we kick the restart onto
// a goroutine, return immediately, and let the pod cycle race in
// the background. The YAML on disk is the source of truth either
// way; the worst case is the new config takes a few extra seconds
// to take effect.
//
// Returns nil on success, or the validation/IO error so the editor
// popup can render it inline.
func WriteWorkspaceCollectorConfig(workspaceID, content string) error {
	if err := collector.WriteWorkspaceConfig(workspaceID, content); err != nil {
		return err
	}
	// Background restart. Errors are logged but never surfaced — by
	// the time the goroutine runs, the Wails save handler has
	// already returned to the frontend.
	go func() {
		if err := RestartWorkspacePod(workspaceID); err != nil {
			slog.Warn("write workspace config: restart pod",
				"workspace", workspaceID, "err", err)
		}
	}()
	return nil
}

// resetPodManagerForTest is the test-only escape hatch. The
// production singleton is sync.Once-guarded so a test that wants
// to swap in its own manager (or simulate a fresh startup) needs
// to clear the once.
func resetPodManagerForTest() {
	podOnce = sync.Once{}
	podManager = nil
	podManagerEs = nil
	_ = store.Workspace{} // satisfies linter that store import is needed
}
