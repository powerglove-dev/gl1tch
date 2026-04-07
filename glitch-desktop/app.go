package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/8op-org/gl1tch/pkg/glitchd"
)

type App struct {
	ctx              context.Context
	cancelBackend    context.CancelFunc
	telemetryShutdown func(context.Context) error
	notifyProc       *os.Process

	// brainStarted guards runBrainLoop so a duplicate Ready() call (HMR,
	// frontend reconnect) doesn't spawn a second loop and double-emit
	// every check-in.
	brainStarted sync.Once
	// brainAnchor is the wall-clock time the brain loop started. The
	// frontend uses it to compute "next run" countdowns for collectors
	// without us having to push a tick on every interval.
	brainAnchor time.Time

	// collectorState is the most recent activity snapshot from
	// Elasticsearch, keyed by source name. The brain loop refreshes it
	// every collectorPollInterval and computes deltas between polls so
	// the UI can show "got 12 new commits in the last 30s" instead of
	// just a derived "next in" countdown.
	collectorMu    sync.Mutex
	collectorState map[string]glitchd.CollectorActivity

	// triageMu guards triageLastMs (the high-water mark of the most
	// recent event the triage loop has already analyzed). Used to ask
	// ES for "events newer than X" on each tick instead of re-feeding
	// the model the same buffer over and over.
	triageMu     sync.Mutex
	triageLastMs int64

	// runs tracks the cancel func for the in-flight run of each workspace.
	// Keyed by workspace ID. The frontend's stop button calls StopRun, which
	// looks up the entry and cancels the per-run context. Each streaming
	// entry point (AskScoped/AskProvider/RunChain/RunWorkflow/Doctor)
	// registers itself before kicking off work and unregisters in defer.
	// Each entry carries a generation token so the release func only
	// deletes the slot it actually owns — protecting against the (rare)
	// case where a newer run for the same workspace has taken over.
	runsMu  sync.Mutex
	runs    map[string]runHandle
	runsGen uint64

	// stepThrough holds active interactive-authoring sessions keyed by
	// session ID. Each entry is driven by a separate sequence of calls
	// from the frontend (Start → Accept/EditOutput pairs → SaveAs/Abort),
	// so sessions are independent and long-lived relative to `runs`. The
	// map is cleaned up when the session terminates.
	stepThroughMu sync.Mutex
	stepThrough   map[string]*glitchd.StepThroughHandle
}

type runHandle struct {
	gen    uint64
	cancel context.CancelFunc
}

func NewApp() *App {
	return &App{
		runs:           map[string]runHandle{},
		collectorState: map[string]glitchd.CollectorActivity{},
		stepThrough:    map[string]*glitchd.StepThroughHandle{},
	}
}

// registerRun derives a child context from the App's lifetime context,
// stores its cancel func under workspaceID (cancelling any prior run for
// the same workspace), and returns the child context plus a release func
// that should be deferred by the caller.
func (a *App) registerRun(workspaceID string) (context.Context, func()) {
	runCtx, cancel := context.WithCancel(a.ctx)
	a.runsMu.Lock()
	a.runsGen++
	gen := a.runsGen
	if prev, ok := a.runs[workspaceID]; ok {
		prev.cancel()
	}
	a.runs[workspaceID] = runHandle{gen: gen, cancel: cancel}
	a.runsMu.Unlock()
	return runCtx, func() {
		a.runsMu.Lock()
		if cur, ok := a.runs[workspaceID]; ok && cur.gen == gen {
			delete(a.runs, workspaceID)
		}
		a.runsMu.Unlock()
		cancel()
	}
}

// StopRun cancels the in-flight run for a workspace, if any. Safe to call
// when no run is active. Triggered by the frontend stop button.
func (a *App) StopRun(workspaceID string) {
	a.runsMu.Lock()
	h, ok := a.runs[workspaceID]
	if ok {
		delete(a.runs, workspaceID)
	}
	a.runsMu.Unlock()
	if ok {
		h.cancel()
	}
}

// emitChunk emits a chat text chunk tagged with workspace_id so the
// frontend can route it to the right workspace's message buffer.
func (a *App) emitChunk(workspaceID, text string) {
	runtime.EventsEmit(a.ctx, "chat:chunk", map[string]any{
		"workspace_id": workspaceID,
		"text":         text,
	})
}

// emitDone signals the end of a run for the given workspace.
func (a *App) emitDone(workspaceID string) {
	runtime.EventsEmit(a.ctx, "chat:done", map[string]any{
		"workspace_id": workspaceID,
	})
}

// emitError reports a run failure for the given workspace.
func (a *App) emitError(workspaceID, msg string) {
	runtime.EventsEmit(a.ctx, "chat:error", map[string]any{
		"workspace_id": workspaceID,
		"message":      msg,
	})
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Install the slog tee FIRST so every subsequent slog line is
	// captured — including the ones that InitPodManager and
	// SetupTelemetry emit during their own startup. The tee's
	// shipper goroutine has a "es nil → requeue" fallback for the
	// brief window before InitPodManager runs, so it's safe to
	// install before the ES client exists.
	glitchd.InstallLogTee()
	slog.Info("glitch: log tee installed")

	// InitPodManager builds the shared ES client used by both the
	// log-tee shipper (for glitch-logs) and the telemetry span
	// exporter (for glitch-traces). Must run before SetupTelemetry
	// so the telemetry path sees a reachable ES address.
	glitchd.InitPodManager(a.ctx)

	// OTel wiring: tracer + meter providers + the elasticsearch span
	// exporter. Every pipeline run, collector tick, brain cycle, and
	// pod start from here on produces spans that land in glitch-traces.
	// Shutdown is deferred to OnShutdown so the batched exporter has a
	// chance to drain before the process dies.
	if shutdown, err := glitchd.SetupTelemetry(a.ctx, "gl1tch-desktop"); err != nil {
		slog.Warn("telemetry: setup failed, continuing without traces", "err", err)
	} else {
		a.telemetryShutdown = shutdown
		slog.Info("telemetry: wired for desktop (logs + traces → elasticsearch)")
	}

	bgCtx, cancel := context.WithCancel(context.Background())
	a.cancelBackend = cancel

	go func() {
		// SkipGlobalCollectors:true is load-bearing here. The desktop
		// owns collector lifecycle through the workspace pod manager
		// (InitPodManager + StartAllWorkspacePods below); running the
		// global supervisor's collector services in parallel makes
		// every indexed doc land in glitch-events with workspace_id=""
		// because the global path doesn't know about workspaces.
		// That bug rendered the brain popover as TOTAL INDEXED 0
		// despite hundreds of thousands of events being indexed —
		// every workspace-scoped query returned nothing.
		opts := glitchd.BackendOptions{SkipGlobalCollectors: true}
		if err := glitchd.RunBackendWithOptions(bgCtx, opts); err != nil {
			slog.Error("backend", "err", err)
		}
	}()
	go glitchd.StartAllWorkspacePods(a.ctx)
	// Tool pod runs copilot ONCE under glitchd.WorkspaceIDTools.
	// Without it copilot either doesn't run at all (it's removed
	// from per-workspace pods to avoid duplicating the same flat
	// command history under every workspace_id) or gets re-indexed
	// under every workspace. The brain popover query OR-includes
	// this bucket so the row still surfaces with its real (single)
	// count.
	go func() {
		if err := glitchd.StartToolPod(); err != nil {
			slog.Error("backend: start tool pod", "err", err)
		}
	}()

	a.startNotify()
}

func (a *App) domReady(_ context.Context) {}

func (a *App) shutdown(_ context.Context) {
	// Stop every workspace pod first so collectors exit cleanly
	// before the parent contexts get torn down. StopAll waits on
	// the goroutines, so callers downstream don't see partially-
	// torn-down state.
	glitchd.StopAllWorkspacePods()
	// Tear down the global tool pod (copilot) too.
	// Same waitgroup-on-exit semantics as workspace pods.
	_ = glitchd.StopToolPod()

	if a.notifyProc != nil {
		_ = a.notifyProc.Kill()
	}
	if a.cancelBackend != nil {
		a.cancelBackend()
	}

	// Drain the OTel BatchSpanProcessor so the last batch of spans
	// (especially the workspace.pod.stop / collector.run.exit spans
	// that the teardown above just emitted) actually reaches
	// glitch-traces instead of dying with the process. A 5s ceiling
	// keeps a wedged ES from blocking app exit forever.
	if a.telemetryShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = a.telemetryShutdown(shutdownCtx)
		cancel()
	}
}

