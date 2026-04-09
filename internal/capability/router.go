package capability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultLocalModel is the single source of truth for the default local
// Ollama model used anywhere inside gl1tch for generation, routing, query
// synthesis, and classification. Pinned to qwen2.5:7b — fast, reliable
// JSON output, no thinking-mode overhead. Heavier models (qwen3-coder,
// qwen3.5) are used for deep analysis where latency is acceptable.
const DefaultLocalModel = "qwen2.5:7b"

// DefaultRouterModel is an alias kept for clarity at the router call site.
// Prefer DefaultLocalModel in new code.
const DefaultRouterModel = DefaultLocalModel

// DefaultRouterBaseURL is the Ollama endpoint Router talks to. Overridable
// via Router.BaseURL for tests (the test suite stands up a fake server and
// points the router at it).
const DefaultRouterBaseURL = "http://localhost:11434"

// ErrNoMatch is returned by Pick / Route when the local model decides no
// registered capability is a good fit for the user message. The caller is
// expected to fall back to a direct LLM answer in that case.
var ErrNoMatch = errors.New("router: no capability matched")

// Router picks the best-matching on-demand capability for a user message
// by asking a local LLM to choose a name from the registry's manifest
// descriptions, then invokes it through the runner.
//
// Only TriggerOnDemand capabilities are presented to the model. Interval
// and daemon capabilities are background workers — nothing the user would
// ask for by name — so including them in the picker would waste tokens and
// increase misrouting risk on a 7B model.
//
// The router never lets the model construct commands or arguments. The
// model sees capability names and descriptions; it picks one; the runner
// executes the capability's declared invocation deterministically. This
// keeps prompt-injection attack surface minimal and keeps background
// ticks out of the LLM path entirely.
type Router struct {
	reg    *Registry
	runner *Runner

	// Model is the Ollama model used for routing. Defaults to
	// DefaultRouterModel (qwen2.5:7b).
	Model string
	// BaseURL overrides the Ollama endpoint. Defaults to
	// DefaultRouterBaseURL (http://localhost:11434).
	BaseURL string
	// HTTPClient overrides the HTTP client. Tests inject a client
	// pointing at a httptest.Server; production uses http.DefaultClient.
	HTTPClient *http.Client
	// Timeout caps how long the router waits for the model's pick.
	// Defaults to 30s. A 7B model usually answers in under two seconds
	// for the short system prompt the router sends.
	Timeout time.Duration
}

// NewRouter constructs a Router bound to the given registry + runner. The
// runner's Invoke is what actually executes the picked capability; the
// router's only job is selecting the name.
func NewRouter(reg *Registry, runner *Runner) *Router {
	return &Router{reg: reg, runner: runner}
}

// Pick returns the best-matching on-demand capability name for the message
// without invoking it. Returns ErrNoMatch when the model declines to pick.
// Exposed so callers can log / confirm / show a preview before executing.
func (r *Router) Pick(ctx context.Context, message string) (string, error) {
	candidates := r.candidates()
	if len(candidates) == 0 {
		return "", ErrNoMatch
	}

	timeout := r.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	askCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	prompt := buildRoutingPrompt(message, candidates)
	raw, err := r.ask(askCtx, prompt)
	if err != nil {
		return "", fmt.Errorf("router: ask model: %w", err)
	}

	name := parsePickedName(raw, candidates)
	if name == "" {
		return "", ErrNoMatch
	}
	return name, nil
}

// Route picks the best-matching capability for the message and invokes it,
// streaming Stream events to w. Returns the picked capability name and any
// invocation error. Pass io.Discard to drop stream output.
//
// Pipeline:
//
//  1. Filter the registry to on-demand capabilities.
//  2. Ask the local model to pick one.
//  3. Invoke the picked capability via the runner.
//
// The user message is passed to the capability as Input.Stdin, which is
// what scriptCapability writes to the subprocess's stdin. AI provider
// plugins (claude, ollama, etc.) consume stdin as the user prompt, so this
// gives you "route a user message to the right model" behaviour for free.
func (r *Router) Route(ctx context.Context, message string, w io.Writer) (string, error) {
	name, err := r.Pick(ctx, message)
	if err != nil {
		return "", err
	}
	if err := r.runner.Invoke(ctx, name, Input{Stdin: message}, w); err != nil {
		return name, err
	}
	return name, nil
}

