package glitchd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/capability"
)

// TriageEvent is a single observation passed into the triage prompt.
// We deliberately keep this small — the local model is rate-limited
// by VRAM, not network, so the cost of stuffing 50 commits into a
// prompt is real. Type/Source/Repo/Author/Message is enough for the
// model to spot anomalies, regressions, and security-shaped chatter.
type TriageEvent struct {
	Type      string    `json:"type"`
	Source    string    `json:"source"`
	Repo      string    `json:"repo,omitempty"`
	Author    string    `json:"author,omitempty"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// TriageAlert is one finding the model wants the user to look at.
// The whole point of the loop: turn raw activity into a curated set
// of "you should look at this" pings rather than letting the user
// drown in a firehose of indexed events.
type TriageAlert struct {
	// Severity drives both the brain icon (warn/error promote it to
	// "alert") and whether the systray sees the entry.
	Severity string `json:"severity"` // "info" | "warn" | "error"
	Title    string `json:"title"`
	Why      string `json:"why"`
	Source   string `json:"source,omitempty"`
}

// TriageResult is the structured output from a single triage call.
// stored is what the model thinks belongs in long-term memory but
// doesn't necessarily warrant a notification.
type TriageResult struct {
	Alerts []TriageAlert `json:"alerts"`
	Stored []struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	} `json:"stored,omitempty"`
}

// QueryRecentEvents pulls events newer than sinceMs from glitch-events
// and returns up to limit of them, newest first. When workspaceID is
// non-empty the query is scoped to that workspace (including the
// shared tool-pod bucket) so the triage loop only surfaces events the
// user would expect to see in the active workspace.
func QueryRecentEvents(ctx context.Context, sinceMs int64, limit int, workspaceID string) ([]TriageEvent, error) {
	cfg, err := capability.LoadConfig()
	if err != nil {
		return nil, err
	}
	addr := cfg.Elasticsearch.Address
	if addr == "" {
		addr = "http://localhost:9200"
	}
	if limit <= 0 {
		limit = 50
	}

	var body string
	if workspaceID != "" {
		body = fmt.Sprintf(`{
			"size": %d,
			"sort": [{ "timestamp": "desc" }],
			"_source": ["type","source","repo","author","message","timestamp"],
			"query": {
				"bool": {
					"must": [
						{ "range": { "timestamp": { "gte": %d, "format": "epoch_millis" } } }
					],
					"filter": {
						"bool": {
							"should": [
								{ "term": { "workspace_id": %q } },
								{ "term": { "workspace_id": %q } }
							],
							"minimum_should_match": 1
						}
					}
				}
			}
		}`, limit, sinceMs, workspaceID, capability.WorkspaceIDTools)
	} else {
		body = fmt.Sprintf(`{
			"size": %d,
			"sort": [{ "timestamp": "desc" }],
			"_source": ["type","source","repo","author","message","timestamp"],
			"query": {
				"range": {
					"timestamp": { "gte": %d, "format": "epoch_millis" }
				}
			}
		}`, limit, sinceMs)
	}

	url := fmt.Sprintf("%s/glitch-events/_search", addr)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	hc := &http.Client{Timeout: 5 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return []TriageEvent{}, nil
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elasticsearch %d: %s", resp.StatusCode, string(raw))
	}

	var parsed struct {
		Hits struct {
			Hits []struct {
				Source TriageEvent `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	out := make([]TriageEvent, 0, len(parsed.Hits.Hits))
	for _, h := range parsed.Hits.Hits {
		out = append(out, h.Source)
	}
	return out, nil
}