// Ready is called by the frontend once event listeners are registered.
// brainStarted ensures the brain loop only ever runs once per app
// instance even if Ready fires more than once (HMR / reconnect).
func (a *App) Ready() {
	go a.pollStatus()
	a.brainStarted.Do(func() {
		a.brainAnchor = time.Now()
		go a.runBrainLoop()
	})
}

// ── Workspace CRUD ─────────────────────────────────────────────────────────

// CreateWorkspace creates a new workspace and returns it as JSON.
// As a side effect, the new workspace gets its own collector pod
// started so anything the user adds to it (directories, github
// repos, etc.) starts collecting immediately without an app
// restart.
func (a *App) CreateWorkspace(title string) string {
	st, err := glitchd.OpenStore()
	if err != nil {
		return "{}"
	}
	ws, err := st.CreateWorkspace(a.ctx, title, time.Now().UnixMilli())
	if err != nil {
		return "{}"
	}
	// Background pod start. Same rationale as
	// WriteWorkspaceCollectorConfig: StartWorkspacePod can block on
	// the per-workspace mutex behind another in-flight Restart, and
	// any blocking work here freezes every subsequent Wails call
	// because they're serialized by the Wails runtime. Returning the
	// new workspace immediately lets the UI render it; the pod
	// catches up in the background.
	go func(id string) {
		if perr := glitchd.StartWorkspacePod(id); perr != nil {
			slog.Error("create workspace: start pod", "err", perr)
		}
	}(ws.ID)
	b, _ := json.Marshal(ws)
	return string(b)
}

// ListWorkspaces returns all workspaces as JSON.
func (a *App) ListWorkspaces() string {
	st, err := glitchd.OpenStore()
	if err != nil {
		return "[]"
	}
	wss, err := st.ListWorkspaces(a.ctx)
	if err != nil || wss == nil {
		return "[]"
	}
	b, _ := json.Marshal(wss)
	return string(b)
}

// DeleteWorkspace removes a workspace and all its data, including
// stopping its collector pod and removing its per-workspace
// collectors.yaml from disk so the workspace leaves no residue.
func (a *App) DeleteWorkspace(id string) {
	// Stop the pod first so collector goroutines exit before we
	// delete the config they were reading from.
	if err := glitchd.StopWorkspacePod(id); err != nil {
		slog.Error("delete workspace: stop pod", "err", err)
	}
	if err := collectorDeleteWorkspaceConfigBridge(id); err != nil {
		slog.Error("delete workspace: remove config", "err", err)
	}

	st, err := glitchd.OpenStore()
	if err != nil {
		return
	}
	_ = st.DeleteWorkspace(a.ctx, id)
}

// collectorDeleteWorkspaceConfigBridge calls into the collector
// package to remove a workspace's collectors.yaml. Wrapped here so
// app.go doesn't have to import internal/collector directly (the
// rest of the file goes through pkg/glitchd).
func collectorDeleteWorkspaceConfigBridge(workspaceID string) error {
	return glitchd.DeleteWorkspaceCollectorConfig(workspaceID)
}

// UpdateWorkspaceTitle sets the title of a workspace.
func (a *App) UpdateWorkspaceTitle(id, title string) {
	st, err := glitchd.OpenStore()
	if err != nil {
		return
	}
	_ = st.UpdateWorkspaceTitle(a.ctx, id, title, time.Now().UnixMilli())
}

// AddWorkspaceDirectory opens a native picker and adds the selected
// dir to the workspace's SQLite directory list. The directories
// collector reads workspace directories at pod-start time (and the
// brain's ListCollectorsForWorkspace re-merges them on every popover
// tick), so no separate observer.yaml sync is needed — and writing
// to the global file would leak this directory across every other
// workspace.
//
// The pod is restarted so the new directory starts being scanned on
// the next tick instead of waiting for an unrelated config write.
func (a *App) AddWorkspaceDirectory(workspaceID string) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Directory to Monitor",
	})
	if err != nil || dir == "" {
		return
	}

	st, err := glitchd.OpenStore()
	if err != nil {
		return
	}
	if err := st.AddWorkspaceDirectory(a.ctx, workspaceID, dir); err != nil {
		slog.Error("add dir", "err", err)
		return
	}

	// Restart the workspace pod so the directories collector picks up
	// the new path immediately (it reads workspace.Directories at
	// start time, not on every tick).
	if err := glitchd.RestartWorkspacePod(workspaceID); err != nil {
		slog.Error("restart pod after add dir", "err", err)
	}

	a.emitBrainActivity("checkin", "info",
		"watching new directory",
		filepath.Base(dir),
		dir)
	go a.refreshCollectorActivity(false)

	ws, _ := st.GetWorkspace(a.ctx, workspaceID)
	b, _ := json.Marshal(ws)
	runtime.EventsEmit(a.ctx, "workspace:updated", string(b))
}

// RemoveWorkspaceDirectory removes a directory from a workspace. The
// observer.yaml entry is only dropped when no OTHER workspace still
// references the same path — so when two workspaces share a repo,
// removing it from one keeps the collector alive for the other.
// Collectors are keyed by path, not by workspace, so this is the
// only dedup layer we need.
// RemoveWorkspaceDirectory drops a directory from the workspace's
// SQLite list and restarts the pod so the directories collector
// stops scanning it on the next tick. Other workspaces are unaffected
// — directories are always workspace-scoped now, so a sibling
// workspace that still lists the same path keeps watching it.
func (a *App) RemoveWorkspaceDirectory(workspaceID, dir string) {
	st, err := glitchd.OpenStore()
	if err != nil {
		return
	}
	_ = st.RemoveWorkspaceDirectory(a.ctx, workspaceID, dir)

	// Restart the workspace pod so the directories collector picks
	// up the removal on the next tick.
	if err := glitchd.RestartWorkspacePod(workspaceID); err != nil {
		slog.Error("restart pod after remove dir", "err", err)
	}

	// Note for the activity panel: we used to flag "still watched by
	// another workspace" in the cross-workspace case, but now every
	// workspace has an independent directory list, so removal is
	// always local. Sibling workspaces are unaffected.
	a.emitBrainActivity("checkin", "info",
		"stopped watching directory",
		filepath.Base(dir),
		dir)
	go a.refreshCollectorActivity(false)

	ws, _ := st.GetWorkspace(a.ctx, workspaceID)
	b, _ := json.Marshal(ws)
	runtime.EventsEmit(a.ctx, "workspace:updated", string(b))
}

// ── Chat ────────────────────────────────────────────────────────────────────

// AskScoped queries the observer scoped to the workspace's directories.
func (a *App) AskScoped(prompt, workspaceID string) {
	go func() {
		runCtx, release := a.registerRun(workspaceID)
		defer release()

		// Get workspace repos for scoping
		var repos []string
		if workspaceID != "" {
			if st, err := glitchd.OpenStore(); err == nil {
				if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
					repos = ws.RepoNames
					_ = st.TouchWorkspace(a.ctx, workspaceID, time.Now().UnixMilli())
				}
			}
		}

		tokenCh := make(chan string, 64)
		go func() {
			for token := range tokenCh {
				a.emitChunk(workspaceID, token)
			}
		}()

		// Always go through the workspace-aware path so the brain
		// answer is filtered to this workspace's indexed data, even
		// when the repos slice is empty (e.g. workspace with no
		// directories yet but other collectors active).
		var err error
		if workspaceID != "" || len(repos) > 0 {
			err = glitchd.StreamAnswerScopedWorkspace(runCtx, prompt, repos, workspaceID, tokenCh)
		} else {
			err = glitchd.StreamAnswer(runCtx, prompt, tokenCh)
		}

		if err != nil {
			if runCtx.Err() != nil {
				a.emitError(workspaceID, "stopped")
			} else {
				a.emitError(workspaceID, err.Error())
			}
			return
		}

		a.emitDone(workspaceID)
	}()
}