// candidates returns the list of on-demand capabilities currently
// registered. The model picks from this filtered view, not from the full
// registry.
func (r *Router) candidates() []Capability {
	all := r.reg.List()
	out := make([]Capability, 0, len(all))
	for _, c := range all {
		if c.Manifest().Trigger.Mode == TriggerOnDemand {
			out = append(out, c)
		}
	}
	return out
}

// ask sends a single chat-completion request to Ollama and returns the
// assistant message content. Kept small and dependency-free — the router
// doesn't need streaming, retry, or tool-use support.
func (r *Router) ask(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model": r.modelName(),
		"messages": []map[string]string{
			{"role": "system", "content": routerSystemPrompt},
			{"role": "user", "content": prompt},
		},
		"stream": false,
		"options": map[string]any{
			// Low temperature — we want a deterministic pick, not
			// creative writing.
			"temperature": 0.0,
		},
	})
	if err != nil {
		return "", err
	}

	base := r.BaseURL
	if base == "" {
		base = DefaultRouterBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := r.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Message.Content, nil
}

func (r *Router) modelName() string {
	if r.Model != "" {
		return r.Model
	}
	return DefaultRouterModel
}

// routerSystemPrompt is what the model sees before the candidate list.
// Short, specific, and explicit about the expected response format so a
// 7B model doesn't editorialise or suggest commands of its own.
const routerSystemPrompt = `You are a capability router for the gl1tch AI assistant.

Given a user message and a list of available capabilities, pick the single best capability to handle the message. Respond with ONLY the capability name (exactly as written, lowercase, no punctuation), or the single word "none" if no capability fits.

Do NOT explain your choice. Do NOT repeat the name twice. Do NOT add quotes. One word, on one line.`

// buildRoutingPrompt produces the per-call user-turn that lists candidates
// and embeds the actual user message. Kept small so the prompt stays well
// under the 7B model's attention-reliable range.
func buildRoutingPrompt(userMsg string, candidates []Capability) string {
	var sb strings.Builder
	sb.WriteString("Available capabilities:\n")
	for _, c := range candidates {
		m := c.Manifest()
		desc := firstLine(m.Description)
		sb.WriteString("- ")
		sb.WriteString(m.Name)
		if desc != "" {
			sb.WriteString(": ")
			sb.WriteString(desc)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\nUser message: ")
	sb.WriteString(strings.TrimSpace(userMsg))
	sb.WriteString("\n\nCapability name:")
	return sb.String()
}

// parsePickedName extracts a capability name from the model's raw reply.
// Tolerates the common shapes a 7B model produces even when told to be
// terse: leading/trailing whitespace, quotes, a trailing period, a
// "Capability name:" echo, or multiple lines where only the last one is
// the answer. Returns the empty string if nothing matches a registered
// capability name or the model said "none".
func parsePickedName(raw string, candidates []Capability) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Take the last non-empty line — 7B models sometimes preface with
	// "Based on the message, the best capability is:" even when told
	// not to. The last line is almost always the actual answer.
	lines := strings.Split(raw, "\n")
	var candidate string
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i])
		if s != "" {
			candidate = s
			break
		}
	}
	// Strip a leading bullet / number, trailing punctuation, quotes.
	candidate = strings.Trim(candidate, "-*•. \"'`")
	// Some models prefix with "capability name:" — drop it.
	if i := strings.LastIndex(strings.ToLower(candidate), "name:"); i >= 0 {
		candidate = strings.TrimSpace(candidate[i+len("name:"):])
	}
	candidate = strings.Trim(candidate, "-*•. \"'`")
	// Split on whitespace and take the first token; capability names
	// never contain spaces, but the model might tack an explanation on.
	if i := strings.IndexAny(candidate, " \t"); i >= 0 {
		candidate = candidate[:i]
	}
	candidate = strings.ToLower(candidate)
	if candidate == "" || candidate == "none" {
		return ""
	}
	// Final guard: the returned name MUST match a registered
	// capability exactly. No fuzzy matching — a misroute caused by a
	// hallucinated name would be a nasty debugging surface.
	for _, c := range candidates {
		if strings.ToLower(c.Manifest().Name) == candidate {
			return c.Manifest().Name
		}
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
