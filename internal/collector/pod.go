// pod.go runs collectors per workspace.
//
// A pod is a tree of collector goroutines scoped to one workspace.
// Each pod loads the workspace's collectors.yaml, instantiates one
// collector per enabled section (git, github, claude, copilot,
// mattermost, directories), stamps every indexed event with the
// workspace id, and runs them under a per-pod context.
//
// PodManager owns the lifetime of all active pods. The desktop app
// creates one PodManager at startup and asks it to start a pod for
// every existing workspace and again on every workspace add. Workspace
// delete tears down the pod.
//
// Why a pod manager instead of using the supervisor's RegisterService?
//   - The supervisor takes a service snapshot at Start() time and
//     can't add or remove services after that. Workspace add/remove
//     happens at runtime; we need real lifecycle.
//   - Pods are conceptually independent of the global supervisor
//     services (cron, busd handlers, the global collector registry).
//     Putting them in their own manager keeps the surface honest.
//   - The pod manager is testable in isolation with a fake collector
//     and a stub ES client; the supervisor's busd-driven dispatch
//     loop is not.
package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// PodManager owns the set of active workspace pods.
//
// PodManager is safe for concurrent use; the desktop app may call
// StartPod / StopPod from any goroutine in response to UI actions.
// Internal state is protected by a single mutex held only across
// map mutations and pod lookups, never across the long-running
// goroutine launches themselves.
type PodManager struct {
	es        *esearch.Client
	parentCtx context.Context
	// builder produces the collectors a pod runs from a config. The
	// production builder is BuildCollectorsFromConfig; tests inject
	// their own to avoid spinning up real git/github/mattermost
	// goroutines.
	builder PodCollectorBuilder

	mu   sync.Mutex
	pods map[string]*pod
}

// pod holds the runtime handles for one workspace's collectors.
type pod struct {
	workspaceID string
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// PodCollectorBuilder maps a workspace's loaded Config into a slice
// of collectors ready to run. Pulled out as a function value so the
// pod manager is testable without depending on every concrete
// collector type.
type PodCollectorBuilder func(workspaceID string, cfg *Config) []Collector

// NewPodManager constructs a manager bound to a parent context. All
// pods run as children of parentCtx — when the parent is cancelled
// every pod is torn down automatically.
//
// The es client is passed to every collector's Start. Pass nil
// during tests with a builder that doesn't actually call Start.
func NewPodManager(parentCtx context.Context, es *esearch.Client, builder PodCollectorBuilder) *PodManager {
	if builder == nil {
		builder = BuildCollectorsFromConfig
	}
	return &PodManager{
		es:        es,
		parentCtx: parentCtx,
		builder:   builder,
		pods:      map[string]*pod{},
	}
}

// StartPod loads the workspace's config and starts every collector
// it declares. Returns an error if a pod for the same workspace is
// already running — callers should StopPod first or use Restart.
//
// Each collector is launched in its own goroutine under a per-pod
// context derived from the manager's parentCtx, so cancelling the
// pod tears down every collector cleanly.
//
// A pod with no enabled collectors is allowed and not an error —
// new workspaces start empty until the user adds sources via the
// editor popup.
func (m *PodManager) StartPod(workspaceID string) error {
	if workspaceID == "" {
		return errors.New("pod manager: workspace id is required")
	}

	cfg, err := LoadWorkspaceConfig(workspaceID)
	if err != nil {
		return fmt.Errorf("pod manager: load config for %s: %w", workspaceID, err)
	}

	collectors := m.builder(workspaceID, cfg)

	m.mu.Lock()
	if _, exists := m.pods[workspaceID]; exists {
		m.mu.Unlock()
		return fmt.Errorf("pod manager: pod for %s already running", workspaceID)
	}
	ctx, cancel := context.WithCancel(m.parentCtx)
	p := &pod{
		workspaceID: workspaceID,
		cancel:      cancel,
	}
	m.pods[workspaceID] = p
	m.mu.Unlock()

	slog.Info("pod manager: starting pod", "workspace", workspaceID, "collectors", len(collectors))

	for _, c := range collectors {
		p.wg.Add(1)
		go func(c Collector) {
			defer p.wg.Done()
			slog.Info("pod manager: collector started", "workspace", workspaceID, "collector", c.Name())
			if err := c.Start(ctx, m.es); err != nil && ctx.Err() == nil {
				slog.Warn("pod manager: collector exited", "workspace", workspaceID, "collector", c.Name(), "err", err)
			}
		}(c)
	}

	return nil
}

// StopPod cancels the workspace's pod context and waits for every
// collector goroutine to return. Idempotent: stopping a workspace
// that has no active pod is not an error.
//
// Stop blocks until every collector has actually exited so the
// caller can safely create or recreate state for the same workspace
// id afterward without leaking goroutines.
func (m *PodManager) StopPod(workspaceID string) error {
	m.mu.Lock()
	p, ok := m.pods[workspaceID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.pods, workspaceID)
	m.mu.Unlock()

	slog.Info("pod manager: stopping pod", "workspace", workspaceID)
	p.cancel()
	p.wg.Wait()
	return nil
}

// RestartPod stops and immediately re-starts a workspace's pod. Used
// when the workspace's collectors.yaml has been edited so the new
// config takes effect without requiring a full app restart.
//
// Safe to call when no pod is currently running — it just does the
// start half.
func (m *PodManager) RestartPod(workspaceID string) error {
	if err := m.StopPod(workspaceID); err != nil {
		return err
	}
	return m.StartPod(workspaceID)
}

// ActiveWorkspaces returns the workspace ids of pods currently
// running. Order is not stable.
func (m *PodManager) ActiveWorkspaces() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.pods))
	for id := range m.pods {
		out = append(out, id)
	}
	return out
}

