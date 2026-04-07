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
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/telemetry"
)

// podTracer is the OTel tracer for collector pod lifecycle spans.
// Every workspace pod start, every collector goroutine launch, and
// every collector exit/panic is captured as a span here so the
// glitch-traces index has a queryable narrative for "what happened
// during the last 10 starts of the workspace pod for robots" — the
// exact question that turned into multiple debugging round-trips
// before this instrumentation existed.
var podTracer = otel.Tracer("gl1tch/collector/pod")

// PodManager owns the set of active workspace pods.
//
// PodManager is safe for concurrent use. Two locks divide the work:
//
//   - mu protects the pods + podLocks maps. Held only across the
//     map lookup/insert; never across the long-running goroutine
//     launches or wg.Wait() so it stays cheap.
//   - podLocks holds a per-workspace mutex that the Start / Stop /
//     Restart paths acquire for the *entire* duration of their
//     operation. This serializes concurrent ops for the same
//     workspace so a save-then-save-again can't drop a request or
//     leak goroutines, while ops for different workspaces remain
//     fully parallel.
//
// The per-workspace locks live forever (we never delete them, even
// when their pod is stopped) because allocating a fresh mutex on
// every Start would defeat the purpose if a fast Restart raced
// against a Stop. The map grows by O(workspaces), which is bounded
// by the user's actual workspace count — small.
type PodManager struct {
	es        *esearch.Client
	parentCtx context.Context
	// builder produces the collectors a pod runs from a config. The
	// production builder is BuildCollectorsFromConfig; tests inject
	// their own to avoid spinning up real git/github/mattermost
	// goroutines.
	builder PodCollectorBuilder
	// dirsResolver is set by the desktop app via
	// SetWorkspaceDirsResolver so the auto-detect overlay can read
	// the active workspace's directories without importing store.
	dirsResolver WorkspaceDirsResolver

	mu       sync.Mutex
	pods     map[string]*pod
	podLocks map[string]*sync.Mutex
}

// pod holds the runtime handles for one workspace's collectors.
type pod struct {
	workspaceID string
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// WorkspaceIDTools is the sentinel workspace_id stamped on every doc
// produced by collectors that read global per-tool data — copilot's
// flat command history, mattermost's shared channels, etc. These
// collectors run inside a single "tool pod" rather than per-workspace
// so the same source data isn't re-indexed once per workspace and
// the brain popover can OR-include this bucket alongside the active
// workspace's bucket via QueryCollectorActivityScoped.
//
// Pre-1.0 we accept that __tools__ surfaces in every workspace's
// popover with identical numbers — that's a true reflection of the
// underlying data, which genuinely IS shared. The alternative was
// either silently re-indexing per workspace (the bug we're fixing)
// or hiding the rows entirely (a regression in visibility).
const WorkspaceIDTools = "__tools__"

// PodCollectorBuilder maps a workspace's loaded Config + directory
// list into a slice of collectors ready to run. Pulled out as a
// function value so the pod manager is testable without depending
// on every concrete collector type.
//
// dirs is the workspace's directory list (from the SQLite store via
// the WorkspaceDirsResolver). Used by collectors that read global
// per-tool files (Claude history, copilot logs, etc.) to filter
// entries down to just those that belong to the active workspace —
// without it, every workspace pod re-indexes the same global data
// with its own workspace_id, and the brain popover shows identical
// counts for every workspace.
type PodCollectorBuilder func(workspaceID string, cfg *Config, dirs []string) []Collector

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
		podLocks:  map[string]*sync.Mutex{},
	}
}

// WorkspaceDirsResolver is the optional callback the pod manager
// uses to look up a workspace's directories without having to
// import store directly (which would create a cycle: store →
// collector → store). The desktop wires this to a closure that
// hits glitchd.OpenStore + ws.GetWorkspace.
//
// Returning nil from the resolver is fine — the manager just
// proceeds with no auto-detect, which preserves the legacy
// behavior for tests and headless paths.
type WorkspaceDirsResolver func(workspaceID string) []string

// SetWorkspaceDirsResolver wires the resolver. Safe to call before
// or after pods have started; the resolver is consulted on each
// StartPod / RestartPod.
func (m *PodManager) SetWorkspaceDirsResolver(r WorkspaceDirsResolver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirsResolver = r
}

