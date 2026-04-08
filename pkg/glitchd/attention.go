// attention.go runs the local attention classifier — a qwen2.5:7b
// call that decides, for each incoming AnalyzableEvent, whether the
// event should interrupt the user right now (`high`), belongs in
// the normal activity feed (`normal`), or is pure noise (`low`).
//
// This is stage 2 of the five-stage analysis ladder:
//
//	1. triage (triage.go)       — "anything here?"       → qwen2.5:7b
//	2. attention (this file)    — "does this touch me?"  → qwen2.5:7b
//	3. research (deep_analysis) — "gather context"       → opencode + coder
//	4. draft    (deep_analysis) — "produce the artifact" → opencode + coder
//	5. polish   (escalate, TBD) — "refine the draft"     → paid provider
//
// Why a separate classifier call and not a heuristic ladder in Go?
// Because the *rules* for what counts as high attention live in the
// user's workspace research prompt (see research_prompt.go), which
// is free-form markdown the user edits. No `if author == me` table,
// no severity map, no keyword list — the judgement is pushed to the
// local LLM which reads the research prompt alongside each batch.
// This is the AI-first rule applied to routing, not just generation.
//
// Failure mode: if Ollama is unreachable, the research prompt is
// missing, or the model returns garbage JSON, every event in the
// batch is stamped `normal` with an explanatory reason. That keeps
// the downstream deep-analysis path on its pre-classifier behaviour
// (plain summary rubric, cooldown enforced) rather than blocking
// the whole pipeline on a best-effort stage.
package glitchd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/capability"
)

// AttentionLevel is the three-valued verdict the classifier
// produces for each event. Levels are compared as strings elsewhere
// in the package so this is a string alias rather than an enum —
// keeps JSON (un)marshalling to the model trivial.
type AttentionLevel = string

const (
	// AttentionHigh means the event matches at least one rule in
	// the user's research prompt and should interrupt them: a
	// review on their PR, a failing check on their branch, a
	// direct mention. The deep analyzer runs in artifact mode
	// (deep_analysis_artifact.md) on these and bypasses the
	// per-process cooldown.
	AttentionHigh AttentionLevel = "high"

	// AttentionNormal is the default. The event is legitimate but
	// nothing in the research prompt flags it as personal. Deep
	// analysis runs in its standard summary mode and honours the
	// cooldown.
	AttentionNormal AttentionLevel = "normal"

	// AttentionLow is automation noise — bot commits, release
	// tags, dependabot bumps. Deep analysis is still allowed to
	// run (the eligibility filter in deep_analysis.go has the
	// final say) but the classifier is signalling that the event
	// would add no value even if the filter lets it through.
	AttentionLow AttentionLevel = "low"
)

// AttentionVerdict is one classifier decision. The Index field
// preserves alignment with the input slice so a reordered or
// dropped response from the model can still be reconciled.
type AttentionVerdict struct {
	Index  int            `json:"index"`
	Level  AttentionLevel `json:"level"`
	Reason string         `json:"reason"`
}

