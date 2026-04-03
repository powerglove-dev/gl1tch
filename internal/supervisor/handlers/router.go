package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/supervisor"
)

// Router routes natural-language utterances to the best-matching executor.
// It is NOT invoked from busd — callers call Route() directly.
type Router struct {
	execMgr *executor.Manager
	pub     EventPublisher
}

// NewRouter creates a Router.
func NewRouter(execMgr *executor.Manager, pub EventPublisher) *Router {
	return &Router{execMgr: execMgr, pub: pub}
}

// Name implements supervisor.Handler (for role lookup).
func (r *Router) Name() string { return "routing" }

// Topics returns nil — Router is not driven by busd events.
func (r *Router) Topics() []string { return nil }

// Handle is a no-op for Router; use Route() instead.
func (r *Router) Handle(_ context.Context, _ supervisor.Event, _ supervisor.ResolvedModel) error {
	return nil
}

// Route uses the "routing" role model to pick the best executor for utterance.
// Returns matched=true and the model response if an executor was dispatched;
// matched=false if no confident match was found (caller falls through to normal LLM).
func (r *Router) Route(ctx context.Context, utterance string, model supervisor.ResolvedModel, w io.Writer) (matched bool, response string, err error) {
	if r.execMgr == nil {
		return false, "", nil
	}

	// Build a list of available executors with their descriptions.
	executors := r.execMgr.List()
	if len(executors) == 0 {
		return false, "", nil
	}

	var catalog strings.Builder
	for _, e := range executors {
		fmt.Fprintf(&catalog, "- %s: %s\n", e.Name(), e.Description())
	}

	// Ask the model to pick the best match.
	routingPrompt := fmt.Sprintf(
		"You are a router. Given the user utterance below and the list of available executors, "+
			"reply with ONLY the executor name that best matches the intent, or reply \"none\" if no good match exists.\n\n"+
			"Available executors:\n%s\n"+
			"User utterance: %s\n\n"+
			"Reply with the executor name or \"none\":",
		catalog.String(), utterance,
	)

	var buf bytes.Buffer
	vars := map[string]string{"model": model.ModelID}
	if err := r.execMgr.Execute(ctx, model.ProviderID, routingPrompt, vars, &buf); err != nil {
		slog.Warn("router: model invocation failed", "err", err)
		return false, "", nil
	}

	chosen := strings.TrimSpace(buf.String())
	// Strip any surrounding quotes or punctuation from the model response.
	chosen = strings.Trim(chosen, `"'.,:;`)

	if strings.EqualFold(chosen, "none") || chosen == "" {
		return false, "", nil
	}

	// Verify the chosen name actually exists.
	_, ok := r.execMgr.Get(chosen)
	if !ok {
		slog.Warn("router: model chose unknown executor", "chosen", chosen)
		return false, "", nil
	}

	// Dispatch.
	var respBuf bytes.Buffer
	if w == nil {
		w = &respBuf
	}
	if err := r.execMgr.Execute(ctx, chosen, utterance, nil, w); err != nil {
		return false, "", fmt.Errorf("router: execute %q: %w", chosen, err)
	}
	return true, respBuf.String(), nil
}