// workspaceDirs returns the directories associated with a workspace
// via the configured resolver. Returns nil when no resolver is set.
func (m *PodManager) workspaceDirs(workspaceID string) []string {
	m.mu.Lock()
	r := m.dirsResolver
	m.mu.Unlock()
	if r == nil {
		return nil
	}
	return r(workspaceID)
}

// lockFor returns the per-workspace mutex for the given id, creating
// it on demand. Held by Start/Stop/Restart for the entire operation
// so two concurrent ops on the same workspace serialize cleanly.
//
// Locks are never deleted from the map. We accept the bounded leak
// (one mutex per workspace ever started) in exchange for not having
// to coordinate "is anyone else holding this lock" cleanup logic.
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
	// Serialize all start/stop/restart calls for this workspace so a
	// concurrent Restart can't race with us mid-launch.
	lk := m.lockFor(workspaceID)
	lk.Lock()
	defer lk.Unlock()
	return m.startPodLocked(workspaceID)
}

// startPodLocked is the unlocked body of StartPod. Callers MUST hold
// the per-workspace lock before invoking this. Used both by the
// public StartPod (which takes the lock for one operation) and by
// RestartPod (which holds the lock across a Stop+Start so the two
// halves are atomic from the perspective of any other caller).
//
// Wrapped in a workspace.pod.start span so the entire start
// sequence — config load, directory resolution, autodetect,
// collector instantiation, goroutine launch — is one queryable
// row in glitch-traces. Span attributes record the resolved
// directory count, autodetected git/github repo counts, and the
// final collector list size, so a "why did this pod start with
// 0 collectors" question is answered without re-running anything.
func (m *PodManager) startPodLocked(workspaceID string) error {
	ctx, span := podTracer.Start(m.parentCtx, "workspace.pod.start",
		oteltrace.WithAttributes(
			attribute.String("workspace_id", workspaceID),
		),
	)
	defer span.End()

	cfg, err := LoadWorkspaceConfig(workspaceID)
	if err != nil {
		span.SetStatus(codes.Error, "load workspace config failed")
		span.RecordError(err)
		slog.Error("pod manager: load config failed", "workspace", workspaceID, "err", err)
		return fmt.Errorf("pod manager: load config for %s: %w", workspaceID, err)
	}

	// Apply the do-what-I-mean overlay: auto-enable collectors that
	// the workspace's directories provide evidence for (.git → git,
	// origin URL → github, ~/.claude or per-dir .claude/ → claude,
	// same for copilot). Without this the user would have to manually
	// flip every collector on, even when the workspace clearly has
	// the tooling installed.
	dirs := m.workspaceDirs(workspaceID)
	AutoDetectFromWorkspace(cfg, dirs)

	span.SetAttributes(
		attribute.Int("workspace.dir_count", len(dirs)),
		attribute.Int("autodetect.git_repos", len(cfg.Git.Repos)),
		attribute.Int("autodetect.github_repos", len(cfg.GitHub.Repos)),
		attribute.Bool("autodetect.claude_enabled", cfg.Claude.Enabled),
		attribute.Bool("autodetect.copilot_enabled", cfg.Copilot.Enabled),
	)

	collectors := m.builder(workspaceID, cfg, dirs)
	collectorNames := make([]string, len(collectors))
	for i, c := range collectors {
		collectorNames[i] = c.Name()
	}
	span.SetAttributes(
		attribute.Int("collectors.count", len(collectors)),
		attribute.StringSlice("collectors.names", collectorNames),
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
	p := &pod{
		workspaceID: workspaceID,
		cancel:      cancel,
	}
	m.pods[workspaceID] = p
	m.mu.Unlock()

	slog.Info("pod manager: starting pod",
		"workspace", workspaceID,
		"collectors", len(collectors),
		"collector_names", collectorNames,
		"dirs", len(dirs),
		"git_repos", len(cfg.Git.Repos),
		"github_repos", len(cfg.GitHub.Repos))

	for _, c := range collectors {
		p.wg.Add(1)
		go func(c Collector) {
			defer p.wg.Done()
			runCollectorGuarded(podCtx, c, m.es, workspaceID)
		}(c)
	}

	span.SetStatus(codes.Ok, "pod started")
	_ = ctx // span.End is the only consumer of ctx; the goroutines run under podCtx
	return nil
}

// runCollectorGuarded runs one collector's Start in a panic-safe
// wrapper. If Start panics — most commonly because m.es is nil and
// the collector tries to BulkIndex on the very first poll, or because
// an external CLI subprocess returns malformed output that the
// collector's parser doesn't handle — we recover, log the panic, and
// emit a RecordRun with the panic message as the error so the brain
// popover surfaces a red dot with a real failure reason instead of
// silently gray-dotting forever.
//
// Without this wrapper a collector panic kills its goroutine without
// updating the run registry; the popover row reads "0 indexed" with
// no last_run_ms and no last_run_error, which is indistinguishable
// from "the collector hasn't fired its first tick yet". The user has
// no way to tell the difference and no way to discover the panic
// short of catting the dev console at the exact moment it happened.
//
// Used by both the per-workspace pod path (startPodLocked) and the
// global tool pod path (startToolPodLocked) so every collector that
// runs through the manager gets the same protection.
func runCollectorGuarded(ctx context.Context, c Collector, es *esearch.Client, workspaceID string) {
	// Each collector goroutine gets its own span so glitch-traces
	// has a row per (workspace, collector) launch with attributes
	// that answer "did this collector run, did it panic, when did
	// it exit, and was es nil at launch time". This is the row I
	// keep wishing existed every time the popover dot is gray.
	ctx, span := podTracer.Start(ctx, "collector.run",
		oteltrace.WithAttributes(
			attribute.String("workspace_id", workspaceID),
			attribute.String("collector", c.Name()),
			attribute.Bool("es_nil", es == nil),
		),
	)
	defer span.End()

	defer func() {
		if r := recover(); r != nil {
			slog.Error("pod manager: collector panicked",
				"workspace", workspaceID,
				"collector", c.Name(),
				"panic", r)
			err := fmt.Errorf("collector panicked: %v", r)
			span.SetStatus(codes.Error, "panic")
			span.RecordError(err)
			RecordRun(c.Name(), time.Now(), 0, err)
			// Ship the recovered panic to Elastic APM with a full
			// Go stack so Kibana's Errors UI shows this as a
			// handled exception correlated to the parent
			// collector.run span. stackSkip=1 trims this anon
			// defer func so the top frame is the collector's own
			// panicking code.
			telemetry.CaptureError(ctx, err, map[string]any{
				"workspace_id": workspaceID,
				"collector":    c.Name(),
			}, 1)
		}
	}()
	slog.Info("pod manager: collector started",
		"workspace", workspaceID, "collector", c.Name())
	span.AddEvent("collector.alive")
	// "Alive" heartbeat fired BEFORE delegating to the collector's
	// own Start. Without this, the popover row's last_run_ms stays
	// at zero until the collector reaches its OWN RecordRun call —
	// which for most collectors only happens on the first ticker
	// tick, anywhere from 60s to 5min after launch depending on the
	// configured Interval. The user opens the popover, sees a gray
	// dot, and has no way to tell whether the collector is "starting
	// up" or "silently dead in another process".
	//
	// More importantly, the in-process run registry is per-process.
	// If there's a second gl1tch instance running (e.g. a stale
	// `glitch serve` from another terminal, or a wails dev rebuild
	// that orphaned the previous binary), the popover-rendering
	// process and the indexing process can be two different binaries
	// with two different registries. They share ES so document
	// counts agree, but heartbeats don't. An "I'm alive" heartbeat
	// at goroutine launch makes the THIS process's collectors
	// visibly distinguishable from the other process's, so the user
	// can immediately tell which side is doing the work.
	//
	// indexed=0 + nil error is the "freshly launched, no work yet"
	// signal. The popover dot logic interprets it as cyan
	// (`total > 0 || lastRun > 0`) and the row shows "ran just now"
	// even before the first real poll completes.
	RecordRun(c.Name(), time.Now(), 0, nil)
	if err := c.Start(ctx, es); err != nil && ctx.Err() == nil {
		slog.Warn("pod manager: collector exited",
			"workspace", workspaceID, "collector", c.Name(), "err", err)
		span.SetStatus(codes.Error, "exited with error")
		span.RecordError(err)
		// Surface a clean exit error to the popover too. ctx-cancel
		// path is excluded above so a normal pod stop doesn't paint
		// every collector red.
		RecordRun(c.Name(), time.Now(), 0,
			fmt.Errorf("collector exited: %w", err))
	} else if ctx.Err() != nil {
		// Normal pod stop — span ends in OK status, popover stays
		// the same color it was when the pod was alive.
		span.SetStatus(codes.Ok, "context cancelled")
	}
}

// StopPod cancels the workspace's pod context and waits for every
// collector goroutine to return. Idempotent: stopping a workspace
// that has no active pod is not an error.
//
// Stop blocks until every collector has actually exited so the
// caller can safely create or recreate state for the same workspace
// id afterward without leaking goroutines.
func (m *PodManager) StopPod(workspaceID string) error {
	if workspaceID == "" {
		return nil
	}
	// Same serialization as StartPod — a Stop racing a concurrent
	// Start would otherwise drop one of the operations.
	lk := m.lockFor(workspaceID)
	lk.Lock()
	defer lk.Unlock()
	return m.stopPodLocked(workspaceID)
}

// stopPodLocked is the unlocked body of StopPod. Callers MUST hold
// the per-workspace lock.
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
	p.wg.Wait()
	return nil
}

