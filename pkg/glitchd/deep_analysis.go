// deep_analysis.go runs gl1tch's "deep analysis" loop.
//
// The loop is the second LLM-driven layer over collector output. The
// first layer is triage (pkg/glitchd/triage.go), which feeds a batch
// of recent events into a stateless Ollama prompt and produces terse
// "look at this" alerts. Triage is fast, batched, and tool-less — it
// answers "anything in the last 2 minutes worth raising an eyebrow?"
//
// Deep analysis answers a different question: for ONE significant
// event, "what is this and what should I do about it?" That answer
// requires the LLM to actually look at the artifact — diff a commit,
// pull a PR's review comments, read a related file — which means it
// needs tools. The Ollama HTTP API is tool-less, so deep analysis
// shells out to the `opencode` CLI, which is an agentic loop with
// built-in shell-tool access. Whatever model you point opencode at
// (qwen2.5-coder, llama3.2, etc.) gets to run `gh pr view`, `git log`,
// `cat README.md`, and friends as part of producing its overview.
//
// Source-agnostic: the analyzer takes any AnalyzableEvent and decides
// based on the event's content (not its type) whether to spend a
// model call on it. Github PRs, git commits, claude session
// summaries, copilot history, directory artifacts — all flow through
// the same eligibility filter and the same opencode invocation. New
// collector types automatically benefit; no analyzer-side changes
// needed when a future collector lands.
//
// Concurrency: at most one opencode invocation runs at a time. The
// queue is bounded so a chatty collector tick can't pile up thousands
// of pending analyses; older queued events are dropped to make room
// for newer ones (newer events are usually more actionable).
//
// Storage: every successful analysis is bulk-indexed into ES under
// the glitch-analyses index, so the activity sidebar's expand-row
// affordance can fetch by event_key. The dedupe table in SQLite
// (analysis_dedupe) prevents the same event from being analyzed
// twice across pod restarts.
package glitchd

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
)

// WireAnalyzerToCollectors installs the process-wide event sink that
// feeds every successfully-indexed collector doc into the given
// Analyzer's queue. The desktop App calls this once at startup after
// constructing its Analyzer; pkg/glitchd owns the
// esearch.Event → AnalyzableEvent translation so the desktop
// doesn't need to import internal/esearch directly (glitch-desktop
// is a separate Go module and can't reach into internal/).
//
// Passing nil clears the sink. Safe to call multiple times — later
// calls replace earlier ones.
func WireAnalyzerToCollectors(a *Analyzer) {
	if a == nil {
		capability.SetEventSink(nil)
		return
	}
	capability.SetEventSink(func(workspaceID, source string, docs []any) {
		if len(docs) == 0 {
			return
		}
		// Load the config so the analyzer's enabled check has
		// something to chew on. Enqueue short-circuits on a nil cfg,
		// so load errors are benign here.
		var cfg *capability.Config
		if workspaceID != "" {
			cfg, _ = capability.LoadWorkspaceConfig(workspaceID)
		} else {
			cfg, _ = capability.LoadConfig()
		}
		ctx := context.Background()

		// Translate every typed event into the analyzer's shape up
		// front so we can run the attention classifier across the
		// whole batch in a single Ollama round-trip. Batching is
		// important: each classifier call has a warm-start cost, so
		// classifying 10 events in one call is dramatically cheaper
		// than 10 sequential calls.
		analyzable := make([]AnalyzableEvent, 0, len(docs))
		for _, d := range docs {
			ev, ok := d.(esearch.Event)
			if !ok {
				// Collectors that index bare map[string]any docs
				// can't be analyzed without a typed shape. Skip
				// silently — the analyzer can't anchor a prompt on
				// untyped data anyway.
				continue
			}
			analyzable = append(analyzable,
				esearchEventToAnalyzable(workspaceID, source, ev))
		}
		if len(analyzable) == 0 {
			return
		}

		// Filter to events worth classifying (non-empty, non-stale),
		// then let the LLM decide everything — no type allow-list.
		relevantIdx := make([]int, 0, len(analyzable))
		relevantEvents := make([]AnalyzableEvent, 0, len(analyzable))
		for i, ae := range analyzable {
			if ClassifierRelevant(ae) {
				relevantIdx = append(relevantIdx, i)
				relevantEvents = append(relevantEvents, ae)
			}
		}
		if len(relevantEvents) > 0 {
			verdicts, _ := ClassifyAttention(ctx, relevantEvents, workspaceID)
			for j := range relevantEvents {
				if j < len(verdicts) {
					target := relevantIdx[j]
					analyzable[target].Attention = verdicts[j].Level
					analyzable[target].AttentionReason = verdicts[j].Reason
				}
			}
		}

		// Notify any registered attention observer for every
		// classified event. The desktop uses this hook to surface
		// high-attention events that the heavy analyzer is going
		// to skip because the user hasn't enabled it — a "flagged
		// high but deep analysis is off" nudge in the activity
		// sidebar. Having the hook called on every event (not just
		// high ones) keeps the contract flexible: a future observer
		// could also badge normal/low rows without another hook.
		if obs := getAttentionObserver(); obs != nil {
			for _, ae := range analyzable {
				obs(ae, isAnalysisEnabled(cfg))
			}
		}

		for _, ae := range analyzable {
			a.Enqueue(ctx, ae, cfg)
		}
	})
}