// SaveMessage persists a chat message to the workspace.
func (a *App) SaveMessage(workspaceID, msgJSON string) {
	var msg struct {
		ID        string          `json:"id"`
		Role      string          `json:"role"`
		Blocks    json.RawMessage `json:"blocks"`
		Timestamp int64           `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(msgJSON), &msg); err != nil {
		return
	}
	_ = glitchd.SaveMessage(a.ctx, msg.ID, workspaceID, msg.Role, string(msg.Blocks), msg.Timestamp)
}

// LoadMessages returns all messages for a workspace as JSON.
func (a *App) LoadMessages(workspaceID string) string {
	st, err := glitchd.OpenStore()
	if err != nil {
		return "[]"
	}
	msgs, err := st.GetWorkspaceMessages(a.ctx, workspaceID)
	if err != nil {
		return "[]"
	}
	b, _ := json.Marshal(msgs)
	return string(b)
}

// ── Providers & Agents ──────────────────────────────────────────────────

// ListProviders returns all available providers and models as JSON.
func (a *App) ListProviders() string {
	providers := glitchd.ListProviders()
	b, _ := json.Marshal(providers)
	return string(b)
}

// ListAgents returns discovered agents/skills for the active workspace dirs.
func (a *App) ListAgents(workspaceID string) string {
	var dirs []string
	if workspaceID != "" {
		if st, err := glitchd.OpenStore(); err == nil {
			if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
				dirs = ws.Directories
			}
		}
	}
	agents := glitchd.ListAgents(dirs)
	if agents == nil {
		return "[]"
	}
	b, _ := json.Marshal(agents)
	return string(b)
}

// AskProvider sends a prompt to a chosen provider/model with full glitch context injected.
// agentPath is optional — if set, the agent's instructions are prepended.
func (a *App) AskProvider(providerID, model, prompt, workspaceID, agentPath string) {
	go func() {
		runCtx, release := a.registerRun(workspaceID)
		defer release()

		// Build context from workspace
		var dirs []string
		var agents []glitchd.AgentInfo
		var pipes []glitchd.WorkflowInfo

		if workspaceID != "" {
			if st, err := glitchd.OpenStore(); err == nil {
				if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
					dirs = ws.Directories
				}
			}
			agents = glitchd.ListAgents(dirs)
			pipes = glitchd.DiscoverWorkspaceWorkflows(dirs)
		}

		systemCtx := glitchd.BuildSystemContext(dirs, agents, pipes)

		var cwd string
		if len(dirs) > 0 {
			cwd = dirs[0]
		}

		tokenCh := make(chan string, 64)
		go func() {
			for token := range tokenCh {
				a.emitChunk(workspaceID, token)
			}
		}()

		err := glitchd.StreamPrompt(runCtx, glitchd.StreamPromptOpts{
			ProviderID: providerID,
			Model:      model,
			Prompt:     prompt,
			SystemCtx:  systemCtx,
			AgentPath:  agentPath,
			Cwd:        cwd,
		}, tokenCh)

		if err != nil {
			if runCtx.Err() != nil {
				a.emitError(workspaceID, "stopped")
			} else {
				a.emitError(workspaceID, err.Error())
			}
			return
		}

		a.emitDone(workspaceID)
	}()
}

// ── Prompts ────────────────────────────────────────────────────────────

// ListPrompts returns all saved prompts as JSON.
func (a *App) ListPrompts() string {
	return glitchd.ListAllPrompts(a.ctx)
}

// CreatePrompt saves a new prompt and returns it as JSON.
func (a *App) CreatePrompt(title, body, modelSlug string) string {
	return glitchd.CreatePrompt(a.ctx, title, body, modelSlug)
}

// DeletePrompt removes a prompt by ID.
func (a *App) DeletePrompt(id int64) {
	glitchd.DeletePromptByID(a.ctx, id)
}

// ── Drafts (refinement loop) ───────────────────────────────────────────
//
// A draft is a work-in-progress prompt/workflow/skill/agent the user is
// iterating on with gl1tch as a collaborator. The frontend popup keeps
// the editor synced with the draft body, and each chat turn calls
// RefineDraft which streams the new body back via draft:* events.
//
// Streams are keyed by draft id (not workspace id) because multiple
// drafts can be open simultaneously and each one needs its own routing.

// CreateDraft inserts a new draft and returns it as JSON. body and
// title may be empty for a brand-new draft the user will start
// refining from scratch.
func (a *App) CreateDraft(workspaceID, kind, title, body string) string {
	return glitchd.CreateDraft(a.ctx, workspaceID, kind, title, body)
}

// GetDraft returns a single draft by ID as JSON, or "{}" if missing.
func (a *App) GetDraft(id int64) string {
	return glitchd.GetDraft(a.ctx, id)
}

// ListDrafts returns all drafts for a workspace as JSON. Empty kind
// returns all kinds; non-empty filters to one of "prompt", "workflow",
// "skill", "agent".
func (a *App) ListDrafts(workspaceID, kind string) string {
	return glitchd.ListDrafts(a.ctx, workspaceID, kind)
}

// DeleteDraft removes a draft by ID. Idempotent.
func (a *App) DeleteDraft(id int64) {
	glitchd.DeleteDraft(a.ctx, id)
}

// PromoteDraft writes the draft's current body to its real target
// (prompts row for kind=prompt, file on disk for the others). For
// kind=prompt, makeGlobal=true clears the cwd scope so the prompt is
// available across all workspaces; false (the default) scopes it to
// the workspace's primary directory. Returns a JSON object with the
// resulting target_id and/or target_path.
func (a *App) PromoteDraft(id int64, makeGlobal bool) string {
	return glitchd.PromoteDraft(a.ctx, id, makeGlobal)
}

// UpdateDraftBody persists manual edits to a draft's title, body, and
// optional input/output format hints without running a refinement
// turn. The editor popup calls this before PromoteDraft so
// freshly-typed text in the CodeMirror surface gets saved alongside
// any model-refined content.
//
// inputFormat and outputFormat are optional — empty string means
// "free-form text", which is the default the popup ships with so
// authors aren't forced to declare a schema before they've even run
// the prompt once. Returns "" on success or an error message.
func (a *App) UpdateDraftBody(id int64, title, body, inputFormat, outputFormat string) string {
	return glitchd.UpdateDraftBody(a.ctx, id, title, body, inputFormat, outputFormat)
}

// RefineDraft runs one refinement turn: queries brainrag for relevant
// workspace context, asks the chosen provider to produce an improved
// draft body, streams the new body back as draft:chunk events, and
// persists the turn + new body when the stream completes.
//
// Streaming is per-draft (not per-workspace) so multiple popups can
// refine simultaneously without their tokens crossing wires. The
// generation handle is also keyed by "draft:<id>" so a stop request
// for one draft never cancels a refine on another.
//
// Empty providerID falls back to the observer default (Ollama with the
// model from observer.yaml). The desktop popup's provider picker can
// override this per-turn for "give me a power answer" without sticking.
func (a *App) RefineDraft(draftID int64, userTurn, providerID, model string) {
	go func() {
		runKey := fmt.Sprintf("draft:%d", draftID)
		runCtx, release := a.registerRun(runKey)
		defer release()

		tokenCh := make(chan string, 64)
		go func() {
			for chunk := range tokenCh {
				a.emitDraftChunk(draftID, chunk)
			}
		}()

		err := glitchd.RefineDraft(runCtx, glitchd.RefineDraftOpts{
			DraftID:    draftID,
			UserTurn:   userTurn,
			ProviderID: providerID,
			Model:      model,
		}, tokenCh)
		if err != nil {
			if runCtx.Err() != nil {
				a.emitDraftError(draftID, "stopped")
			} else {
				a.emitDraftError(draftID, err.Error())
			}
			return
		}
		a.emitDraftDone(draftID)
	}()
}

// StopDraftRefine cancels an in-flight refinement for the given draft.
// Safe to call when nothing is running.
func (a *App) StopDraftRefine(draftID int64) {
	a.StopRun(fmt.Sprintf("draft:%d", draftID))
}

// emitDraftChunk forwards a streaming chunk to the frontend. The
// payload mirrors chat:chunk's shape (id + text) so the frontend's
// existing block-stream consumers can be reused with minimal forking.
func (a *App) emitDraftChunk(draftID int64, text string) {
	runtime.EventsEmit(a.ctx, "draft:chunk", map[string]any{
		"draft_id": draftID,
		"text":     text,
	})
}

// emitDraftDone signals successful completion of a refinement turn.
// The frontend should re-fetch the draft after this to get the
// canonical persisted body and updated turn history.
func (a *App) emitDraftDone(draftID int64) {
	runtime.EventsEmit(a.ctx, "draft:done", map[string]any{
		"draft_id": draftID,
	})
}

// emitDraftError reports a refinement failure. The popup should
// surface the message inline (the partial body, if any, has already
// been persisted by RefineDraft so the user can keep iterating).
func (a *App) emitDraftError(draftID int64, msg string) {
	runtime.EventsEmit(a.ctx, "draft:error", map[string]any{
		"draft_id": draftID,
		"message":  msg,
	})
}

// GetWorkflowFileDetails returns metadata about a workflow YAML file on disk:
// description and the list of inner steps with their executor and a short
// prompt preview. Used by the step editor in the desktop builder so users
// can see what a workflow does without leaving the chat.
func (a *App) GetWorkflowFileDetails(path string) string {
	return glitchd.GetWorkflowFileDetails(path)
}

// ── Chain execution ─────────────────────────────────────────────────────

// RunChain executes a builder chain (JSON-encoded list of ChainStep) sequentially.
// Each step's output flows into the next via {{ steps.step-N.value }} refs.
// userText is appended as a final implicit prompt step if non-empty.
func (a *App) RunChain(stepsJSON, userText, workspaceID, defaultProvider, defaultModel string) {
	go func() {
		runCtx, release := a.registerRun(workspaceID)
		defer release()

		// Build system context from workspace.
		var dirs []string
		var agents []glitchd.AgentInfo
		var pipes []glitchd.WorkflowInfo
		if workspaceID != "" {
			if st, err := glitchd.OpenStore(); err == nil {
				if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
					dirs = ws.Directories
				}
			}
			agents = glitchd.ListAgents(dirs)
			pipes = glitchd.DiscoverWorkspaceWorkflows(dirs)
		}
		systemCtx := glitchd.BuildSystemContext(dirs, agents, pipes)

		// Start clarification poller for the duration of the run. The poller
		// is scoped to runCtx so it dies on stop along with everything else.
		clarifyCtx, clarifyCancel := context.WithCancel(runCtx)
		go a.pollClarifications(clarifyCtx, workspaceID)

		// Structured block events from the protocol splitter. Each event
		// becomes a single Wails message, so the frontend can build typed
		// blocks (notes, tables, status) incrementally as bytes arrive.
		eventCh := make(chan glitchd.BlockEvent, 64)
		go func() {
			for ev := range eventCh {
				runtime.EventsEmit(a.ctx, "chat:event", encodeBlockEvent(workspaceID, ev))
			}
		}()

		// Primary workspace directory → step cwd. Without this, shell steps
		// run from glitch-desktop's own cwd (usually the gl1tch repo), which
		// is why a workflow launched from a different workspace would
		// otherwise read files out of gl1tch instead of the intended project.
		var cwd string
		if len(dirs) > 0 {
			cwd = dirs[0]
		}

		err := glitchd.RunChain(runCtx, glitchd.RunChainOpts{
			StepsJSON:       stepsJSON,
			UserText:        userText,
			WorkspaceID:     workspaceID,
			DefaultProvider: defaultProvider,
			DefaultModel:    defaultModel,
			SystemCtx:       systemCtx,
			Cwd:             cwd,
			EventCh:         eventCh,
		}, nil)
		clarifyCancel()

		if err != nil {
			if runCtx.Err() != nil {
				a.emitError(workspaceID, "stopped")
			} else {
				a.emitError(workspaceID, err.Error())
			}
			return
		}
		a.emitDone(workspaceID)
	}()
}

// ── Workflows (file-backed, single source of truth) ──────────────────────
//
// Workflows are .workflow.yaml files under
// <workspace>/.glitch/workflows/. The legacy chat_workflows SQLite
// table is migrated to disk on startup (see MigrateChatWorkflowsToYAML)
// and the desktop only ever talks to files. This collapses the previous
// "two ways to save a workflow" friction into one path.

// SaveChatWorkflow saves the current chain bar contents as a
// .workflow.yaml file under the workspace's primary directory.
//
// defaultProvider/defaultModel are baked into prompt steps that don't
// have their own override, locking in the picker state at save time.
// Errors (no provider, no workspace dir, etc.) come back as a JSON
// {error: "..."} payload so the frontend can render them inline.
//
// The method name is preserved (rather than renamed to e.g.
// SaveWorkflowFile) so the frontend's existing call site keeps
// working — only the signature gains the provider/model args.
func (a *App) SaveChatWorkflow(workspaceID, name, stepsJSON, defaultProvider, defaultModel string) string {
	return glitchd.SaveChainAsWorkflow(a.ctx, workspaceID, name, "", stepsJSON, defaultProvider, defaultModel)
}

// ── Step-through authoring sessions ───────────────────────────────────────
//
// Step-through mode runs a pipeline serially and pauses between each step so
// the user can inspect the output, optionally hand-edit it, and continue or
// abort. The goal is to bridge chat exploration → reusable workflow file
// without forcing users to eyeball raw YAML as they go: they drive the run
// from the desktop UI, then hit save-as to crystallize what they kept.
//
// Lifecycle from the frontend's POV:
//
//  1. StepThroughStart(workspaceID, yaml, userInput) → JSON {session_id} or {error}
//  2. Subscribe to runtime event "step-through:event" (payload is a
//     StepThroughEvent carrying kind = step_paused | step_committed |
//     final | error). Events are tagged with session_id so multiple
//     concurrent sessions don't cross-wire.
//  3. On a step_paused event, call StepThroughAccept(sessionID) or
//     StepThroughEditOutput(sessionID, edited) to unblock the runner.
//  4. On a final event, optionally call StepThroughSave(sessionID, name)
//     to persist the (unmodified) pipeline YAML to
//     <workspace>/.glitch/workflows/<name>.workflow.yaml. The router's
//     on-demand discover+embed path picks up the new file on the next
//     ask call — no explicit reindex is required.
//  5. At any point, StepThroughAbort(sessionID) tears the run down.

// StepThroughStartFromChain is the chat-bar entry point into step-through
// mode. It parallels RunChain, but instead of running the chain to
// completion it starts a paused step-through session and forwards per-step
// pause events to the frontend as "step-through:event" wails events.
//
// Per the "step-through isn't a mode" rule (see project_step_through_mode):
// the frontend routes chain.length >= 2 sends through this path. Single-
// step sends keep going through RunChain / AskScoped and never pause.
//
// Lifecycle: chat:chunk and chat:event stream from the running step the
// same way RunChain streams them, so the chat surface renders step output
// in real time. On step_paused, the frontend shows an Accept/Abort
// affordance; on final the session terminates and chat:done fires.
func (a *App) StepThroughStartFromChain(
	stepsJSON, userText, workspaceID, defaultProvider, defaultModel string,
) string {
	// Build system context from workspace — mirrors RunChain so chain
	// steps see the same agents/workflows/dirs regardless of which path
	// the chat bar took.
	var dirs []string
	var agents []glitchd.AgentInfo
	var pipes []glitchd.WorkflowInfo
	if workspaceID != "" {
		if st, err := glitchd.OpenStore(); err == nil {
			if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
				dirs = ws.Directories
			}
		}
		agents = glitchd.ListAgents(dirs)
		pipes = glitchd.DiscoverWorkspaceWorkflows(dirs)
	}
	systemCtx := glitchd.BuildSystemContext(dirs, agents, pipes)
	var cwd string
	if len(dirs) > 0 {
		cwd = dirs[0]
	}

	// Register a run slot so StopRun cancels the session the same way it
	// cancels a RunChain. The release closure fires when the pump goroutine
	// observes the session's Events channel close.
	runCtx, release := a.registerRun(workspaceID)

	// Block-event pump — matches RunChain, which deliberately runs
	// EventCh-only (no TokenCh). The StreamSplitter consumes
	// <<GLITCH_TEXT>> markers and gl1tch-stats trailers and emits clean
	// BlockEvents. Wiring TokenCh in parallel would re-emit the raw bytes
	// verbatim and the chat surface would render the protocol markers
	// alongside the parsed text — exactly the leak the screenshot showed.
	eventCh := make(chan glitchd.BlockEvent, 64)
	go func() {
		for ev := range eventCh {
			runtime.EventsEmit(a.ctx, "chat:event", encodeBlockEvent(workspaceID, ev))
		}
	}()

	h, err := glitchd.StartStepThroughFromChain(runCtx, glitchd.StepThroughChainOpts{
		WorkspaceID:     workspaceID,
		StepsJSON:       stepsJSON,
		UserText:        userText,
		DefaultProvider: defaultProvider,
		DefaultModel:    defaultModel,
		SystemCtx:       systemCtx,
		Cwd:             cwd,
		EventCh:         eventCh,
	})
	if err != nil {
		close(eventCh)
		release()
		a.emitError(workspaceID, err.Error())
		return errorJSONStr(err.Error())
	}

	a.stepThroughMu.Lock()
	a.stepThrough[h.ID] = h
	a.stepThroughMu.Unlock()

	// Pump session events → "step-through:event". Tag each event with
	// workspace_id so the frontend can route it (matches chat:chunk
	// semantics). On terminal events (final/error) we also emit the
	// chat:done / chat:error pair so the existing streaming-state
	// machinery in the React store flips out of "streaming" naturally.
	go func(id, wsID string, events <-chan glitchd.StepThroughEvent) {
		defer release()
		defer close(eventCh)
		for ev := range events {
			payload := map[string]any{
				"workspace_id": wsID,
				"kind":         ev.Kind,
				"session_id":   ev.SessionID,
				"step_id":      ev.StepID,
				"output":       ev.Output,
				"step_index":   ev.StepIndex,
				"step_total":   ev.StepTotal,
				"final_output": ev.FinalOutput,
				"error":        ev.Error,
			}
			runtime.EventsEmit(a.ctx, "step-through:event", payload)
			switch ev.Kind {
			case "final":
				a.emitDone(wsID)
			case "error":
				a.emitError(wsID, ev.Error)
			}
		}
		a.stepThroughMu.Lock()
		delete(a.stepThrough, id)
		a.stepThroughMu.Unlock()
	}(h.ID, workspaceID, h.Session.Events)

	out, _ := json.Marshal(map[string]string{"session_id": h.ID})
	return string(out)
}

// StepThroughStart parses the supplied pipeline YAML, starts a step-through
// session under the given workspace, and spawns a pump goroutine that
// forwards every session event to the "step-through:event" Wails runtime
// topic. Returns a JSON {session_id: "..."} or {error: "..."} payload.
func (a *App) StepThroughStart(workspaceID, yamlContent, userInput string) string {
	h, err := glitchd.StartStepThrough(a.ctx, workspaceID, yamlContent, userInput)
	if err != nil {
		return errorJSONStr(err.Error())
	}

	a.stepThroughMu.Lock()
	a.stepThrough[h.ID] = h
	a.stepThroughMu.Unlock()

	// Pump session events → Wails runtime events. When the session's
	// Events channel closes, we drop the handle from the session map.
	go func(id string, events <-chan glitchd.StepThroughEvent) {
		for ev := range events {
			runtime.EventsEmit(a.ctx, "step-through:event", ev)
		}
		a.stepThroughMu.Lock()
		delete(a.stepThrough, id)
		a.stepThroughMu.Unlock()
	}(h.ID, h.Session.Events)

	out, _ := json.Marshal(map[string]string{"session_id": h.ID})
	return string(out)
}

// StepThroughAccept resumes a paused session with the current (unmodified)
// step output. Returns "" on success or an error message.
func (a *App) StepThroughAccept(sessionID string) string {
	h := a.lookupStepThrough(sessionID)
	if h == nil {
		return "session not found"
	}
	if err := h.Session.Accept(); err != nil {
		return err.Error()
	}
	return ""
}

// StepThroughEditOutput resumes a paused session after replacing the current
// step's output "value" with editedValue. The edit is recorded in the
// session's provenance map and surfaced via StepThroughSnapshot.
func (a *App) StepThroughEditOutput(sessionID, editedValue string) string {
	h := a.lookupStepThrough(sessionID)
	if h == nil {
		return "session not found"
	}
	if err := h.Session.EditOutput(editedValue); err != nil {
		return err.Error()
	}
	return ""
}

// StepThroughAbort cancels a running session. Pending interceptor pauses
// unblock with an abort decision and the runner context is cancelled. The
// pump goroutine removes the handle from the session map once the Events
// channel closes.
func (a *App) StepThroughAbort(sessionID string) string {
	h := a.lookupStepThrough(sessionID)
	if h == nil {
		return "session not found"
	}
	if err := h.Session.Abort(); err != nil {
		return err.Error()
	}
	return ""
}

// StepThroughSnapshot returns the current session state as JSON. Useful for
// reconnecting a frontend mid-session after an HMR or tab switch — the
// frontend can re-render the paused output and edited-step markers without
// replaying the event stream.
func (a *App) StepThroughSnapshot(sessionID string) string {
	h := a.lookupStepThrough(sessionID)
	if h == nil {
		return errorJSONStr("session not found")
	}
	snap := h.Session.Snapshot()
	out, _ := json.Marshal(snap)
	return string(out)
}

// StepThroughSave writes the session's original pipeline YAML to the
// workspace's .glitch/workflows/<name>.workflow.yaml file. The router
// auto-indexes new workflows on the next ask call (hash-based embedding
// cache in internal/router/embed.go), so the saved workflow becomes
// matchable immediately without an explicit reindex step.
func (a *App) StepThroughSave(sessionID, name string) string {
	h := a.lookupStepThrough(sessionID)
	if h == nil {
		return errorJSONStr("session not found")
	}
	path, err := glitchd.SaveStepThroughAs(a.ctx, h, name)
	if err != nil {
		return errorJSONStr(err.Error())
	}
	out, _ := json.Marshal(map[string]string{"path": path, "name": name})
	return string(out)
}

// lookupStepThrough fetches the handle for a session ID under the session
// mutex. Returns nil when the session is unknown (typically because it has
// already terminated and been cleaned up by the pump goroutine).
func (a *App) lookupStepThrough(sessionID string) *glitchd.StepThroughHandle {
	a.stepThroughMu.Lock()
	defer a.stepThroughMu.Unlock()
	return a.stepThrough[sessionID]
}

// errorJSONStr returns a canned `{"error": "..."}` JSON payload the
// frontend can surface inline. Kept narrow (string → string) so
// Wails-generated bindings stay ergonomic.
func errorJSONStr(msg string) string {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return string(b)
}

// DeleteWorkflowFile removes a workflow YAML file by absolute path.
// Path must live under a .glitch/workflows directory and end in
// .workflow.yaml — anything else is rejected. Returns an empty string
// on success or the error message on failure.
func (a *App) DeleteWorkflowFile(path string) string {
	return glitchd.DeleteWorkflowFile(path)
}

// ReadWorkflowFile returns the raw YAML contents of a workflow file
// as JSON {content: "..."} or {error: "..."}. Used by the editor
// popup to seed CodeMirror when opening an existing workflow.
func (a *App) ReadWorkflowFile(path string) string {
	return glitchd.ReadWorkflowFile(path)
}

// WriteWorkflowFile overwrites a workflow YAML file with new content.
// Returns "" on success or an error message. Used as a fallback
// "save without going through draft promote" path; the canonical
// editor save uses PromoteDraft.
func (a *App) WriteWorkflowFile(path, content string) string {
	return glitchd.WriteWorkflowFile(path, content)
}

// CreateDraftFromTarget creates a draft seeded from an existing
// entity. kind ∈ {"prompt","workflow"}. For prompt, targetID is the
// row id. For workflow, targetPath is the absolute file path.
//
// Used by the sidebar's "edit" button — clicking it on a workflow or
// prompt row creates a draft pointed at the source entity, opens the
// popup, and lets the user iterate. PromoteDraft writes the draft
// body back to the source on save.
func (a *App) CreateDraftFromTarget(workspaceID, kind string, targetID int64, targetPath string) string {
	return glitchd.CreateDraftFromTarget(a.ctx, workspaceID, kind, targetID, targetPath)
}

// WorkflowPathForName returns the absolute path where a new workflow
// with the given name will be saved for the active workspace. The
// editor popup uses this to show "saves to <path>" as the user types
// a name for a brand-new workflow draft.
func (a *App) WorkflowPathForName(workspaceID, name string) string {
	return glitchd.WorkflowPathForName(a.ctx, workspaceID, name)
}

// ReadSkillOrAgentFile returns the markdown contents of a skill or
// agent file as JSON {content: "..."} or {error: "..."}. The path
// must live under one of the recognized skill/agent locations
// (workspace .claude/, global ~/.claude/, etc.). Used by the editor
// popup as a fallback when CreateDraftFromTarget hasn't already
// loaded the body.
func (a *App) ReadSkillOrAgentFile(path string) string {
	return glitchd.ReadSkillOrAgentFile(path)
}

// SkillPathForName returns where a new workspace skill with the
// given name would be written. Used as a preview in the editor
// popup before the user commits to save.
func (a *App) SkillPathForName(workspaceID, name string) string {
	return glitchd.SkillPathForName(a.ctx, workspaceID, name)
}

// AgentPathForName returns where a new workspace agent with the
// given name would be written.
func (a *App) AgentPathForName(workspaceID, name string) string {
	return glitchd.AgentPathForName(a.ctx, workspaceID, name)
}

// PromoteDraftAs is the "save as new" path. Detaches the draft from
// its current target (so global entities don't get overwritten),
// renames it to newName, and promotes. Always lands in the
// workspace. Returns the same JSON shape as PromoteDraft.
func (a *App) PromoteDraftAs(id int64, newName string) string {
	return glitchd.PromoteDraftAs(a.ctx, id, newName)
}

// ── Clarification ──────────────────────────────────────────────────────

// AnswerClarification writes the user's answer for a pending clarification.
func (a *App) AnswerClarification(runID, answer string) {
	glitchd.AnswerClarification(runID, answer)
}

// pollClarifications polls the DB for pending clarification requests during
// pipeline runs and forwards them to the frontend as Wails events. The
// workspaceID is included on every emitted event so the frontend can route
// the prompt to the correct workspace's chat surface.
func (a *App) pollClarifications(ctx context.Context, workspaceID string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	notified := map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reqs, err := glitchd.LoadPendingClarifications()
			if err != nil {
				continue
			}
			for _, req := range reqs {
				if notified[req.RunID] {
					continue
				}
				notified[req.RunID] = true
				runtime.EventsEmit(a.ctx, "chat:clarify", map[string]string{
					"workspace_id": workspaceID,
					"run_id":       req.RunID,
					"step_id":      req.StepID,
					"question":     req.Question,
				})
			}
		}
	}
}

// ── Workflows ───────────────────────────────────────────────────────────

// ListWorkflows returns discovered workflows from the active workspace's directories.
func (a *App) ListWorkflows(workspaceID string) string {
	var dirs []string
	if workspaceID != "" {
		if st, err := glitchd.OpenStore(); err == nil {
			if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
				dirs = ws.Directories
			}
		}
	}
	workflows := glitchd.DiscoverWorkspaceWorkflows(dirs)
	if workflows == nil {
		return "[]"
	}
	b, _ := json.Marshal(workflows)
	return string(b)
}

// RunWorkflow executes a workflow and streams output as chat events.
func (a *App) RunWorkflow(workflowPath, input, workspaceID string) {
	go func() {
		runCtx, release := a.registerRun(workspaceID)
		defer release()

		// Start polling for clarification requests during this workflow run.
		clarifyCtx, clarifyCancel := context.WithCancel(runCtx)
		go a.pollClarifications(clarifyCtx, workspaceID)

		tokenCh := make(chan string, 64)
		go func() {
			for token := range tokenCh {
				a.emitChunk(workspaceID, token)
			}
		}()

		err := glitchd.RunWorkflow(runCtx, workflowPath, input, tokenCh)
		clarifyCancel()

		if err != nil {
			if runCtx.Err() != nil {
				a.emitError(workspaceID, "stopped")
			} else {
				a.emitError(workspaceID, err.Error())
			}
			return
		}
		a.emitDone(workspaceID)
	}()
}

// SaveWorkflow saves workflow YAML to a workspace directory.
func (a *App) SaveWorkflow(workspaceDir, name, yamlContent string) string {
	path, err := glitchd.SaveWorkflow(workspaceDir, name, yamlContent)
	if err != nil {
		return ""
	}
	return path
}

// Doctor runs health checks and streams the report. Tagged to a workspace
// so the report lands in the workspace the user invoked it from.
func (a *App) Doctor(workspaceID string) {
	go func() {
		runCtx, release := a.registerRun(workspaceID)
		defer release()

		checks := glitchd.Doctor(runCtx)
		report := glitchd.DoctorReport(checks)
		a.emitChunk(workspaceID, report)
		a.emitDone(workspaceID)
	}()
}

// ── Status polling ──────────────────────────────────────────────────────────

func (a *App) pollStatus() {
	check := func() {
		runtime.EventsEmit(a.ctx, "status:all", map[string]bool{
			"ollama":        pingHTTP("http://localhost:11434"),
			"elasticsearch": pingHTTP("http://localhost:9200"),
			"busd":          pingUnix(busdSocket()),
		})
	}
	check()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

// ── Collectors ──────────────────────────────────────────────────────────────

// ListCollectors returns the set of configured collectors as JSON for
// the brain popover. workspaceID scopes the view: directories come
// from that workspace's SQLite row (not observer.yaml) and git/github
// are auto-derived from those directories. Each entry is decorated
// with real activity from Elasticsearch (total_docs, last_seen_ms) so
// the UI can show what's actually been indexed instead of just a
// derived "next in" countdown.
//
// An empty workspaceID falls back to the observer.yaml view so the
// popover still renders during startup before any workspace is
// active.
//
// The brain anchor is also returned so the frontend can fall back to
// the countdown when ES has nothing for that collector yet.
func (a *App) ListCollectors(workspaceID string) string {
	cols, err := glitchd.ListCollectorsForWorkspace(a.ctx, workspaceID)
	if err != nil {
		cols = nil
	}
	if cols == nil {
		cols = []glitchd.CollectorInfo{}
	}

	// Pick the activity snapshot to merge in. Two paths:
	//
	//   workspaceID == ""  → use the cached global snapshot maintained
	//                        by refreshCollectorActivity. Same behavior
	//                        as before; supports the headless / pre-
	//                        workspace startup window.
	//
	//   workspaceID != ""  → query ES inline with a workspace_id term
	//                        filter so the popover shows ONLY this
	//                        workspace's activity. The cached snapshot
	//                        is global so it can't answer this on its
	//                        own. The aggregation is one query against
	//                        a keyword field — cheap enough to run on
	//                        every popover open without caching.
	snapshot := make(map[string]glitchd.CollectorActivity, 8)
	if workspaceID == "" {
		a.collectorMu.Lock()
		for k, v := range a.collectorState {
			snapshot[k] = v
		}
		a.collectorMu.Unlock()
	} else {
		queryCtx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
		defer cancel()
		if rows, err := glitchd.QueryCollectorActivityScoped(queryCtx, workspaceID); err == nil {
			for _, r := range rows {
				snapshot[r.Source] = r
			}
		}
		// Code-index docs live in glitch-vectors, NOT glitch-events,
		// so the source aggregation above can never find them. Look
		// up the workspace's directories from SQLite, translate them
		// into the brainrag "cwd:<abs>" scopes the collector writes
		// under, and count the chunks in glitch-vectors directly so
		// the popover row shows real numbers instead of the
		// always-zero you'd get from the events index.
		if st, err := glitchd.OpenStore(); err == nil {
			if ws, err := st.GetWorkspace(queryCtx, workspaceID); err == nil && len(ws.Directories) > 0 {
				if act, err := glitchd.QueryCodeIndexActivityScoped(queryCtx, ws.Directories); err == nil && act.TotalDocs > 0 {
					snapshot["code-index"] = act
				}
			}
		}
	}

	// Out type carries the static config, the ES doc-count snapshot,
	// AND the in-process collector run heartbeat in a single flat
	// shape so the frontend doesn't have to do a join.
	//
	//   total_docs / last_seen_ms  → from ES (what's actually indexed)
	//   last_run_ms / last_run_*   → from the in-process registry
	//                                (proves the collector cycled)
	type out struct {
		glitchd.CollectorInfo
		TotalDocs        int64  `json:"total_docs"`
		LastSeenMs       int64  `json:"last_seen_ms,omitempty"`
		LastRunMs        int64  `json:"last_run_ms,omitempty"`
		LastRunIndexed   int    `json:"last_run_indexed,omitempty"`
		LastRunDurationMs int64 `json:"last_run_duration_ms,omitempty"`
		LastRunError     string `json:"last_run_error,omitempty"`
	}
	runs := glitchd.CollectorRuns()
	merged := make([]out, 0, len(cols))
	for _, c := range cols {
		row := out{CollectorInfo: c}
		if act, ok := snapshot[c.Name]; ok {
			row.TotalDocs = act.TotalDocs
			row.LastSeenMs = act.LastSeenMs
		}
		if r, ok := runs[c.Name]; ok {
			row.LastRunMs = r.AtMs
			row.LastRunIndexed = r.IndexedCount
			row.LastRunDurationMs = r.DurationMs
			row.LastRunError = r.Error
		}
		merged = append(merged, row)
	}

	b, _ := json.Marshal(map[string]any{
		"anchor_ms":  a.brainAnchor.UnixMilli(),
		"now_ms":     time.Now().UnixMilli(),
		"collectors": merged,
	})
	return string(b)
}

// BrainDecisions returns a JSON snapshot of brain-decision activity
// for the given workspace, used by the DECISIONS section of the
// brain popover. Empty workspaceID falls back to the global view so
// the popover renders during startup before a workspace is active.
//
// Output shape mirrors glitchd.BrainDecisionsActivity:
//
//	{
//	  "total": 12,
//	  "escalated": 3,
//	  "last_decision_ms": 1712499600000,
//	  "last_provider": "ollama",
//	  "last_escalated": false
//	}
//
// Returns "{}" on any error so the popover renders an empty section
// instead of an error toast — the brain decision log is informational,
// never load-bearing for the rest of the UI.
func (a *App) BrainDecisions(workspaceID string) string {
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
	defer cancel()
	act, err := glitchd.QueryBrainDecisionsActivityScoped(ctx, workspaceID)
	if err != nil || act == nil {
		return "{}"
	}
	b, _ := json.Marshal(act)
	return string(b)
}

// CollectorsConfigPath returns the absolute path to observer.yaml.
// CollectorsConfigPath returns the absolute path of a collectors
// config file. When workspaceID is non-empty, returns the path of
// that workspace's per-workspace collectors.yaml. When empty,
// returns the global ~/.config/glitch/observer.yaml path (the
// force-overwrite escape hatch — global is read-only by default
// in the editor popup).
func (a *App) CollectorsConfigPath(workspaceID string) string {
	if workspaceID != "" {
		p, err := glitchd.WorkspaceCollectorConfigPath(workspaceID)
		if err != nil {
			return ""
		}
		return p
	}
	p, err := glitchd.CollectorConfigPath()
	if err != nil {
		return ""
	}
	return p
}

// ReadCollectorsConfig returns the raw YAML contents of a collectors
// config file. workspaceID="" reads the global observer.yaml; non-
// empty reads the per-workspace file. Both auto-create a starter
// file from defaults if none exists, so the editor always opens to
// a useful starting point.
func (a *App) ReadCollectorsConfig(workspaceID string) string {
	if workspaceID != "" {
		s, err := glitchd.ReadWorkspaceCollectorConfig(workspaceID)
		if err != nil {
			return ""
		}
		return s
	}
	s, err := glitchd.ReadCollectorConfig()
	if err != nil {
		return ""
	}
	return s
}

// WriteCollectorsConfig validates and saves collectors config content.
// workspaceID="" writes the global observer.yaml (force-overwrite
// path); non-empty writes the per-workspace file AND restarts that
// workspace's collector pod so the new config takes effect immediately.
//
// Returns "" on success, or the validation/IO error message so the
// editor popup can render it inline.
func (a *App) WriteCollectorsConfig(workspaceID, content string) string {
	if workspaceID != "" {
		if err := glitchd.WriteWorkspaceCollectorConfig(workspaceID, content); err != nil {
			return err.Error()
		}
		go a.refreshCollectorActivity(false)
		return ""
	}
	if err := glitchd.WriteCollectorConfig(content); err != nil {
		return err.Error()
	}
	go a.refreshCollectorActivity(false)
	return ""
}

// GetCollectorsConfigJSON returns the workspace's collectors config as
// a JSON object whose shape mirrors the typed collector.Config struct.
// Used by the schema-driven config modal so it can render structured
// fields without parsing YAML in the renderer process.
//
// workspaceID="" is not supported here — the structured modal is
// always per-workspace; global edits keep using the raw YAML editor.
func (a *App) GetCollectorsConfigJSON(workspaceID string) string {
	if workspaceID == "" {
		return ""
	}
	s, err := glitchd.ReadWorkspaceCollectorConfigJSON(workspaceID)
	if err != nil {
		return ""
	}
	return s
}

// WriteCollectorsConfigJSON parses jsonContent into the typed Config,
// re-marshals to YAML, and saves via the same validated writer the
// raw YAML editor uses. Triggers a pod restart on success.
//
// Returns "" on success or the parse/validation error string so the
// modal can render it inline.
func (a *App) WriteCollectorsConfigJSON(workspaceID, jsonContent string) string {
	if workspaceID == "" {
		return "workspace id is required"
	}
	if err := glitchd.WriteWorkspaceCollectorConfigJSON(workspaceID, jsonContent); err != nil {
		return err.Error()
	}
	go a.refreshCollectorActivity(false)
	return ""
}

// RecentCollectorLogs returns the last `limit` log entries from the
// glitch-logs Elasticsearch index as JSON, newest first. This is the
// read path for the brain popover's live logs panel.
//
// Records land in ES via the teeHandler's 2s ship queue
// (InstallLogTee → runLogShipper), so there is a brief lag before a
// freshly emitted log appears here — acceptable given the popover
// polls every 1-5s. ES is the authoritative source: logs survive
// restarts and are queryable across processes.
//
// Returns "[]" when ES is unavailable so the frontend always gets
// valid JSON it can safely parse.
func (a *App) RecentCollectorLogs(limit int) string {
	entries, err := glitchd.QueryRecentLogs(glitchd.EsClient(), limit)
	if err != nil || entries == nil {
		entries = []glitchd.LogEntry{}
	}
	b, _ := json.Marshal(entries)
	return string(b)
}

// ── Brain loop ──────────────────────────────────────────────────────────────
//
// The brain loop is the single ambient surface in the UI. It owns three
// jobs:
//
//  1. Publish a live brain state ("idle" | "collecting" | "analyzing" |
//     "alert" | "error") that drives the persistent brain icon.
//  2. Drop low-noise periodic check-ins into the in-app activity panel
//     so the user can see the brain is alive.
//  3. Provide a single entry point (emitBrainActivity) that future
//     collectors / triage code can call to surface findings, both into
//     the in-app activity stream and (for warn+ severity) the systray.
//
// The full local-Ollama triage loop — buffering collected items and
// running them through a triage prompt — is intentionally not wired up
// here yet; that lands once the Phase 1 collector pipeline starts
// emitting items we can subscribe to. The protocol on the wire is
// already in place so the UI doesn't have to change when it does.

// brainStateIdle and friends are the wire values the frontend's
// BrainState union understands. Keep these in sync with
// glitch-desktop/frontend/src/lib/types.ts.
const (
	brainStateIdle       = "idle"
	brainStateCollecting = "collecting"
	brainStateAnalyzing  = "analyzing"
	brainStateError      = "error"
)

// emitBrainStatus publishes the current brain state + a short detail
// string. The frontend treats brain:status as the source of truth for
// the indicator's visual state.
func (a *App) emitBrainStatus(state, detail string) {
	runtime.EventsEmit(a.ctx, "brain:status", map[string]string{
		"state":  state,
		"detail": detail,
	})
}

// emitBrainActivity surfaces a single entry into the in-app activity
// panel and (for warn/error severity) the systray via busd. kind is
// "alert" (something the user should look at) or "checkin" (low-noise
// status).
func (a *App) emitBrainActivity(kind, severity, title, detail, source string) {
	now := time.Now()
	payload := map[string]any{
		"id":        now.UnixNano(),
		"kind":      kind,
		"severity":  severity,
		"title":     title,
		"detail":    detail,
		"source":    source,
		"timestamp": now.UnixMilli(),
	}
	runtime.EventsEmit(a.ctx, "brain:activity", payload)

	// Alerts (severity warn/error) also fan out to glitch-notify via
	// busd so the user gets a macOS notification. Check-ins stay
	// in-app so we don't spam the systray.
	if kind == "alert" && (severity == "warn" || severity == "error") {
		_ = glitchd.PublishBusEvent(glitchd.BrainAlertTopic, map[string]any{
			"title":    title,
			"subtitle": detail,
			"severity": severity,
			"source":   source,
		})
	}
}

// runBrainLoop drives the ambient brain indicator. It owns four
// tickers:
//
//   - stateTick:     republish brain:status so the icon stays accurate
//   - collectorTick: poll Elasticsearch for per-source doc counts and
//                    emit "got N new items in <source>" deltas
//   - triageTick:    feed recent events into the local Ollama model
//                    for triage; emit alerts the model raises
//   - checkinTick:   periodic "I'm here" pulse so the user can see the
//                    brain is alive even when collectors are quiet
func (a *App) runBrainLoop() {
	const (
		stateInterval     = 4 * time.Second
		collectorInterval = 15 * time.Second
		triageInterval    = 2 * time.Minute
		checkinInterval   = 5 * time.Minute
	)

	stateTick := time.NewTicker(stateInterval)
	defer stateTick.Stop()
	collectorTick := time.NewTicker(collectorInterval)
	defer collectorTick.Stop()
	triageTick := time.NewTicker(triageInterval)
	defer triageTick.Stop()
	checkinTick := time.NewTicker(checkinInterval)
	defer checkinTick.Stop()

	// Seed the triage high-water mark to "now" so the first tick
	// doesn't try to triage every event in history.
	a.triageMu.Lock()
	a.triageLastMs = time.Now().UnixMilli()
	a.triageMu.Unlock()

	// Initial check-in so the activity panel isn't empty on first launch.
	time.AfterFunc(2*time.Second, func() {
		a.emitBrainActivity("checkin", "info", "brain online",
			"watching for activity in your workspaces", "")
	})

	// Prime the collector snapshot so the first real poll can compute
	// a delta. Run immediately on startup so the popover has data.
	go a.refreshCollectorActivity(true)

	var lastState string
	publish := func() {
		state, detail := a.deriveBrainState()
		if state == lastState {
			// Still emit on detail change so the tooltip stays fresh.
			a.emitBrainStatus(state, detail)
			return
		}
		lastState = state
		a.emitBrainStatus(state, detail)
	}
	publish()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-stateTick.C:
			publish()
		case <-collectorTick.C:
			a.refreshCollectorActivity(false)
		case <-triageTick.C:
			// Triage runs in its own goroutine so a slow Ollama
			// response can't stall the rest of the brain loop.
			go a.runTriageOnce()
		case <-checkinTick.C:
			// Periodic check-in. We only emit a brain activity row
			// when there's something genuinely actionable — i.e. an
			// error the user needs to know about. Idle/collecting/
			// analyzing states are already shown live in the brain
			// indicator's color + the collectors panel; spamming
			// "watching · no new activity" rows on top of that just
			// fills the activity log with noise. The brain status
			// publish loop above keeps the live indicator fresh.
			if state, _ := a.deriveBrainState(); state == brainStateError {
				a.emitBrainActivity("alert", "warn", "brain offline",
					"local model unreachable — start ollama to re-enable triage", "")
			}
		}
	}
}

// deriveBrainState computes the current brain state from observable
// signals. No local model → error. Any in-flight run OR a collector
// that indexed something in the last 30s → collecting. Otherwise idle.
// The "analyzing" state is reserved for the (future) triage loop and
// isn't reachable from this function yet.
func (a *App) deriveBrainState() (state, detail string) {
	if !pingHTTP("http://localhost:11434") {
		return brainStateError, "ollama not reachable"
	}
	a.runsMu.Lock()
	active := len(a.runs)
	a.runsMu.Unlock()
	if active > 0 {
		if active == 1 {
			return brainStateCollecting, "1 run in flight"
		}
		return brainStateCollecting, fmt.Sprintf("%d runs in flight", active)
	}

	// If any collector indexed something recently, treat that as active
	// collection too — gives the user "watching" → "collecting" feedback
	// when ES picks up new docs without a chat run.
	a.collectorMu.Lock()
	var freshSource string
	now := time.Now().UnixMilli()
	for name, act := range a.collectorState {
		if act.LastSeenMs > 0 && now-act.LastSeenMs < 30_000 {
			freshSource = name
			break
		}
	}
	a.collectorMu.Unlock()
	if freshSource != "" {
		return brainStateCollecting, "indexed activity in " + freshSource
	}
	return brainStateIdle, "watching"
}

// runTriageOnce performs one triage cycle: pull recent events from
// ES, hand them to the local Ollama model with the triage prompt,
// and surface any returned alerts as brain:activity entries. The
// brain state goes to "analyzing" while the model is working so the
// user can see the indicator pulse purple.
//
// Triage is best-effort and degrades silently:
//   - no events since last tick → nothing to do
//   - ES unreachable           → skip, try again next tick
//   - ollama unreachable       → empty result, no alerts emitted
//   - model returns nonsense   → empty result, no alerts emitted
//
// The high-water mark advances on every successful query so we don't
// re-feed the model the same buffer.
func (a *App) runTriageOnce() {
	a.triageMu.Lock()
	since := a.triageLastMs
	a.triageMu.Unlock()

	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	defer cancel()

	events, err := glitchd.QueryRecentEvents(ctx, since, 50)
	if err != nil {
		// ES probably down — quiet skip.
		return
	}
	if len(events) == 0 {
		return
	}

	// Show the user something is happening. We deliberately set the
	// state directly here instead of waiting for the next stateTick
	// so the icon flips to "analyzing" the moment triage starts.
	a.emitBrainStatus(brainStateAnalyzing,
		fmt.Sprintf("triaging %d event(s)", len(events)))

	result, err := glitchd.TriageEvents(ctx, events, "")
	// Restore state to whatever's currently true (collecting/idle/error)
	// regardless of triage outcome — we don't want the icon stuck on
	// "analyzing" if the model errors.
	defer func() {
		state, detail := a.deriveBrainState()
		a.emitBrainStatus(state, detail)
	}()

	if err != nil {
		// Triage error is unusual enough to surface, but only as a
		// low-noise check-in (not a systray alert).
		a.emitBrainActivity("checkin", "info",
			"triage skipped",
			err.Error(), "")
		return
	}
	if result == nil {
		return
	}

	// Advance the high-water mark to the newest event we just
	// considered. Events are sorted desc, so [0] is the most recent.
	if len(events) > 0 {
		newest := events[0].Timestamp.UnixMilli()
		a.triageMu.Lock()
		if newest > a.triageLastMs {
			a.triageLastMs = newest
		}
		a.triageMu.Unlock()
	}

	// Emit each alert. Severity warn/error fan out to glitch-notify
	// via emitBrainActivity → busd publish.
	for _, al := range result.Alerts {
		title := strings.TrimSpace(al.Title)
		if title == "" {
			continue
		}
		kind := "alert"
		if al.Severity == "info" {
			kind = "checkin"
		}
		a.emitBrainActivity(kind, al.Severity, title, al.Why, al.Source)
	}
	// "Stored" entries are low-noise FYIs the model wants to remember.
	// We surface them as info check-ins so the activity panel reflects
	// the model's reasoning, but they don't trigger systray pings.
	for _, s := range result.Stored {
		t := strings.TrimSpace(s.Title)
		if t == "" {
			continue
		}
		a.emitBrainActivity("checkin", "info", t, s.Summary, "")
	}
}

// refreshCollectorActivity polls Elasticsearch for per-source doc
// counts, computes deltas vs. the previous snapshot, and emits a
// brain:activity entry for any source that picked up new docs since
// the last poll. Runs in the brain loop's own ticker so the user-facing
// ListCollectors() call doesn't have to hit ES.
//
// initial=true skips the delta emission (we don't want a flood of
// "got 12 new commits" entries on every fresh launch).
func (a *App) refreshCollectorActivity(initial bool) {
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	rows, err := glitchd.QueryCollectorActivity(ctx)
	if err != nil {
		// ES probably not running. Don't spam alerts — the brain state
		// already shows "error" via the ollama check, and ES is its own
		// service. Just skip silently.
		return
	}

	a.collectorMu.Lock()
	prev := a.collectorState
	a.collectorState = make(map[string]glitchd.CollectorActivity, len(rows))
	for _, r := range rows {
		a.collectorState[r.Source] = r
	}
	a.collectorMu.Unlock()

	if initial {
		return
	}

	// Compare against the previous snapshot. Anything with a higher
	// total_docs gets a check-in entry; large jumps (>= 10) get
	// promoted to a warn alert that fans out to the systray.
	for _, r := range rows {
		old := prev[r.Source]
		delta := r.TotalDocs - old.TotalDocs
		if delta <= 0 {
			continue
		}
		title := fmt.Sprintf("%s · %d new", r.Source, delta)
		detail := fmt.Sprintf("indexed %d new doc(s) since last poll · %d total",
			delta, r.TotalDocs)
		severity := "info"
		kind := "checkin"
		if delta >= 10 {
			severity = "warn"
			kind = "alert"
		}
		a.emitBrainActivity(kind, severity, title, detail, r.Source)
	}
}

// ── Notify ──────────────────────────────────────────────────────────────────

func (a *App) startNotify() {
	home, _ := os.UserHomeDir()
	binary := filepath.Join(home, ".local", "bin", "glitch-notify")
	if _, err := os.Stat(binary); err != nil {
		return
	}
	_ = exec.Command("pkill", "-f", "glitch-notify").Run()
	cmd := exec.Command(binary)
	if err := cmd.Start(); err != nil {
		return
	}
	a.notifyProc = cmd.Process
	go func() { _ = cmd.Wait() }()
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// encodeBlockEvent flattens a glitchd.BlockEvent into a plain map for the
// Wails wire format. The frontend dispatches on the "kind" field
// ("start" | "chunk" | "end") and reconstructs blocks from there. The
// workspace_id field lets the frontend route the event to the correct
// workspace's message buffer instead of the active one.
func encodeBlockEvent(workspaceID string, ev glitchd.BlockEvent) map[string]any {
	out := map[string]any{
		"workspace_id": workspaceID,
		"block":        ev.Block,
	}
	switch ev.Kind {
	case glitchd.BlockStart:
		out["kind"] = "start"
		if len(ev.Attrs) > 0 {
			out["attrs"] = ev.Attrs
		}
	case glitchd.BlockChunk:
		out["kind"] = "chunk"
		out["text"] = ev.Text
	case glitchd.BlockEnd:
		out["kind"] = "end"
	}
	return out
}

func pingHTTP(url string) bool {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func pingUnix(path string) bool {
	if path == "" {
		return false
	}
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func busdSocket() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "glitch", "bus.sock")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "glitch", "bus.sock")
}