// RestartPod stops and immediately re-starts a workspace's pod. Used
// when the workspace's collectors.yaml has been edited so the new
// config takes effect without requiring a full app restart.
//
// Safe to call when no pod is currently running — it just does the
// start half. The per-workspace lock is held across the entire
// stop+start so two concurrent restarts on the same workspace
// serialize fully (no goroutine leaks, no dropped operations).
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
//
// dirs is the workspace's directory list (from SQLite via the
// dirsResolver), passed through to Claude collectors so they can
// filter the global ~/.claude history down to just those entries
// whose project path belongs to this workspace. An empty dirs slice
// disables filtering — used by tests and the headless `glitch serve`
// path.
func BuildCollectorsFromConfig(workspaceID string, cfg *Config, dirs []string) []Collector {
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
			&ClaudeCollector{
				Interval:    cfg.Claude.Interval,
				WorkspaceID: workspaceID,
				Dirs:        dirs,
			},
			&ClaudeProjectCollector{
				Interval:    cfg.Claude.Interval,
				WorkspaceID: workspaceID,
				Dirs:        dirs,
			},
		)
	}

	// Copilot is intentionally NOT registered as a per-workspace
	// collector. The data source (~/.copilot/command-history-state.json)
	// is a flat array of command strings with no project / cwd
	// metadata, so the same commands would be re-indexed under every
	// workspace_id and the brain popover would show identical copilot
	// counts in every workspace — which is what the screenshot bug
	// looked like before we started tracking this down. Until copilot
	// surfaces a per-project signal (or we add a separate "global
	// tool" collector path that runs once with workspace_id=""), this
	// collector is dropped from the per-workspace builder entirely.
	//
	// The headless `glitch serve` path still runs copilot via the
	// supervisor's RegisterCollectors call, so users without the
	// desktop app keep getting their copilot history indexed.
	_ = cfg.Copilot.Enabled // see comment above

	if len(cfg.GitHub.Repos) > 0 {
		out = append(out, &GitHubCollector{
			Repos:       cfg.GitHub.Repos,
			Interval:    cfg.GitHub.Interval,
			WorkspaceID: workspaceID,
		})
	}

	// Mattermost is intentionally NOT registered as a per-workspace
	// collector. The data source is a single Mattermost server
	// shared across every workspace; running one client per pod
	// re-indexes the same channels under each workspace_id and the
	// brain popover ends up showing identical mattermost counts
	// everywhere. The tool-pod path (BuildToolCollectorsFromConfig)
	// runs a single instance with workspace_id=WorkspaceIDTools, and
	// the popover query OR-includes that bucket alongside the active
	// workspace.
	_ = cfg.Mattermost.URL // see comment above

	// Directory collector always runs (even with empty paths) so the
	// user can add directories at runtime via the desktop's "add
	// directory" button. The collector re-reads its config on each
	// tick so new paths get picked up without a pod restart.
	out = append(out, &DirectoryCollector{
		Dirs:        cfg.Directories.Paths,
		Interval:    cfg.Directories.Interval,
		WorkspaceID: workspaceID,
	})

	// Code-index is opt-in: only spawn when the user has enabled it
	// AND given at least one path. Embedding a large tree on first
	// run can take minutes, so we never auto-start it.
	if cfg.CodeIndex.Enabled && len(cfg.CodeIndex.Paths) > 0 {
		out = append(out, &CodeIndexCollector{
			Paths:         cfg.CodeIndex.Paths,
			Extensions:    cfg.CodeIndex.Extensions,
			ChunkSize:     cfg.CodeIndex.ChunkSize,
			Interval:      cfg.CodeIndex.Interval,
			EmbedProvider: cfg.CodeIndex.EmbedProvider,
			EmbedModel:    cfg.CodeIndex.EmbedModel,
			EmbedBaseURL:  cfg.CodeIndex.EmbedBaseURL,
			EmbedAPIKey:   cfg.CodeIndex.EmbedAPIKey,
			WorkspaceID:   workspaceID,
		})
	}

	// PipelineIndexer is workspace-scoped too so each workspace's
	// pipeline runs land in its own slice of glitch-pipelines.
	out = append(out, &PipelineIndexer{WorkspaceID: workspaceID})

	return out
}