// esearchEventToAnalyzable converts one indexed esearch.Event into
// the AnalyzableEvent shape the analyzer consumes. Pulled into its
// own function so tests can exercise the translation without wiring
// a full sink.
func esearchEventToAnalyzable(workspaceID, source string, ev esearch.Event) AnalyzableEvent {
	return AnalyzableEvent{
		Type:        ev.Type,
		Source:      source,
		Repo:        ev.Repo,
		Author:      ev.Author,
		Title:       ev.Message,
		Body:        ev.Body,
		Identifier:  esearchEventIdentifier(ev),
		URL:         esearchEventURL(ev),
		WorkspaceID: workspaceID,
		Timestamp:   ev.Timestamp,
	}
}

// esearchEventIdentifier extracts the most stable id we can find on
// an esearch.Event for the analyzer's dedupe key. Falls back through
// SHA → review_id → comment_id → pr_number → url → empty. Empty is
// OK: AnalyzableEvent.EventKey hashes title+body in that case.
//
// review_id and comment_id are per-event identifiers set by the
// GitHub collector, so two different reviews on the same PR get
// distinct dedupe keys instead of collapsing to the PR number.
func esearchEventIdentifier(ev esearch.Event) string {
	if ev.SHA != "" {
		return ev.SHA
	}
	if ev.Metadata != nil {
		// Per-event identifiers (review, comment) take priority over
		// the PR/issue number so events on the same PR dedupe independently.
		if s, ok := ev.Metadata["review_id"].(string); ok && s != "" {
			return s
		}
		if s, ok := ev.Metadata["comment_id"].(string); ok && s != "" {
			return s
		}
		if n, ok := ev.Metadata["number"].(float64); ok && n > 0 {
			return fmt.Sprintf("%d", int(n))
		}
		if n, ok := ev.Metadata["number"].(int); ok && n > 0 {
			return fmt.Sprintf("%d", n)
		}
		if s, ok := ev.Metadata["url"].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// esearchEventURL extracts a canonical URL for the event so the
// activity row can render a link alongside the markdown. Github
// events stash it in metadata.url; other sources get nothing.
func esearchEventURL(ev esearch.Event) string {
	if ev.Metadata != nil {
		if s, ok := ev.Metadata["url"].(string); ok {
			return s
		}
	}
	return ""
}

// AnalyzableEvent is the minimal shape the analyzer needs to decide
// whether an event is worth a model call and to construct the prompt
// for it. Pulled out as its own type so the WorkspaceCollector (and
// any future collector) can build one without depending on the full
// esearch.Event surface — keeps the collector → analyzer wire small.
type AnalyzableEvent struct {
	// Type is the granular event type (e.g. "git.commit", "github.pr",
	// "claude.session"). Stored on the analysis doc but the analyzer
	// itself doesn't switch on it — eligibility is content-based.
	Type string
	// Source is the collector that produced the event ("git",
	// "github", "directory", "claude", etc.). Used for the dedupe
	// table's source column and the activity row label.
	Source string
	// Repo is the short repo name (filepath.Base for git; owner/repo
	// for github). Empty for sources that don't have a repo notion.
	Repo string
	// Author is the human or bot who produced the event. Used by the
	// bot filter and surfaced in the prompt for context.
	Author string
	// Title is the one-line summary the LLM should anchor on. For git
	// it's the commit subject; for github.pr it's "#42 fix: foo".
	Title string
	// Body is the long-form context (commit body, PR description,
	// session content). Truncated by the prompt builder if it's huge.
	Body string
	// Identifier is whatever stable string makes the event unique
	// inside its source — a SHA for git, a PR number for github,
	// a session id for claude. Combined with Source and Repo to form
	// the dedupe key. Empty Identifier falls back to a hash of
	// Title+Body so events without natural ids still dedupe.
	Identifier string
	// URL is an optional pointer to the canonical artifact (github
	// PR/issue URL, commit URL). Surfaced in the activity row so the
	// user can jump out to the source.
	URL string
	// WorkspaceID stamps the analysis doc so per-workspace queries
	// can scope correctly. Empty for events from the global tool
	// pod.
	WorkspaceID string
	// Timestamp is when the original event happened (commit time,
	// PR updated_at, etc.). Used to skip events older than the
	// "fresh enough to act on" cutoff.
	Timestamp time.Time
	// Attention is the local classifier's verdict for this event —
	// one of "high", "normal", "low" (see attention.go). Empty
	// string means the classifier was skipped (analysis disabled in
	// config) and callers should treat the event as normal.
	Attention AttentionLevel
	// AttentionReason is the one-sentence explanation the
	// classifier attached to its verdict. Surfaced in the activity
	// row so the user can see *why* gl1tch flagged (or didn't flag)
	// an event. Empty when Attention is empty.
	AttentionReason string
}

// EventKey returns a stable, source-agnostic dedupe key for this
// event. Same key produced on every poll cycle so the SQLite
// dedupe check returns true on the second sighting and the analyzer
// skips a duplicate opencode run.
//
// Format: "<source>:<type>:<repo>:<id>"
//   - <id> is e.Identifier when set
//   - otherwise a sha1 of (title+body) so different events with the
//     same source/type/repo still get unique keys
func (e AnalyzableEvent) EventKey() string {
	id := e.Identifier
	if id == "" {
		h := sha1.Sum([]byte(e.Title + "\n" + e.Body))
		id = hex.EncodeToString(h[:])[:16]
	}
	return fmt.Sprintf("%s:%s:%s:%s", e.Source, e.Type, e.Repo, id)
}

// AnalysisResult is one finished analysis ready to be indexed and
// surfaced to the user. The Markdown field is what the activity
// sidebar renders when the row is expanded.
type AnalysisResult struct {
	EventKey    string
	Source      string
	Type        string
	Repo        string
	Title       string
	Model       string
	Markdown    string
	ExitCode    int
	Duration    time.Duration
	WorkspaceID string
	CreatedAt   time.Time
	// URL is the canonical web URL for the underlying event — a
	// github PR link, a commit URL, etc. Populated from the input
	// AnalyzableEvent's URL field and surfaced in the chat header
	// as a clickable link so the user can jump straight to the
	// source from a proactive assistant message.
	URL string
	// Attention is the classifier verdict that drove this run — one
	// of "high", "normal", "low", or empty for legacy callers that
	// bypass the classifier. Persisted onto the analysis doc so the
	// frontend can route `high` results to the chat/pinned lane and
	// `normal` results to the expand-row lane.
	Attention AttentionLevel
	// AttentionReason is the classifier's one-sentence rationale.
	// Surfaced next to the verdict badge in the activity row.
	AttentionReason string
}

// AnalysisHandler is the callback invoked once a result is ready.
// The desktop App passes its own emitter so the result can be
// pushed to the frontend as a brain:activity entry without the
// analyzer importing wails runtime.
type AnalysisHandler func(AnalysisResult)

// Analyzer is the package-level singleton that owns the in-process
// queue and worker goroutine. The desktop App constructs one at
// startup via NewAnalyzer and feeds events into it from the
// collector → analyzer wiring in app.go.
//
// At most one opencode invocation runs at a time. The queue is a
// fixed-capacity ring (default 32 events); when full, the oldest
// queued event is dropped to make room. Newer events are almost
// always more interesting than older ones, so dropping old over
// rejecting new is the right tradeoff.
type Analyzer struct {
	es      *esearch.Client
	handler AnalysisHandler

	mu      sync.Mutex
	queue   []AnalyzableEvent
	cap     int
	cond    *sync.Cond
	closed  bool
	running bool

	// last is the wall-clock time of the most recent finished run.
	// Used to enforce the cooldown gap between consecutive runs so
	// the local LLM doesn't spin continuously.
	lastRun time.Time
}

// NewAnalyzer constructs an Analyzer bound to the given ES client and
// activity handler. Caller must call Start to launch the worker
// goroutine.
//
// es may be nil — the analyzer will still run opencode and emit the
// activity entry, it just won't persist the result to ES. Tests use
// nil; production wires the desktop App's shared client.
func NewAnalyzer(es *esearch.Client, handler AnalysisHandler) *Analyzer {
	a := &Analyzer{
		es:      es,
		handler: handler,
		cap:     32,
	}
	a.cond = sync.NewCond(&a.mu)
	return a
}

// Enqueue pushes one event onto the analyzer queue. Returns true if
// the event was accepted, false if it was dropped (queue full and
// the event lost the oldest-vs-newest contest, or the event was
// rejected by the eligibility filter, or the analyzer is closed).
//
// Always returns quickly — Enqueue never blocks on the worker. The
// collector tick goroutine that calls this is in the hot path of
// every poll cycle, so blocking here would back up the entire pod.
func (a *Analyzer) Enqueue(ctx context.Context, ev AnalyzableEvent, cfg *capability.Config) bool {
	if !isAnalysisEnabled(cfg) {
		return false
	}
	if !eligibleForAnalysis(ev) {
		return false
	}

	// Dedupe against the SQLite table. Events that have already been
	// analyzed (in this process or a previous one) are skipped here
	// so we don't waste a queue slot on them.
	if st, err := OpenStore(); err == nil {
		if seen, _ := st.HasAnalyzedEvent(ctx, ev.EventKey()); seen {
			return false
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return false
	}
	// Drop the oldest queued event when the buffer is full. The
	// alternative — rejecting new events — is worse because new
	// events are almost always more actionable than stale ones.
	if len(a.queue) >= a.cap {
		dropped := a.queue[0]
		a.queue = a.queue[1:]
		slog.Warn("analyzer: queue full, dropping oldest",
			"dropped_key", dropped.EventKey(),
			"new_key", ev.EventKey())
	}
	a.queue = append(a.queue, ev)
	a.cond.Signal()
	return true
}

// Start launches the worker goroutine. Safe to call once; subsequent
// calls are no-ops. The goroutine runs until ctx is cancelled OR
// Close is called, whichever comes first.
func (a *Analyzer) Start(ctx context.Context) {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.mu.Unlock()

	go a.workerLoop(ctx)
}

// Close signals the worker to drain the queue and exit. Idempotent.
// Used by the desktop App's shutdown path so a graceful exit doesn't
// leak the goroutine.
func (a *Analyzer) Close() {
	a.mu.Lock()
	a.closed = true
	a.cond.Broadcast()
	a.mu.Unlock()
}

// workerLoop is the single goroutine that pulls events off the queue
// one at a time, runs opencode against each, and persists the
// result. Blocks on the cond when the queue is empty so it doesn't
// busy-spin.
func (a *Analyzer) workerLoop(ctx context.Context) {
	for {
		ev, ok := a.dequeue(ctx)
		if !ok {
			return
		}

		// Honor the cooldown between runs so the local LLM gets a
		// breather between consecutive analyses. The cooldown is
		// per-process not per-event, so a long burst of new events
		// still gets processed — just at a steady rate rather than
		// all at once.
		//
		// Exception: high-attention events bypass the cooldown. The
		// cooldown exists to keep a chatty collector from pinning
		// the GPU on low-value events, but a high-attention event
		// is precisely what the GPU is for — making the user wait
		// 30s for a review-reply draft when the model is otherwise
		// idle defeats the whole point of the classifier.
		cfg, _ := capability.LoadConfig()
		cooldown := analysisCooldown(cfg)
		if ev.Attention == AttentionHigh {
			cooldown = 0
		}
		if cooldown > 0 && !a.lastRun.IsZero() {
			elapsed := time.Since(a.lastRun)
			if elapsed < cooldown {
				wait := cooldown - elapsed
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
				}
			}
		}

		result := a.runOne(ctx, ev, cfg)
		a.lastRun = time.Now()

		// Persist the dedupe entry whether the run succeeded or
		// failed — failed runs shouldn't be retried in a tight loop.
		if st, err := OpenStore(); err == nil {
			_ = st.MarkEventAnalyzed(ctx, ev.EventKey(), ev.Source,
				time.Now().UnixMilli())
		}

		if result.Markdown != "" && a.es != nil {
			doc := analysisDoc{
				EventKey:        result.EventKey,
				Source:          result.Source,
				Type:            result.Type,
				Repo:            result.Repo,
				Title:           result.Title,
				Model:           result.Model,
				Markdown:        result.Markdown,
				ExitCode:        result.ExitCode,
				DurationMs:      result.Duration.Milliseconds(),
				WorkspaceID:     result.WorkspaceID,
				CreatedAt:       result.CreatedAt,
				Attention:       result.Attention,
				AttentionReason: result.AttentionReason,
			}
			if err := a.es.BulkIndex(ctx, esearch.IndexAnalyses, []any{doc}); err != nil {
				slog.Warn("analyzer: index result failed",
					"event_key", result.EventKey, "err", err)
			}
		}

		if a.handler != nil {
			a.handler(result)
		}
	}
}

// dequeue pops one event off the queue, blocking if the queue is
// empty. Returns ok=false when the analyzer has been closed or ctx
// is done.
func (a *Analyzer) dequeue(ctx context.Context) (AnalyzableEvent, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for len(a.queue) == 0 && !a.closed {
		// Wake on close OR on a queue push. Context cancellation
		// also has to wake the worker; we run a tiny goroutine that
		// signals the cond when ctx is done. The cond signal is
		// idempotent so spurious wakeups are harmless.
		go func() {
			<-ctx.Done()
			a.mu.Lock()
			a.cond.Broadcast()
			a.mu.Unlock()
		}()
		a.cond.Wait()
		if ctx.Err() != nil {
			return AnalyzableEvent{}, false
		}
	}
	if a.closed && len(a.queue) == 0 {
		return AnalyzableEvent{}, false
	}
	ev := a.queue[0]
	a.queue = a.queue[1:]
	return ev, true
}

// runOne executes opencode against a single event and returns the
// resulting AnalysisResult. Failures are returned as results with an
// empty Markdown so the caller can still emit an activity row that
// surfaces the error.
//
// Thin wrapper around the package-level AnalyzeOne so the queue
// worker and out-of-band callers (CLI, smoke tests) share one code
// path. The receiver is retained for historical reasons and in case
// per-analyzer state (rate limiters, progress counters) grows here
// later.
func (a *Analyzer) runOne(ctx context.Context, ev AnalyzableEvent, cfg *capability.Config) AnalysisResult {
	return AnalyzeOne(ctx, ev, cfg)
}

// AnalyzeOne runs a single deep analysis synchronously and returns
// the result. This is the lowest common denominator of the
// analyzer: no queue, no cooldown, no dedupe table — just build the
// prompt (artifact-mode for high-attention events, summary-mode
// otherwise), shell out to opencode, parse the response.
//
// Used by the queue worker to process dequeued events and by the
// `glitch attention analyze` CLI and `test/smoke` harness to drive
// the full analysis pipeline on an ad-hoc event without needing a
// running Analyzer, ES, or the collector loop.
//
// Failures return a result with empty Markdown and a non-zero
// ExitCode so the caller can distinguish "ran but produced nothing"
// from "ran and produced content". Callers that care about the
// distinction should check ExitCode first.
func AnalyzeOne(ctx context.Context, ev AnalyzableEvent, cfg *capability.Config) AnalysisResult {
	model := analysisModel(cfg)
	prompt := buildAnalysisPrompt(ev)
	start := time.Now()

	// Run opencode with a generous but bounded timeout. Tool-using
	// agents can take a while — the prompt asks the model to gather
	// context with shell commands, and 4-8 tool round-trips against
	// gh / git / file system is normal. 5 minutes is enough for any
	// reasonable single-event analysis without letting a runaway
	// model burn the GPU forever.
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "opencode",
		"run",
		"--model", model,
		"--format", "json",
		"--", prompt)

	stdout, err := cmd.Output()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
		slog.Warn("analyzer: opencode failed",
			"event_key", ev.EventKey(), "err", err)
	}

	markdown := extractTextFromOpencodeJSON(stdout)
	if markdown == "" && exitCode == 0 {
		// opencode returned successfully but with empty content —
		// most likely the user has an unconfigured / broken model.
		// Surface a clear message instead of an empty activity row.
		markdown = "_(opencode returned no text — check that `opencode` is installed and the configured model is pulled)_"
	}

	return AnalysisResult{
		EventKey:        ev.EventKey(),
		Source:          ev.Source,
		Type:            ev.Type,
		Repo:            ev.Repo,
		Title:           ev.Title,
		Model:           model,
		Markdown:        markdown,
		ExitCode:        exitCode,
		Duration:        time.Since(start),
		WorkspaceID:     ev.WorkspaceID,
		CreatedAt:       time.Now(),
		URL:             ev.URL,
		Attention:       ev.Attention,
		AttentionReason: ev.AttentionReason,
	}
}