// ClassifierRelevant reports whether an event type is worth
// spending a classifier call on. The classifier is cheap per call
// (~1-5s on qwen2.5:7b in JSON mode) but becomes catastrophic at
// scale: a copilot.log backfill easily produces 30k events in a
// few ticks, and classifying all of them would pin the collector
// path for ~50 minutes.
//
// The allow-list here is deliberately narrow: only events that
// represent *external coordination signals directed at the user*.
// Container events (PRs, issues) are intentionally excluded —
// they describe state but aren't actionable on their own. What's
// actionable is what happens ON them: reviews, comments, checks.
//
// Why github.pr and github.issue are NOT in this list:
//
//   A PR existing isn't a coordination signal to its author —
//   they already know, they opened it. It isn't a signal to
//   anyone else either, until someone acts on it (reviews,
//   comments, check results). Classifying the bare PR event was
//   generating a second chat injection per PR that duplicated
//   the derived-event injection a few seconds later, because
//   qwen2.5:7b couldn't reliably follow the "my own activity →
//   normal" rule and kept hallucinating a reviewer on the
//   container event. Dropping the container types solves both
//   the duplicate-notification UX AND the classifier-accuracy
//   problem in one move.
//
//   The PRs and issues themselves still land in glitch-events
//   and are searchable via the observer. They just don't trigger
//   the attention funnel.
//
// Events rejected here still flow through the deep-analysis
// queue if they pass eligibleForAnalysis; they just get no
// Attention verdict stamped on them, which the downstream prompt
// logic treats as "summary mode" per buildAnalysisPrompt. In
// practice eligibleForAnalysis *also* gates on ClassifierRelevant,
// so rejection here cleanly removes them from the whole pipeline.
//
// When you add a new coordination source (e.g. gitlab MRs or
// slack mentions), add its type string here so classification
// starts firing against it automatically.
func ClassifierRelevant(eventType string) bool {
	switch eventType {
	case
		// External actions ON github PRs/issues — these are the
		// real coordination signals. A review, comment, or failing
		// check is someone asking the user for something.
		"github.pr_review",
		"github.pr_comment",
		"github.pr_check",
		"github.check",
		"github.issue_comment",
		// git commits land here so the classifier can flag
		// "someone pushed to main on a branch I'm working on".
		"git.commit",
		"git.push":
		return true
	}
	return false
}

// AttentionObserver is the callback fired once per classified
// event after ClassifyAttention stamps a verdict but before the
// event enters the deep-analysis queue. analysisEnabled reports
// whether the heavy runOne path is going to pick the event up —
// observers use it to distinguish "high-attention and working on
// it" from "high-attention and deep analysis is off, nudge the
// user".
//
// Registered via SetAttentionObserver. At most one observer is
// active process-wide; the desktop registers its own at startup.
// Leaving it nil disables the hook entirely — nothing observes.
type AttentionObserver func(ev AnalyzableEvent, analysisEnabled bool)

var (
	attentionObserverMu sync.RWMutex
	attentionObserver   AttentionObserver
)

// SetAttentionObserver registers the package-level attention
// observer. Passing nil clears it. Safe to call multiple times —
// later calls replace earlier ones, matching the pattern of
// SetEventSink in internal/capability.
//
// The desktop wires this at startup so high-attention verdicts
// can fan out to the chat pane and activity sidebar before the
// heavy analyzer has a chance to produce its artifact. Tests that
// exercise the classifier in isolation can leave it unset.
func SetAttentionObserver(obs AttentionObserver) {
	attentionObserverMu.Lock()
	attentionObserver = obs
	attentionObserverMu.Unlock()
}

// getAttentionObserver returns the current observer for callers
// inside this package that want to fire it. Returns nil when
// nothing is registered, which callers should treat as "skip the
// hook" rather than an error.
func getAttentionObserver() AttentionObserver {
	attentionObserverMu.RLock()
	defer attentionObserverMu.RUnlock()
	return attentionObserver
}

// attentionHTTPTimeout bounds a single classifier call. 90 seconds
// is generous by design: qwen2.5:7b on a freshly-booted Ollama
// (no keep-alive, cold VRAM) can take 20-40 seconds just to load
// the model before it even starts generating, and a batch of 5
// events then needs another 5-15s to actually classify. Production
// steady-state calls finish in well under 5s, so the 90s ceiling
// only bites on the first call after a restart — which is exactly
// when failing closed would hurt most, because every event defaults
// to `normal` and the user's first real interaction gets missed.
const attentionHTTPTimeout = 90 * time.Second