// BuildToolCollectorsFromConfig instantiates the "global tool"
// collector set: copilot + mattermost. These read shared, machine-
// global data sources and run inside a single dedicated pod with
// workspace_id=WorkspaceIDTools so the same source data isn't
// re-indexed once per workspace.
//
// The corresponding popover/query path OR-includes the tools bucket
// alongside the active workspace's bucket so users still see these
// rows in the brain popover with real (one-and-only) counts.
//
// cfg here is the GLOBAL observer.yaml (collector.LoadConfig), not a
// per-workspace collectors.yaml. Tool collectors aren't editable
// per-workspace because their data isn't workspace-scoped to begin
// with.
func BuildToolCollectorsFromConfig(cfg *Config) []Collector {
	var out []Collector
	if cfg == nil {
		return out
	}

	if cfg.Copilot.Enabled {
		out = append(out, &CopilotCollector{
			Interval:    cfg.Copilot.Interval,
			WorkspaceID: WorkspaceIDTools,
		})
	}

	if cfg.Mattermost.URL != "" && cfg.Mattermost.Token != "" {
		out = append(out, &MattermostCollector{
			URL:         cfg.Mattermost.URL,
			Token:       cfg.Mattermost.Token,
			Channels:    cfg.Mattermost.Channels,
			Interval:    cfg.Mattermost.Interval,
			WorkspaceID: WorkspaceIDTools,
		})
	}

	return out
}

