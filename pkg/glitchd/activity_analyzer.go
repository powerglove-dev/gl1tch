// activity_analyzer.go runs on-demand analysis of user-selected
// activity-panel documents through opencode's tool-using loop,
// streaming tokens back to the caller as they arrive.
//
// This is the second analyzer path in the gl1tch brain. The first
// is deep_analysis.go's Analyzer, which runs autonomously against
// significant events from the triage queue — one event at a time,
// blocking on opencode, result returned whole. That's fine for the
// background path because the user isn't waiting on it.
//
// The activity analyzer answers a different shape of question:
// the user selected N documents in the modal, optionally typed a
// prompt, and pressed Analyze. They're actively watching the
// modal. Blocking on `cmd.Output()` and waiting 30-60 seconds for
// a complete markdown blob is a bad UX. Instead we:
//
//   1. Shell out to `opencode run --format json` as before,
//      passing the rendered prompt from pkg/glitchd/prompts/
//      activity_analyzer.md (operators can edit the rubric
//      without recompiling — this is what "AI-first, nothing
//      hardcoded" means in practice).
//
//   2. Read opencode's stdout line-by-line via cmd.StdoutPipe +
//      bufio.Scanner. opencode emits one JSON object per line
//      (text deltas, tool calls, step boundaries); we pluck the
//      text parts and emit them to a caller-supplied stream
//      channel as they land.
//
//   3. On completion (or error), persist the full markdown to
//      glitch-analyses with a deterministic event_key and return
//      an AnalysisResult. The caller (the desktop Wails binding)
//      emits a brain:activity kind=analysis event so the row
//      shows up in the sidebar like any other analysis.
//
// Refinements: each refinement is an independent opencode run.
// The design originally called for session continuation but
// `opencode run` is one-shot with no session handle, so we took
// the simpler path — see design.md decision #6. The caller passes
// the same document set plus the new user question; ParentID
// links the refinement to its predecessor in ES metadata.
package glitchd

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
)

// activityAnalyzerModelDefault is the model the activity analyzer
// falls back to when neither the request nor config override it.
// qwen2.5:7b is the project-wide local LLM default for generation
// tasks — it has the tool-use quality the opencode loop needs and
// is expected to be auto-pulled on every install.
const activityAnalyzerModelDefault = "ollama/qwen2.5:7b"

// activityAnalyzerTimeout bounds a single opencode invocation.
// Tool-using agents can take a while — 3-5 tool round-trips
// against git/gh/cat over a handful of selected docs is normal.
// 6 minutes is enough for a chunky refinement against a dozen
// docs without letting a runaway model burn the GPU forever.
const activityAnalyzerTimeout = 6 * time.Minute

// ActivityAnalyzeRequest is what the caller hands to
// RunActivityAnalysis. It's the backend shape of the Wails
// AnalyzeRequest the frontend sends — we translate between them
// in the desktop App binding layer so this package doesn't have
// to know about Wails types.
type ActivityAnalyzeRequest struct {
	// Source is the collector source the docs came from (e.g.
	// "git", "github", "claude"). Stored on the persisted
	// analysis so the sidebar row can badge the source. Does
	// NOT scope document selection — the docs are passed in
	// directly via Docs below.
	Source string
	// WorkspaceID scopes the persisted analysis so per-workspace
	// queries against glitch-analyses see only their own runs.
	// May be empty for global / unattributed analyses.
	WorkspaceID string
	// Docs is the set of documents the user selected in the
	// modal, already fetched from ES. Passing them in directly
	// keeps this function free of ES coupling and lets the
	// caller decide how much of each doc to include.
	Docs []RecentEvent
	// UserPrompt is the optional free-form question the user
	// typed into the analysis pane. Empty is fine — the rubric
	// handles the no-prompt case.
	UserPrompt string
	// ParentAnalysisID is the event_key of the analysis this
	// run is refining. Empty on the first analysis of a chain;
	// non-empty for follow-up refinements. Stored on the
	// persisted doc so the UI can render threaded chains.
	ParentAnalysisID string
	// Model overrides the default. Empty means use
	// capability.Config.Analysis.Model, or fall back to the
	// activityAnalyzerModelDefault constant.
	Model string
}

