// pod_manager.go runs capabilities per workspace.
//
// A pod is one capability.Runner scoped to one workspace. Each pod loads the
// workspace's config, instantiates a registry populated with every capability
// that workspace has asked for (workspace scanner, claude, claude-projects,
// code-index, pipeline), and hands it to a runner that drives the lifecycle.
//
// PodManager owns the lifetime of every active pod. The desktop app creates
// one PodManager at startup and asks it to start a pod for every existing
// workspace and again on every workspace add. Workspace delete tears down
// the pod.
//
// Why a pod manager instead of the supervisor's RegisterService?
//   - The supervisor takes a service snapshot at Start() time and can't
//     add or remove services after that. Workspace add/remove happens at
//     runtime; we need real lifecycle.
//   - Pods are conceptually independent of the global supervisor services
//     (cron, busd handlers, global capability registry). Putting them in
//     their own manager keeps the surface honest.
package capability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// WorkspaceIDTools is the sentinel workspace_id stamped on every doc produced
// by global-tool capabilities — copilot's flat command history being the sole
// current example. Tool capabilities run inside a single "tool pod" rather
// than per-workspace so the same source data is not re-indexed once per
// workspace. The brain popover OR-includes this bucket alongside the active
// workspace's bucket so tool rows render with real counts.
const WorkspaceIDTools = "__tools__"

// PodCapabilityBuilder maps a workspace's loaded Config + directory list
// into a slice of capabilities ready to run. Used only by tests — the
// production path calls (*PodManager).buildWorkspaceCapabilities directly
// so new data sources can be added by editing one function instead of
// being injected via a callback.
type PodCapabilityBuilder func(workspaceID string, cfg *Config, dirs []string) []Capability

// PodManager owns the set of active workspace pods. Safe for concurrent use
// via a per-workspace mutex so Start / Stop / Restart calls for the same
// workspace serialise, while ops for different workspaces remain parallel.
type PodManager struct {
	es           *esearch.Client
	parentCtx    context.Context
	dirsResolver WorkspaceDirsResolver

	mu            sync.Mutex
	pods          map[string]*pod
	toolPod       *pod
	podLocks      map[string]*sync.Mutex
	customBuilder PodCapabilityBuilder
	toolBuilder   PodCapabilityBuilder
}

// pod holds the runtime handles for one workspace's runner.
type pod struct {
	workspaceID string
	runner      *Runner
	cancel      context.CancelFunc
}

// WorkspaceDirsResolver is the optional callback the pod manager uses to
// look up a workspace's directories without having to import the store
// package directly (which would create a cycle). The desktop wires this to a
// closure that hits glitchd.OpenStore + ws.GetWorkspace.
type WorkspaceDirsResolver func(workspaceID string) []string

// NewPodManager constructs a manager bound to a parent context. All pods run
// as children of parentCtx — cancelling the parent tears down every pod. The
// es client is used by capabilities that bulk-index through the runner and
// by code-index capabilities that talk to brainrag.
func NewPodManager(parentCtx context.Context, es *esearch.Client) *PodManager {
	return &PodManager{
		es:        es,
		parentCtx: parentCtx,
		pods:      map[string]*pod{},
		podLocks:  map[string]*sync.Mutex{},
	}
}

// SetWorkspaceDirsResolver wires the resolver. Safe to call before or after
// pods have started; the resolver is consulted on each StartPod / RestartPod.
func (m *PodManager) SetWorkspaceDirsResolver(r WorkspaceDirsResolver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirsResolver = r
}

// SetCapabilityBuilder overrides the per-workspace capability constructor.
// Intended for tests — the production path reads Config directly to build
// the real capability set. Passing nil restores the default.
func (m *PodManager) SetCapabilityBuilder(b PodCapabilityBuilder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customBuilder = b
}

// SetToolCapabilityBuilder overrides the tool-pod capability constructor.
// Intended for tests. Passing nil restores the default.
func (m *PodManager) SetToolCapabilityBuilder(b PodCapabilityBuilder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolBuilder = b
}

func (m *PodManager) workspaceDirs(workspaceID string) []string {
	m.mu.Lock()
	r := m.dirsResolver
	m.mu.Unlock()
	if r == nil {
		return nil
	}
	return r(workspaceID)
}

func (m *PodManager) lockFor(workspaceID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	lk, ok := m.podLocks[workspaceID]
	if !ok {
		lk = &sync.Mutex{}
		m.podLocks[workspaceID] = lk
	}
	return lk
}