// StartToolPod starts the dedicated tool collectors pod under the
// reserved WorkspaceIDTools key. The tool pod loads the GLOBAL
// observer.yaml (not a per-workspace file) and runs the collector
// set returned by BuildToolCollectorsFromConfig.
//
// Idempotent at the same level as StartPod: a duplicate StartToolPod
// while one is already running returns the same "already running"
// error. Use RestartToolPod to cycle.
func (m *PodManager) StartToolPod() error {
	lk := m.lockFor(WorkspaceIDTools)
	lk.Lock()
	defer lk.Unlock()
	return m.startToolPodLocked()
}

// startToolPodLocked is the unlocked body of StartToolPod. Mirrors
// startPodLocked but uses the global observer.yaml and the tool
// collector builder.
func (m *PodManager) startToolPodLocked() error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("pod manager: load global config for tool pod: %w", err)
	}

	collectors := BuildToolCollectorsFromConfig(cfg)
	if len(collectors) == 0 {
		// Nothing enabled — silently no-op so a user with no copilot
		// or mattermost configured doesn't see scary errors.
		slog.Info("pod manager: tool pod has no enabled collectors, skipping")
		return nil
	}

	m.mu.Lock()
	if _, exists := m.pods[WorkspaceIDTools]; exists {
		m.mu.Unlock()
		return fmt.Errorf("pod manager: tool pod already running")
	}
	ctx, cancel := context.WithCancel(m.parentCtx)
	p := &pod{
		workspaceID: WorkspaceIDTools,
		cancel:      cancel,
	}
	m.pods[WorkspaceIDTools] = p
	m.mu.Unlock()

	slog.Info("pod manager: starting tool pod", "collectors", len(collectors))

	for _, c := range collectors {
		p.wg.Add(1)
		go func(c Collector) {
			defer p.wg.Done()
			// Same panic-and-exit guard as the per-workspace path,
			// keyed under the tool pod's reserved workspace id so
			// red-dot diagnostics work uniformly across both surfaces.
			runCollectorGuarded(ctx, c, m.es, WorkspaceIDTools)
		}(c)
	}

	return nil
}

// StopToolPod cancels the tool pod's context and waits for every
// collector goroutine to return. Idempotent.
func (m *PodManager) StopToolPod() error {
	lk := m.lockFor(WorkspaceIDTools)
	lk.Lock()
	defer lk.Unlock()
	return m.stopPodLocked(WorkspaceIDTools)
}
