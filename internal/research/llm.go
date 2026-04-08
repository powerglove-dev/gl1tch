package research

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
	"github.com/8op-org/gl1tch/pkg/glitchproto"
)

// LLMFn is the seam every loop stage uses to talk to the local model. It is
// intentionally minimal: a prompt in, a string out. Tests inject a stub
// closure; production callers use NewOllamaLLM to wire up the existing
// pipeline.Run + executor=ollama path with qwen2.5:7b as the model.
//
// Keeping the interface this small means the loop has no opinion on which
// model is in use, no opinion on tool-use vs raw text, and no dependency on
// the assistant or router packages. Anything that can answer prompts is a
// valid LLMFn.
type LLMFn func(ctx context.Context, prompt string) (string, error)

// DefaultLocalModel is the model identifier the loop uses for plan, draft,
// critique, and judge calls when not overridden by the caller. Per the
// project's hard rules this stays as qwen2.5:7b.
const DefaultLocalModel = "qwen2.5:7b"

// NewOllamaLLM returns an LLMFn that runs each prompt as a one-step pipeline
// through the existing pipeline.Run path with executor=ollama and the given
// model. The mgr argument is the live executor manager from the host; tests
// must not call this constructor — they should use a stub LLMFn directly.
//
// The function intentionally does not pre-warm Ollama, retry on failure, or
// stream tokens. The loop calls it sequentially per stage and treats every
// failure as a stage error so the loop can decide whether to refine, escalate,
// or return a partial result. Streaming is a UX concern that belongs in the
// chat renderer, not in the loop.
func NewOllamaLLM(mgr *executor.Manager, model string) LLMFn {
	if model == "" {
		model = DefaultLocalModel
	}
	return func(ctx context.Context, prompt string) (string, error) {
		p := &pipeline.Pipeline{
			Name: "research-llm",
			Steps: []pipeline.Step{
				{
					ID:        "in",
					Type:      "input",
					NoBrain:   true, // research stages own their own context
					NoClarify: true,
				},
				{
					ID:        "ask",
					Executor:  "ollama",
					Model:     model,
					Prompt:    prompt,
					Needs:     []string{"in"},
					NoBrain:   true,
					NoClarify: true,
				},
				{
					ID:    "out",
					Type:  "output",
					Needs: []string{"ask"},
				},
			},
		}
		out, err := pipeline.Run(ctx, p, mgr, "",
			pipeline.WithSilentStatus(),
			pipeline.WithNoClarification(),
		)
		if err != nil {
			return "", fmt.Errorf("ollama %s: %w", model, err)
		}
		return cleanLLMOutput(out), nil
	}
}

// cleanLLMOutput strips the gl1tch-stats sentinel JSON, brain blocks, and
// other sidecar markers the executor appends to model output. Without this
// the planner and drafter would see the stats line as part of the model's
// reply and either confuse the parser (planner: looks for a JSON array,
// finds an object) or leak it into the user-visible answer (drafter:
// dumps the JSON into the chat as if the model had written it).
//
// We pipe the raw output through glitchproto.NewContentOnlyWriter, which
// is the same scrubber the chat renderer uses, so a draft inside the
// research loop and a draft inside the legacy chat path get the same
// cleanup rules without two implementations drifting.
func cleanLLMOutput(raw string) string {
	var buf bytes.Buffer
	w := glitchproto.NewContentOnlyWriter(&buf)
	_, _ = w.Write([]byte(raw))
	_ = w.Close()
	return strings.TrimSpace(buf.String())
}