// StartPod loads the workspace's config and starts every capability it
// declares. Returns an error if a pod for the same workspace is already
// running — callers should StopPod first or use RestartPod.
func (m *PodManager) StartPod(workspaceID string) error {
	if workspaceID == "" {
		return errors.New("pod manager: workspace id is required")
	}
	lk := m.lockFor(workspaceID)
	lk.Lock()
	defer lk.Unlock()
	return m.startPodLocked(workspaceID)
}

func (m *PodManager) startPodLocked(workspaceID string) error {
	ctx, span := podTracer.Start(m.parentCtx, "workspace.pod.start",
		oteltrace.WithAttributes(attribute.String("workspace_id", workspaceID)),
	)
	defer span.End()

	cfg, err := LoadWorkspaceConfig(workspaceID)
	if err != nil {
		span.SetStatus(codes.Error, "load workspace config failed")
		span.RecordError(err)
		slog.Error("pod manager: load config failed", "workspace", workspaceID, "err", err)
		return fmt.Errorf("pod manager: load config for %s: %w", workspaceID, err)
	}

	dirs := m.workspaceDirs(workspaceID)
	AutoDetectFromWorkspace(cfg, dirs)

	span.SetAttributes(
		attribute.Int("workspace.dir_count", len(dirs)),
		attribute.Bool("autodetect.claude_enabled", cfg.Claude.Enabled),
		attribute.Bool("autodetect.copilot_enabled", cfg.Copilot.Enabled),
	)

	reg := NewRegistry()
	m.mu.Lock()
	builder := m.customBuilder
	m.mu.Unlock()
	var caps []Capability
	if builder != nil {
		caps = builder(workspaceID, cfg, dirs)
	} else {
		caps = m.buildWorkspaceCapabilities(workspaceID, cfg, dirs)
	}
	for _, c := range caps {
		if err := reg.Register(c); err != nil {
			slog.Warn("pod manager: register capability failed",
				"workspace", workspaceID, "err", err)
		}
	}
	names := reg.Names()
	span.SetAttributes(
		attribute.Int("capabilities.count", len(names)),
		attribute.StringSlice("capabilities.names", names),
	)

	m.mu.Lock()
	if _, exists := m.pods[workspaceID]; exists {
		m.mu.Unlock()
		err := fmt.Errorf("pod manager: pod for %s already running", workspaceID)
		span.SetStatus(codes.Error, "pod already running")
		span.RecordError(err)
		return err
	}
	podCtx, cancel := context.WithCancel(m.parentCtx)
	runner := NewRunner(reg, m.es)
	runner.SetWorkspaceID(workspaceID)
	runner.SetAfterInvoke(func(name string, dur time.Duration, indexed int, ierr error) {
		RecordRun(name, time.Now().Add(-dur), indexed, ierr)
	})
	p := &pod{
		workspaceID: workspaceID,
		runner:      runner,
		cancel:      cancel,
	}
	m.pods[workspaceID] = p
	m.mu.Unlock()

	slog.Info("pod manager: starting pod",
		"workspace", workspaceID,
		"capabilities", len(names),
		"capability_names", names,
		"dirs", len(dirs))

	runner.Start(podCtx)
	span.SetStatus(codes.Ok, "pod started")
	_ = ctx
	return nil
}

// buildWorkspaceCapabilities instantiates the capability set for one
// workspace pod from the workspace's Config + directory list. Used only at
// pod start — tests can construct capabilities directly without going through
// here.
//
// Copilot is intentionally NOT included here. Its data source is a flat
// global command history with no per-project metadata, so it lives in the
// dedicated tool pod (StartToolPod) under WorkspaceIDTools instead.
func (m *PodManager) buildWorkspaceCapabilities(workspaceID string, cfg *Config, dirs []string) []Capability {
	var out []Capability

	if len(dirs) > 0 {
		out = append(out, &WorkspaceCapability{
			Dirs:        dirs,
			Interval:    cfg.Directories.Interval,
			WorkspaceID: workspaceID,
		})
	}

	if cfg.Claude.Enabled {
		out = append(out,
			&ClaudeHistoryCapability{
				Interval:    cfg.Claude.Interval,
				WorkspaceID: workspaceID,
				Dirs:        dirs,
			},
			&ClaudeProjectsCapability{
				Interval:    cfg.Claude.Interval,
				WorkspaceID: workspaceID,
				Dirs:        dirs,
			},
		)
	}

	if cfg.CodeIndex.Enabled && len(cfg.CodeIndex.Paths) > 0 {
		out = append(out, &CodeIndexCapability{
			Paths:         cfg.CodeIndex.Paths,
			Extensions:    cfg.CodeIndex.Extensions,
			ChunkSize:     cfg.CodeIndex.ChunkSize,
			Interval:      cfg.CodeIndex.Interval,
			EmbedProvider: cfg.CodeIndex.EmbedProvider,
			EmbedModel:    cfg.CodeIndex.EmbedModel,
			EmbedBaseURL:  cfg.CodeIndex.EmbedBaseURL,
			EmbedAPIKey:   cfg.CodeIndex.EmbedAPIKey,
			WorkspaceID:   workspaceID,
			ES:            m.es,
		})
	}

	out = append(out, &PipelineRunsCapability{WorkspaceID: workspaceID})

	return out
}

