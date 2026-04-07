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
	"errors"
	"log/slog"
	"os"
	"sync"

	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/store"
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

// WriteWorkspaceCollectorConfig validates and writes new content to
// a workspace's collectors.yaml. On a successful write, the
// workspace's collector pod is restarted so the new config takes
// effect immediately without requiring an app restart.
//
// Returns nil on success, or the validation/IO error so the editor
// popup can render it inline.
func WriteWorkspaceCollectorConfig(workspaceID, content string) error {
	if err := collector.WriteWorkspaceConfig(workspaceID, content); err != nil {
		return err
	}
	// Best-effort pod restart. If the pod manager isn't initialized
	// (test path), or if the pod hasn't been started yet, we just
	// log and continue — the next StartWorkspacePod call will pick
	// up the new config.
	if err := RestartWorkspacePod(workspaceID); err != nil {
		slog.Warn("write workspace config: restart pod", "workspace", workspaceID, "err", err)
	}
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