// TriageEvents calls Ollama with a structured triage prompt over the
// given events and returns the model's findings. Returns an empty
// result + nil error when ollama isn't reachable so the caller can
// degrade to "no alerts this tick" silently.
//
// model defaults to the observer.yaml `model` setting (typically
// llama3.2 or similar small instruct model). The prompt is shaped
// around small models — JSON-only output, terse instructions, no
// chain-of-thought.
func TriageEvents(ctx context.Context, events []TriageEvent, model string) (*TriageResult, error) {
	if len(events) == 0 {
		return &TriageResult{}, nil
	}
	if model == "" {
		cfg, _ := capability.LoadConfig()
		if cfg != nil {
			model = cfg.Model
		}
	}
	if model == "" {
		model = "qwen2.5:7b"
	}

	// Compact event list — one line per event so the model can hold
	// 50 of them in working memory without truncation pain.
	var b strings.Builder
	for i, e := range events {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "[%s] %s", e.Source, e.Type)
		if e.Repo != "" {
			fmt.Fprintf(&b, " · %s", e.Repo)
		}
		if e.Author != "" {
			fmt.Fprintf(&b, " · @%s", e.Author)
		}
		msg := strings.TrimSpace(e.Message)
		if len(msg) > 240 {
			msg = msg[:237] + "…"
		}
		if msg != "" {
			fmt.Fprintf(&b, ": %s", msg)
		}
	}

	prompt := triageSystemPrompt + "\n\nEVENTS:\n" + b.String() + "\n\nRespond with JSON only."

	type ollamaReq struct {
		Model  string         `json:"model"`
		Prompt string         `json:"prompt"`
		Stream bool           `json:"stream"`
		Format string         `json:"format,omitempty"`
		Options map[string]any `json:"options,omitempty"`
	}
	type ollamaResp struct {
		Response string `json:"response"`
		Error    string `json:"error,omitempty"`
	}

	body, err := json.Marshal(ollamaReq{
		Model:  model,
		Prompt: prompt,
		Stream: false,
		Format: "json", // ollama JSON mode — guarantees parseable output
		Options: map[string]any{
			"temperature": 0.2,
			"num_ctx":     4096,
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"http://localhost:11434/api/generate",
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Triage is best-effort and we don't want it blocking the brain
	// loop forever — give the model 60s to chew on the buffer.
	hc := &http.Client{Timeout: 60 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		// Ollama not reachable → empty result, no error. Brain state
		// already shows red via the ollama ping.
		return &TriageResult{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama %d: %s", resp.StatusCode, string(raw))
	}

	var or ollamaResp
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return nil, err
	}
	if or.Error != "" {
		return nil, fmt.Errorf("ollama: %s", or.Error)
	}

	var result TriageResult
	if err := json.Unmarshal([]byte(or.Response), &result); err != nil {
		// Model returned malformed JSON despite format="json". Return
		// an empty result rather than blowing up — triage is advisory.
		return &TriageResult{}, nil
	}
	// Normalize severity values so the desktop's brain state machine
	// doesn't have to do it.
	for i, a := range result.Alerts {
		s := strings.ToLower(strings.TrimSpace(a.Severity))
		switch s {
		case "warning", "warn":
			s = "warn"
		case "critical", "err", "error":
			s = "error"
		default:
			s = "info"
		}
		result.Alerts[i].Severity = s
	}
	return &result, nil
}

// triageSystemPrompt is the instruction we hand to the local model.
// Kept short and explicit so 7B/8B instruct models can follow it.
//
// Design notes:
//   - We tell it to return JSON only because format="json" alone
//     doesn't always produce *useful* JSON — small models sometimes
//     emit valid JSON containing prose.
//   - We give concrete examples of what counts as an alert so it
//     doesn't go either too noisy ("git commit happened!") or too
//     quiet ("nothing to report" on every tick).
//   - We tell it to be skeptical of its own findings — small models
//     hallucinate confidently. Skepticism nudges precision over recall.
const triageSystemPrompt = `You are gl1tch's local triage brain. You read a list of recent dev-activity events
(git commits, GitHub PRs, chat messages, CI runs, …) and decide which ones the
user should look at right now.

Output a JSON object: { "alerts": [...], "stored": [...] }

For "alerts", include only events that meet at least one bar:
  - a CI/build/release pipeline FAILED
  - a security/credential/permission concern (leaked key, force-push to main, etc.)
  - an unexpected regression (revert, hotfix, "fix:" after a recent merge)
  - a coordination ping the user might miss (mention, review request, blocking comment)
  - an anomaly in volume or pattern (10+ commits from one author at once, sudden silence)

Each alert: { "severity": "info"|"warn"|"error", "title": "<≤60 chars>",
              "why": "<one sentence>", "source": "<source name>" }

For "stored", include up to 3 short summaries the user might want to remember
later. Skip routine commits and unrelated chatter.

Rules:
  - Return JSON ONLY. No prose, no markdown, no preamble.
  - If nothing meets the alert bar, return { "alerts": [], "stored": [] }.
  - Be skeptical: it's better to under-alert than to cry wolf.
  - Severity "error" is for failures/regressions, "warn" for things to look at,
    "info" for FYI. Default to "info" when in doubt.`