// ActivityAnalysisStreamEvent is one token/status emission from a
// running analysis. The caller's stream channel receives these in
// order; the final event always has Kind == "done" or "error".
type ActivityAnalysisStreamEvent struct {
	// Kind is one of "token" (append Data to the output),
	// "done" (the analysis finished successfully, Data is empty),
	// or "error" (the analysis failed, Error holds the message).
	Kind string
	// Data is the token text for Kind == "token"; empty otherwise.
	Data string
	// Error is the human-readable failure reason for
	// Kind == "error"; empty otherwise.
	Error string
}

// RunActivityAnalysis kicks off an opencode run over the selected
// docs and streams tokens to out until completion. It returns an
// AnalysisResult on success (Markdown populated with the full
// output) or a result with ExitCode != 0 on failure.
//
// The caller owns `out`: RunActivityAnalysis closes it when the
// analysis terminates (successfully or not), so callers should
// range over it in a select that also watches their own ctx.
//
// This function blocks until the opencode run finishes. Callers
// that need it to run in the background should invoke it from a
// goroutine.
func RunActivityAnalysis(
	ctx context.Context,
	req ActivityAnalyzeRequest,
	out chan<- ActivityAnalysisStreamEvent,
) AnalysisResult {
	defer close(out)

	// Resolve the model. Request override wins; then config; then
	// the qwen2.5:7b default. Keep the default in a constant so
	// operators can grep for it and the memory of "hard default
	// is qwen2.5:7b" has a single source of truth.
	model := strings.TrimSpace(req.Model)
	if model == "" {
		cfg, _ := capability.LoadConfig()
		if cfg != nil && cfg.Analysis.Model != "" {
			model = cfg.Analysis.Model
		}
	}
	if model == "" {
		model = activityAnalyzerModelDefault
	}

	// Render the prompt from the editable template. If the
	// template is missing the install is broken — surface a clear
	// error rather than falling back to a hardcoded literal,
	// which would defeat the AI-first rule.
	prompt, err := RenderPrompt("activity_analyzer", map[string]string{
		"USER_PROMPT": userPromptOrPlaceholder(req.UserPrompt),
		"DOC_COUNT":   fmt.Sprintf("%d", len(req.Docs)),
		"DOCUMENTS":   formatDocsForPrompt(req.Docs),
	})
	if err != nil {
		sendStreamError(out, fmt.Sprintf("analyzer prompt missing: %v", err))
		return AnalysisResult{
			EventKey:    computeAnalysisKey(req),
			Source:      req.Source,
			Model:       model,
			Markdown:    "",
			ExitCode:    -1,
			WorkspaceID: req.WorkspaceID,
			CreatedAt:   time.Now(),
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, activityAnalyzerTimeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(runCtx, "opencode",
		"run",
		"--model", model,
		"--format", "json",
		"--", prompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sendStreamError(out, fmt.Sprintf("stdout pipe: %v", err))
		return AnalysisResult{
			EventKey: computeAnalysisKey(req), Source: req.Source,
			Model: model, ExitCode: -1, WorkspaceID: req.WorkspaceID,
			CreatedAt: time.Now(),
		}
	}
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		sendStreamError(out, fmt.Sprintf("opencode start: %v", err))
		return AnalysisResult{
			EventKey: computeAnalysisKey(req), Source: req.Source,
			Model: model, ExitCode: -1, WorkspaceID: req.WorkspaceID,
			CreatedAt: time.Now(),
		}
	}

	// Drain stderr in the background so a chatty opencode build
	// doesn't deadlock the pipe. We don't surface stderr to the
	// user directly — if the process fails, the error reported
	// back to the modal is either the exit code or the last
	// parsed event, not raw stderr noise.
	var stderrBuf strings.Builder
	var stderrWG sync.WaitGroup
	if stderr != nil {
		stderrWG.Add(1)
		go func() {
			defer stderrWG.Done()
			_, _ = io.Copy(&stderrBuf, stderr)
		}()
	}

	// Scan stdout line-by-line. opencode emits one JSON object
	// per line in --format json mode: text deltas, tool calls,
	// step boundaries. We only forward text deltas to the caller.
	var full strings.Builder
	scanner := bufio.NewScanner(stdout)
	// Allow very large lines — opencode occasionally emits a
	// single line per tool-call payload that can exceed the
	// default 64k bufio limit on big diffs.
	scanBuf := make([]byte, 0, 256*1024)
	scanner.Buffer(scanBuf, 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		text := extractTextDelta(line)
		if text == "" {
			continue
		}
		full.WriteString(text)
		select {
		case out <- ActivityAnalysisStreamEvent{Kind: "token", Data: text}:
		case <-runCtx.Done():
			// Context cancelled while we were mid-stream — stop
			// parsing and let cmd.Wait surface the exit.
			break
		}
	}

	waitErr := cmd.Wait()
	stderrWG.Wait()

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
		slog.Warn("activity analyzer: opencode failed",
			"source", req.Source, "exit", exitCode,
			"stderr", strings.TrimSpace(stderrBuf.String()), "err", waitErr)
	}

	markdown := strings.TrimSpace(full.String())
	if markdown == "" && exitCode == 0 {
		// opencode exited cleanly but produced no text — almost
		// always a broken install or missing model pull. Tell the
		// user something concrete instead of an empty pane.
		markdown = "_(opencode returned no text — check that `opencode` is installed and that `" + model + "` is pulled locally.)_"
	}

	result := AnalysisResult{
		EventKey:    computeAnalysisKey(req),
		Source:      req.Source,
		Type:        "activity.adhoc",
		Title:       activityAnalysisTitle(req),
		Model:       model,
		Markdown:    markdown,
		ExitCode:    exitCode,
		Duration:    time.Since(start),
		WorkspaceID: req.WorkspaceID,
		CreatedAt:   time.Now(),
	}

	// Terminal event for the stream. "done" even on non-zero
	// exit when we have at least partial markdown — the caller
	// wants the partial output rendered. Only "error" when we
	// have nothing to show.
	if markdown == "" {
		select {
		case out <- ActivityAnalysisStreamEvent{
			Kind:  "error",
			Error: fmt.Sprintf("opencode exit %d", exitCode),
		}:
		default:
		}
	} else {
		select {
		case out <- ActivityAnalysisStreamEvent{Kind: "done"}:
		default:
		}
	}

	return result
}