// StopPod cancels the workspace's pod context and waits for every capability
// goroutine to exit. Idempotent: stopping a workspace that has no active pod
// is not an error.
func (m *PodManager) StopPod(workspaceID string) error {
	if workspaceID == "" {
		return nil
	}
	lk := m.lockFor(workspaceID)
	lk.Lock()
	defer lk.Unlock()
	return m.stopPodLocked(workspaceID)
}

func (m *PodManager) stopPodLocked(workspaceID string) error {
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
	p.runner.Stop()
	return nil
}

// RestartPod stops and immediately re-starts a workspace's pod. Used when a
// workspace's collectors.yaml has been edited so the new config takes effect
// without requiring a full app restart.
func (m *PodManager) RestartPod(workspaceID string) error {
	if workspaceID == "" {
		return errors.New("pod manager: workspace id is required")
	}
	lk := m.lockFor(workspaceID)
	lk.Lock()
	defer lk.Unlock()
	if err := m.stopPodLocked(workspaceID); err != nil {
		return err
	}
	return m.startPodLocked(workspaceID)
}

// ActiveWorkspaces returns the workspace ids of pods currently running.
// Order is not stable.
func (m *PodManager) ActiveWorkspaces() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.pods))
	for id := range m.pods {
		out = append(out, id)
	}
	return out
}

// StopAll tears down every active pod including the tool pod. Used at app
// shutdown so the caller does not have to enumerate workspace ids.
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
	_ = m.StopToolPod()
}

// StartToolPod starts the dedicated tool-capabilities pod under the reserved
// WorkspaceIDTools key. The tool pod reads the GLOBAL observer config and
// runs capabilities whose data source is machine-global (currently just
// copilot). Running copilot in a per-workspace pod would re-index the same
// flat command history under every workspace_id.
func (m *PodManager) StartToolPod() error {
	m.mu.Lock()
	if m.toolPod != nil {
		m.mu.Unlock()
		return fmt.Errorf("pod manager: tool pod already running")
	}
	m.mu.Unlock()

	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("pod manager: load tool config: %w", err)
	}

	reg := NewRegistry()
	m.mu.Lock()
	toolBuilder := m.toolBuilder
	m.mu.Unlock()
	if toolBuilder != nil {
		for _, c := range toolBuilder(WorkspaceIDTools, cfg, nil) {
			_ = reg.Register(c)
		}
	} else if cfg.Copilot.Enabled {
		_ = reg.Register(&CopilotCapability{
			Interval:    cfg.Copilot.Interval,
			WorkspaceID: WorkspaceIDTools,
		})
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.toolPod != nil {
		return fmt.Errorf("pod manager: tool pod already running")
	}
	podCtx, cancel := context.WithCancel(m.parentCtx)
	runner := NewRunner(reg, m.es)
	runner.SetWorkspaceID(WorkspaceIDTools)
	runner.SetAfterInvoke(func(name string, dur time.Duration, indexed int, ierr error) {
		RecordRun(name, time.Now().Add(-dur), indexed, ierr)
	})
	m.toolPod = &pod{
		workspaceID: WorkspaceIDTools,
		runner:      runner,
		cancel:      cancel,
	}
	slog.Info("pod manager: starting tool pod", "capabilities", reg.Names())
	runner.Start(podCtx)
	return nil
}

// StopToolPod tears down the tool-capabilities pod. Idempotent.
func (m *PodManager) StopToolPod() error {
	m.mu.Lock()
	p := m.toolPod
	m.toolPod = nil
	m.mu.Unlock()
	if p == nil {
		return nil
	}
	slog.Info("pod manager: stopping tool pod")
	p.cancel()
	p.runner.Stop()
	return nil
}