// attentionMaxBatch caps the number of events per classifier call.
// Set to 1 after batch=5 was proven to cause cross-event contamination:
// qwen2.5:7b on a 2-event batch would hallucinate fields from event 2
// onto event 1's verdict (e.g. "review from @amannocci" on an event
// where amannocci's name never appeared) and sometimes return fewer
// verdicts than events, leaving tail items with default-normal.
// One-event batches eliminate both failure modes, at the cost of N
// sequential ollama calls. At ~1-2s per warm call, a burst of 12
// github events takes under 30s — acceptable given the reliability
// gain. The "small local model can't multitask" lesson is load-bearing
// and worth the perf cost.
const attentionMaxBatch = 1

// ClassifyAttention runs the attention classifier against a batch
// of events and returns one verdict per event in input order.
//
// The function NEVER returns an error when the model is simply
// unreachable — in that case every event gets a `normal` verdict
// and the error is logged at warn level. This keeps the collector
// hot path free of ollama-dependent branching: callers can always
// trust the returned slice's length matches the input.
//
// An error is returned only for programming-level failures (nil
// events slice elements, inability to marshal the request) that
// indicate a bug in the caller.
func ClassifyAttention(
	ctx context.Context,
	events []AnalyzableEvent,
	workspaceID string,
) ([]AttentionVerdict, error) {
	if len(events) == 0 {
		return nil, nil
	}

	// Pre-allocate the result with every event stamped `normal`.
	// Any path that bails out below leaves this slice intact so the
	// caller gets a fully-populated, input-aligned response.
	verdicts := make([]AttentionVerdict, len(events))
	for i := range verdicts {
		verdicts[i] = AttentionVerdict{
			Index:  i,
			Level:  AttentionNormal,
			Reason: "classifier default (no override)",
		}
	}

	// Resolve the research prompt. Missing prompts are a fatal
	// install-level condition for the classifier (see
	// research_prompt.go), but we degrade to the default-normal
	// verdicts rather than bubbling an error up into the sink.
	research, err := LoadResearchPrompt(workspaceID)
	if err != nil {
		slog.Warn("attention: research prompt unavailable, defaulting to normal",
			"workspace_id", workspaceID, "err", err)
		for i := range verdicts {
			verdicts[i].Reason = "research prompt unavailable"
		}
		return verdicts, nil
	}

	// Resolve the user identity. Both values are injected into the
	// classifier prompt so it can match against event authors and
	// mention strings. Best-effort — empty values are fine.
	userName, userEmail := localGitIdentity()

	// Process in chunks. Each chunk is a fresh ollama call; the
	// first chunk's warm-start cost dominates, subsequent chunks
	// are cheap.
	for start := 0; start < len(events); start += attentionMaxBatch {
		end := start + attentionMaxBatch
		if end > len(events) {
			end = len(events)
		}
		batch := events[start:end]

		batchVerdicts, err := classifyAttentionBatch(
			ctx, batch, research, userName, userEmail)
		if err != nil {
			slog.Warn("attention: batch classify failed, defaulting to normal",
				"err", err, "batch_size", len(batch))
			// Leave this chunk's pre-filled `normal` verdicts in
			// place and continue to the next chunk.
			continue
		}
		// Re-base the returned indices onto the parent slice's
		// coordinate system and copy into the final result.
		for _, bv := range batchVerdicts {
			if bv.Index < 0 || bv.Index >= len(batch) {
				continue
			}
			abs := start + bv.Index
			verdicts[abs] = AttentionVerdict{
				Index:  abs,
				Level:  normalizeAttentionLevel(bv.Level),
				Reason: strings.TrimSpace(bv.Reason),
			}
			if verdicts[abs].Reason == "" {
				verdicts[abs].Reason = "no reason provided"
			}
		}
	}

	return verdicts, nil
}