// PersistActivityAnalysis indexes a completed AnalysisResult into
// glitch-analyses so it survives process restarts and shows up in
// future activity-panel queries. The parentID argument is added to
// the persisted doc as a metadata field so threaded chains can be
// reconstructed when the frontend queries by source or by chain
// root. Empty parentID is fine.
//
// Degrades to a no-op when es is nil — matches the pattern used by
// the autonomous Analyzer so tests can run without wiring ES.
func PersistActivityAnalysis(
	ctx context.Context,
	es *esearch.Client,
	result AnalysisResult,
	parentID string,
) {
	if es == nil || result.Markdown == "" {
		return
	}
	doc := map[string]any{
		"event_key":    result.EventKey,
		"source":       result.Source,
		"type":         result.Type,
		"title":        result.Title,
		"model":        result.Model,
		"markdown":     result.Markdown,
		"exit_code":    result.ExitCode,
		"duration_ms":  result.Duration.Milliseconds(),
		"workspace_id": result.WorkspaceID,
		"created_at":   result.CreatedAt,
	}
	if parentID != "" {
		doc["parent_analysis_id"] = parentID
	}
	if err := es.BulkIndex(ctx, esearch.IndexAnalyses, []any{doc}); err != nil {
		slog.Warn("activity analyzer: persist failed",
			"event_key", result.EventKey, "err", err)
	}
}

// computeAnalysisKey hashes the inputs of an activity analysis
// into a stable event_key. Two runs with identical inputs dedupe
// to the same doc in ES; adding the user prompt to the hash means
// refinements never collide with their parent analyses. The
// parent id participates too so the same refinement against two
// different parents stays distinct.
func computeAnalysisKey(req ActivityAnalyzeRequest) string {
	h := sha1.New()
	fmt.Fprintln(h, req.Source)
	fmt.Fprintln(h, req.WorkspaceID)
	fmt.Fprintln(h, req.UserPrompt)
	fmt.Fprintln(h, req.ParentAnalysisID)
	for _, d := range req.Docs {
		fmt.Fprintln(h, d.Source, d.SHA, d.URL, d.TimestampMs, d.Message)
	}
	return "activity:" + hex.EncodeToString(h.Sum(nil))[:20]
}