// StopAll tears down every active pod. Used at app shutdown so the
// caller doesn't have to enumerate workspace ids.
func (m *PodManager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.pods))
	for id := range m.pods {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.StopPod(id)
	}
}

// BuildCollectorsFromConfig instantiates the collector set declared
// by a workspace's Config. This is the production builder used by
// PodManager when no override is supplied.
//
// Each collector gets the workspace id stamped on it so its
// indexed events carry the right workspace_id field. Empty sections
// (no repos, no channels, etc.) are skipped to avoid spawning idle
// goroutines.
func BuildCollectorsFromConfig(workspaceID string, cfg *Config) []Collector {
	var out []Collector
	if cfg == nil {
		return out
	}

	if len(cfg.Git.Repos) > 0 {
		out = append(out, &GitCollector{
			Repos:       cfg.Git.Repos,
			Interval:    cfg.Git.Interval,
			WorkspaceID: workspaceID,
		})
	}

	if cfg.Claude.Enabled {
		out = append(out,
			&ClaudeCollector{Interval: cfg.Claude.Interval, WorkspaceID: workspaceID},
			&ClaudeProjectCollector{Interval: cfg.Claude.Interval, WorkspaceID: workspaceID},
		)
	}

	if cfg.Copilot.Enabled {
		out = append(out, &CopilotCollector{
			Interval:    cfg.Copilot.Interval,
			WorkspaceID: workspaceID,
		})
	}

	if len(cfg.GitHub.Repos) > 0 {
		out = append(out, &GitHubCollector{
			Repos:       cfg.GitHub.Repos,
			Interval:    cfg.GitHub.Interval,
			WorkspaceID: workspaceID,
		})
	}

	if cfg.Mattermost.URL != "" && cfg.Mattermost.Token != "" {
		out = append(out, &MattermostCollector{
			URL:         cfg.Mattermost.URL,
			Token:       cfg.Mattermost.Token,
			Channels:    cfg.Mattermost.Channels,
			Interval:    cfg.Mattermost.Interval,
			WorkspaceID: workspaceID,
		})
	}

	// Directory collector always runs (even with empty paths) so the
	// user can add directories at runtime via the desktop's "add
	// directory" button. The collector re-reads its config on each
	// tick so new paths get picked up without a pod restart.
	out = append(out, &DirectoryCollector{
		Dirs:        cfg.Directories.Paths,
		Interval:    cfg.Directories.Interval,
		WorkspaceID: workspaceID,
	})

	// PipelineIndexer is workspace-scoped too so each workspace's
	// pipeline runs land in its own slice of glitch-pipelines.
	out = append(out, &PipelineIndexer{WorkspaceID: workspaceID})

	return out
}