// classifyAttentionBatch is one ollama round-trip over a chunk of
// events. Returns the parsed verdicts in the batch's own coordinate
// system (0-based within the chunk) so the caller can re-base onto
// the full event slice.
func classifyAttentionBatch(
	ctx context.Context,
	batch []AnalyzableEvent,
	researchPrompt, userName, userEmail string,
) ([]AttentionVerdict, error) {
	eventsJSON, err := marshalEventsForClassifier(batch)
	if err != nil {
		return nil, fmt.Errorf("marshal events: %w", err)
	}

	// Enrich the identity block with the github handle we can
	// infer from the git config email. GitHub's "keep my email
	// private" setting rewrites commits to use a noreply address
	// of the form "<id>+<handle>@users.noreply.github.com" — a
	// huge chunk of users end up with git user.name that doesn't
	// match their github login but an email that encodes it
	// exactly. Feeding the parsed handle into the classifier
	// prompt lets it match author fields on PR review events
	// without the user editing research.md first.
	githubHandle := parseGitHubHandleFromEmail(userEmail)

	prompt, err := RenderPrompt("attention_classifier", map[string]string{
		"USER_NAME":       userName,
		"USER_EMAIL":      userEmail,
		"USER_GITHUB":     githubHandle,
		"RESEARCH_PROMPT": researchPrompt,
		"EVENTS_JSON":     eventsJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("render classifier prompt: %w", err)
	}

	// Model selection mirrors the triage path: config.Model wins,
	// then the project-wide qwen2.5:7b default. We intentionally do
	// NOT use Config.Analysis.Model here — that knob points at the
	// tool-using coder model run through opencode (e.g.
	// qwen2.5-coder), which is overkill for a routing decision.
	// Classification is a language task, not a code task.
	model := "qwen2.5:7b"
	if cfg, _ := capability.LoadConfig(); cfg != nil && cfg.Model != "" {
		model = cfg.Model
	}

	type ollamaReq struct {
		Model   string         `json:"model"`
		Prompt  string         `json:"prompt"`
		Stream  bool           `json:"stream"`
		Format  string         `json:"format,omitempty"`
		Options map[string]any `json:"options,omitempty"`
	}
	type ollamaResp struct {
		Response string `json:"response"`
		Error    string `json:"error,omitempty"`
	}

	reqBody, err := json.Marshal(ollamaReq{
		Model:  model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
		Options: map[string]any{
			"temperature": 0.1,
			// Larger context than triage because the research
			// prompt alone can easily run 500+ tokens and we're
			// also carrying up to 20 events.
			"num_ctx": 8192,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, attentionHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, "POST",
		ollamaGenerateURL(),
		bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	hc := &http.Client{Timeout: attentionHTTPTimeout}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama %d: %s", resp.StatusCode, string(raw))
	}

	var or ollamaResp
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}
	if or.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", or.Error)
	}

	return parseClassifierResponse(or.Response, len(batch))
}

// marshalEventsForClassifier renders the batch as a compact JSON
// array the classifier prompt embeds verbatim. We pass the curated
// fields the classifier actually needs — type/source/repo/author/
// title/body/timestamp — and drop fields that are either too noisy
// for a small model (full URL) or irrelevant to routing
// (identifier, event key).
func marshalEventsForClassifier(batch []AnalyzableEvent) (string, error) {
	type classifierEvent struct {
		Index     int    `json:"index"`
		Source    string `json:"source"`
		Type      string `json:"type"`
		Repo      string `json:"repo,omitempty"`
		Author    string `json:"author,omitempty"`
		Title     string `json:"title,omitempty"`
		Body      string `json:"body,omitempty"`
		Timestamp string `json:"timestamp,omitempty"`
	}
	out := make([]classifierEvent, 0, len(batch))
	for i, ev := range batch {
		body := strings.TrimSpace(ev.Body)
		// Keep bodies short so a 20-event batch fits comfortably
		// inside the 8k context window we allocate for the call.
		if len(body) > 400 {
			body = body[:400] + "…"
		}
		ts := ""
		if !ev.Timestamp.IsZero() {
			ts = ev.Timestamp.UTC().Format(time.RFC3339)
		}
		out = append(out, classifierEvent{
			Index:     i,
			Source:    ev.Source,
			Type:      ev.Type,
			Repo:      ev.Repo,
			Author:    ev.Author,
			Title:     ev.Title,
			Body:      body,
			Timestamp: ts,
		})
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// parseClassifierResponse extracts the verdicts from the model's
// JSON reply. Tolerant of missing fields and extra whitespace; the
// caller has already stamped every slot with a `normal` default so
// anything we successfully parse is strictly an improvement.
//
// expectedLen is the batch size — verdicts with an index outside
// [0, expectedLen) are dropped since they can't be reconciled back
// onto the input slice.
func parseClassifierResponse(raw string, expectedLen int) ([]AttentionVerdict, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty classifier response")
	}

	var parsed struct {
		Verdicts []AttentionVerdict `json:"verdicts"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("decode classifier json: %w", err)
	}

	out := make([]AttentionVerdict, 0, len(parsed.Verdicts))
	for _, v := range parsed.Verdicts {
		if v.Index < 0 || v.Index >= expectedLen {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

// normalizeAttentionLevel coerces whatever the model returned into
// one of the three valid levels. Anything unrecognized degrades to
// `normal` — the safe middle — so a hallucinated "urgent" or
// "critical" doesn't silently bypass the cooldown.
func normalizeAttentionLevel(level string) AttentionLevel {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "high":
		return AttentionHigh
	case "low":
		return AttentionLow
	default:
		return AttentionNormal
	}
}

// localGitIdentity reads `git config user.name` and
// `git config user.email` to give the classifier a baseline
// identity to match against. Best-effort: missing git or empty
// values return empty strings, which the prompt tolerates (the
// research prompt is expected to list additional handles there
// anyway).
//
// We shell out rather than parse ~/.gitconfig directly because git
// applies a layered lookup (system → global → repo → includeIf)
// that would be painful to reimplement. The exec is cheap and runs
// at most once per classifier batch.
func localGitIdentity() (name, email string) {
	name = runGitConfig("user.name")
	email = runGitConfig("user.email")
	return name, email
}

// parseGitHubHandleFromEmail extracts the github login from a
// noreply email of the form "<id>+<handle>@users.noreply.github.com"
// (or the older "<handle>@users.noreply.github.com"). Returns an
// empty string when the address is not a github noreply, when the
// format is malformed, or when the handle is empty — callers are
// expected to treat empty as "don't inject a github handle into
// the classifier prompt".
//
// We parse this in Go rather than asking the LLM to infer the
// handle because (a) the format is precise and regex-clean, (b)
// user identity is load-bearing for classification accuracy, and
// (c) an LLM hallucinating a wrong handle would misclassify
// someone else's comments as belonging to the user.
func parseGitHubHandleFromEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	const suffix = "@users.noreply.github.com"
	if !strings.HasSuffix(email, suffix) {
		return ""
	}
	local := strings.TrimSuffix(email, suffix)
	// New format: "<id>+<handle>". Old format: "<handle>".
	if plus := strings.Index(local, "+"); plus >= 0 {
		return strings.TrimSpace(local[plus+1:])
	}
	return strings.TrimSpace(local)
}

// runGitConfig returns `git config --get <key>` stripped of
// trailing whitespace, or "" if git fails or the key is unset.
func runGitConfig(key string) string {
	cmd := exec.Command("git", "config", "--get", key)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ollamaGenerateURL returns the URL the classifier POSTs to. The
// default is the standard local Ollama endpoint; tests (and users
// running Ollama on a non-default port or remote host) can
// override via GLITCH_OLLAMA_URL, which should point at the
// server's base URL — the `/api/generate` suffix is appended
// automatically so the env value matches what Ollama prints in its
// own startup log.
func ollamaGenerateURL() string {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("GLITCH_OLLAMA_URL")), "/")
	if base == "" {
		base = "http://localhost:11434"
	}
	return base + "/api/generate"
}