// activityAnalysisTitle builds the sidebar title for an activity
// analysis. Prefers the user's prompt when they asked a specific
// question; falls back to a generic "analyzed N docs from source"
// phrasing so the row still reads cleanly when the prompt is empty.
func activityAnalysisTitle(req ActivityAnalyzeRequest) string {
	p := strings.TrimSpace(req.UserPrompt)
	if p != "" {
		if len(p) > 60 {
			p = p[:57] + "…"
		}
		return p
	}
	src := req.Source
	if src == "" {
		src = "activity"
	}
	return fmt.Sprintf("analyzed %d %s doc(s)", len(req.Docs), src)
}

// userPromptOrPlaceholder returns the trimmed user prompt, or a
// placeholder string when empty. The placeholder signals the
// no-question case to the rubric so the model knows to produce a
// general analysis rather than looking for a specific answer.
func userPromptOrPlaceholder(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "(no specific question — give a general analysis of these documents)"
	}
	return p
}

// formatDocsForPrompt renders the document list into a compact
// human-readable block the model can anchor on. Each doc gets a
// short header (source / repo / author / timestamp / id) and its
// message body, truncated to keep the total prompt reasonable.
//
// We do NOT include the full raw JSON — that wastes tokens and
// makes the prompt fragile. The curated fields here are the ones
// the rubric actually asks the model to reason about.
func formatDocsForPrompt(docs []RecentEvent) string {
	if len(docs) == 0 {
		return "(no documents)"
	}
	var b strings.Builder
	for i, d := range docs {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "— Document %d —\n", i+1)
		fmt.Fprintf(&b, "source: %s\n", d.Source)
		if d.Type != "" {
			fmt.Fprintf(&b, "type: %s\n", d.Type)
		}
		if d.Repo != "" {
			fmt.Fprintf(&b, "repo: %s\n", d.Repo)
		}
		if d.Branch != "" {
			fmt.Fprintf(&b, "branch: %s\n", d.Branch)
		}
		if d.Author != "" {
			fmt.Fprintf(&b, "author: %s\n", d.Author)
		}
		if d.SHA != "" {
			fmt.Fprintf(&b, "sha: %s\n", d.SHA)
		}
		if d.URL != "" {
			fmt.Fprintf(&b, "url: %s\n", d.URL)
		}
		if d.TimestampMs > 0 {
			t := time.UnixMilli(d.TimestampMs).UTC().Format(time.RFC3339)
			fmt.Fprintf(&b, "timestamp: %s\n", t)
		}
		if msg := strings.TrimSpace(d.Message); msg != "" {
			fmt.Fprintf(&b, "message: %s\n", msg)
		}
		if body := strings.TrimSpace(d.Body); body != "" {
			if len(body) > 1500 {
				body = body[:1500] + "…"
			}
			fmt.Fprintf(&b, "body:\n%s\n", body)
		}
		if len(d.Files) > 0 {
			files := d.Files
			if len(files) > 20 {
				files = append(files[:20:20], "…")
			}
			fmt.Fprintf(&b, "files_changed: %s\n", strings.Join(files, ", "))
		}
	}
	return b.String()
}

// extractTextDelta parses one line of opencode --format json
// output and returns the text content if the line is a text event.
// Returns empty string for non-text events (tool calls, step
// boundaries) and for malformed lines — the caller ignores empty
// results.
//
// opencode's JSON stream shape is `{"type":"text","part":{"text":"..."}}`
// for text deltas. We use encoding/json here (unlike the
// deep_analysis extractor which does a best-effort substring pass)
// because we're streaming in real time and want to get this right
// without the substring fallback's edge cases.
func extractTextDelta(line string) string {
	if !strings.Contains(line, `"type":"text"`) {
		return ""
	}
	var ev struct {
		Type string `json:"type"`
		Part struct {
			Text string `json:"text"`
		} `json:"part"`
	}
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return ""
	}
	if ev.Type != "text" {
		return ""
	}
	return ev.Part.Text
}

// sendStreamError sends a terminal error event on the stream.
// Non-blocking: if the caller has already drained and bailed, the
// error drops on the floor rather than hanging the analyzer
// goroutine.
func sendStreamError(out chan<- ActivityAnalysisStreamEvent, msg string) {
	select {
	case out <- ActivityAnalysisStreamEvent{Kind: "error", Error: msg}:
	default:
	}
}