// analysisDoc is the on-the-wire shape we index into glitch-analyses.
// Mirrors the analysesMapping in internal/esearch/mappings.go field
// for field — keep them in sync.
type analysisDoc struct {
	EventKey        string    `json:"event_key"`
	Source          string    `json:"source"`
	Type            string    `json:"type"`
	Repo            string    `json:"repo,omitempty"`
	Title           string    `json:"title,omitempty"`
	Model           string    `json:"model"`
	Markdown        string    `json:"markdown"`
	ExitCode        int       `json:"exit_code"`
	DurationMs      int64     `json:"duration_ms"`
	WorkspaceID     string    `json:"workspace_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	Attention       string    `json:"attention,omitempty"`
	AttentionReason string    `json:"attention_reason,omitempty"`
}

// ── Eligibility filter ─────────────────────────────────────────────

// eligibleForAnalysis decides whether an event is worth a model call.
// eligibleForAnalysis gates deep analysis on structural checks only.
// No semantic filtering — the LLM classifier already decided what
// matters. We just skip events that are structurally unfit for a
// model call (empty content, stale timestamps).
func eligibleForAnalysis(ev AnalyzableEvent) bool {
	return ClassifierRelevant(ev)
}

// ── Prompt construction ────────────────────────────────────────────

// buildAnalysisPrompt produces the opencode prompt for one event.
//
// Two modes, chosen by the event's classifier verdict:
//
//   - Artifact mode (ev.Attention == "high"): the prompt is loaded
//     from pkg/glitchd/prompts/deep_analysis_artifact.md with the
//     user's research prompt spliced in. The rubric tells opencode
//     to produce the concrete artifact the user needs (a drafted
//     reply, a patch, a command) rather than a summary. This is
//     the "local does the work" path.
//
//   - Summary mode (everything else): the legacy inline prompt
//     below, which asks for a What/Why/Next rubric. Used when the
//     classifier says the event is normal or low, when the
//     classifier was skipped, or when the artifact template fails
//     to load.
//
// Artifact-mode failures fall through to summary mode silently —
// we'd rather produce the old summary than no analysis at all.
func buildAnalysisPrompt(ev AnalyzableEvent) string {
	if ev.Attention == AttentionHigh {
		if p, err := buildArtifactPrompt(ev); err == nil && p != "" {
			return p
		}
		slog.Warn("analyzer: artifact prompt unavailable, falling back to summary",
			"event_key", ev.EventKey())
	}
	return buildSummaryPrompt(ev)
}

// buildArtifactPrompt renders the high-attention artifact-mode
// template. Splices in the workspace research prompt (the one the
// classifier also read) so the model produces exactly the kind of
// artifact the user declared they want for this event class.
func buildArtifactPrompt(ev AnalyzableEvent) (string, error) {
	research, err := LoadResearchPrompt(ev.WorkspaceID)
	if err != nil {
		return "", err
	}
	body := strings.TrimSpace(ev.Body)
	if body == "" {
		body = "(empty)"
	} else if len(body) > 1500 {
		body = body[:1500] + "…"
	}
	// Resolve the user's github handle the same way the classifier
	// does: git config user.email → noreply parse → handle. Fall
	// back to git config user.name if the email is not a github
	// noreply, then to a placeholder if neither is set. The
	// artifact template is emphatic that the model must write AS
	// this identity, not about them — so an empty {{USER_GITHUB}}
	// would break the whole rubric's instructions about
	// first-person framing.
	userName, userEmail := localGitIdentity()
	userGithub := parseGitHubHandleFromEmail(userEmail)
	if userGithub == "" {
		userGithub = userName
	}
	if userGithub == "" {
		userGithub = "the user"
	}
	return RenderPrompt("deep_analysis_artifact", map[string]string{
		"RESEARCH_PROMPT":  research,
		"USER_GITHUB":      userGithub,
		"SOURCE":           ev.Source,
		"TYPE":             ev.Type,
		"REPO":             ev.Repo,
		"AUTHOR":           ev.Author,
		"IDENTIFIER":       ev.Identifier,
		"URL":              ev.URL,
		"TITLE":            ev.Title,
		"ATTENTION_REASON": ev.AttentionReason,
		"BODY":             body,
	})
}

// buildSummaryPrompt is the legacy summary-mode prompt — the
// What/Why/Next rubric used for normal and low attention events,
// and as the artifact-mode fallback. Kept as inline strings (not a
// template file) because this is the one prompt that must never
// fail to load: if everything else is broken, this is what keeps
// the analyzer producing *something* the user can read.
func buildSummaryPrompt(ev AnalyzableEvent) string {
	var b strings.Builder
	b.WriteString("You are gl1tch's deep-analysis assistant. ")
	b.WriteString("A new event landed in one of the user's collectors. ")
	b.WriteString("Use your shell tools to gather context about it, then produce a concise markdown overview the user can act on.\n\n")

	b.WriteString("## Event\n")
	fmt.Fprintf(&b, "- source: `%s`\n", ev.Source)
	fmt.Fprintf(&b, "- type: `%s`\n", ev.Type)
	if ev.Repo != "" {
		fmt.Fprintf(&b, "- repo: `%s`\n", ev.Repo)
	}
	if ev.Author != "" {
		fmt.Fprintf(&b, "- author: `%s`\n", ev.Author)
	}
	if ev.Identifier != "" {
		fmt.Fprintf(&b, "- id: `%s`\n", ev.Identifier)
	}
	if ev.URL != "" {
		fmt.Fprintf(&b, "- url: %s\n", ev.URL)
	}
	if ev.Title != "" {
		fmt.Fprintf(&b, "- title: %s\n", ev.Title)
	}
	if body := strings.TrimSpace(ev.Body); body != "" {
		if len(body) > 1500 {
			body = body[:1500] + "…"
		}
		b.WriteString("\n### Body\n")
		b.WriteString(body)
		b.WriteString("\n")
	}

	b.WriteString(`
## Tool guidance

Pick the right tool for the source:

- **github** — use `)
	b.WriteString("`gh pr view <number> --repo <owner/repo>`, `gh pr diff`, `gh pr checks`, `gh issue view`, `gh issue comments` to gather PR/issue context.")
	b.WriteString("\n- **git** — use `git -C <repo path> show <sha>`, `git log -p -1 <sha>`, `git diff <sha>~1 <sha>` to inspect commits.")
	b.WriteString("\n- **directory / claude / copilot** — use `cat`, `head`, `ls`, `rg` to read related files in the workspace.")
	b.WriteString("\n- For any source you're unsure about, run a couple of quick `ls`/`cat` calls to orient yourself before writing the overview.\n")

	b.WriteString(`
## Output format

Produce **markdown only** with these sections (omit any that don't apply):

### What it is
One or two sentences. What changed, who changed it, why it appeared in the capability.

### Why it matters
Why this is interesting / risky / blocking / boring. Be honest — "low signal, you can ignore" is a valid answer.

### Suggested next steps
A short bulleted list (max 5 items) of concrete actions the user could take. Use code blocks for any commands.

### Risks / open questions
Anything you noticed that might bite later. Skip the section if there's nothing.

Keep the whole response under 400 words. Skip filler. Don't echo the event back at me.`)

	return b.String()
}

// ── opencode output parsing ────────────────────────────────────────

// extractTextFromOpencodeJSON walks opencode's --format json stream
// (one JSON event per line) and concatenates the "text" parts so the
// caller gets the model's actual reply without tool-call markers,
// step boundaries, or internal events.
//
// opencode's JSON format mirrors what plugins/opencode/main.go's
// execOpencode already parses — we re-implement the same loop here
// because the desktop binary doesn't shell out through that plugin.
func extractTextFromOpencodeJSON(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var out strings.Builder
	// Opencode emits one JSON object per line in --format json mode.
	// Text deltas are shaped `{"type":"text","part":{"text":"..."}}`.
	// We parse each candidate line with encoding/json instead of the
	// earlier substring hack, which used `LastIndex('"')` and often
	// caught a closing quote from a trailing field like `"time":{...}`
	// on the same line — the result was that everything from the end
	// of the real text up through the next closing quote anywhere on
	// the line leaked into the final markdown. The symptom was chat
	// messages that trailed off into a snippet like
	// `…review again!","time":{"start":1775673438908,"end` in the
	// user's screenshot. Proper parsing eliminates that failure mode
	// and makes this function behave identically to the streaming
	// extractor in activity_analyzer.go's extractTextDelta.
	var evShape struct {
		Type string `json:"type"`
		Part struct {
			Text string `json:"text"`
		} `json:"part"`
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, `"type":"text"`) {
			continue
		}
		// Reset between lines so a malformed event can't leak its
		// half-parsed state into the next line's output.
		evShape.Type = ""
		evShape.Part.Text = ""
		if err := json.Unmarshal([]byte(line), &evShape); err != nil {
			continue
		}
		if evShape.Type != "text" || evShape.Part.Text == "" {
			continue
		}
		out.WriteString(evShape.Part.Text)
	}
	return strings.TrimSpace(out.String())
}

// ── Config helpers ─────────────────────────────────────────────────

func isAnalysisEnabled(cfg *capability.Config) bool {
	return cfg != nil && cfg.Analysis.Enabled
}

func analysisModel(cfg *capability.Config) string {
	if cfg != nil && cfg.Analysis.Model != "" {
		return cfg.Analysis.Model
	}
	return "ollama/qwen3-coder:30b"
}

func analysisCooldown(cfg *capability.Config) time.Duration {
	if cfg != nil && cfg.Analysis.Cooldown > 0 {
		return cfg.Analysis.Cooldown
	}
	return 30 * time.Second
}
