package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// PhraseGenerator generates representative imperative trigger phrases for a
// pipeline. Implementations must be safe for concurrent use.
//
// Generated phrases are cached alongside the pipeline embedding (keyed by the
// pipeline hash) so the LLM is called at most once per pipeline version.
type PhraseGenerator interface {
	GeneratePhrases(ctx context.Context, name, description string) ([]string, error)
}

// LLMPhraseGenerator generates trigger phrases via a single Ollama call.
// Phrases are short imperative invocation strings used as embedding targets.
type LLMPhraseGenerator struct {
	mgr   *executor.Manager
	model string
}

// NewLLMPhraseGenerator creates a generator that uses mgr and model.
func NewLLMPhraseGenerator(mgr *executor.Manager, model string) *LLMPhraseGenerator {
	return &LLMPhraseGenerator{mgr: mgr, model: model}
}

// GeneratePhrases asks the LLM for 3–5 short imperative phrases that a user
// would type to explicitly invoke the named pipeline.
func (g *LLMPhraseGenerator) GeneratePhrases(ctx context.Context, name, description string) ([]string, error) {
	p := &pipeline.Pipeline{
		Name:    "phrase-gen",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "gen",
				Executor: "ollama",
				Model:    g.model,
				Prompt:   buildPhrasesPrompt(name, description),
			},
		},
	}

	raw, err := pipeline.Run(ctx, p, g.mgr, "", pipeline.WithSilentStatus(), pipeline.WithNoClarification())
	if err != nil {
		return nil, fmt.Errorf("phrases: llm call: %w", err)
	}
	return parsePhrasesResponse(raw)
}

func buildPhrasesPrompt(name, description string) string {
	return fmt.Sprintf(`Generate 3 to 5 short imperative phrases a user would type to explicitly invoke this pipeline.
Each phrase should start with a verb like "run", "execute", "launch", or "trigger".
Return ONLY a JSON array of strings — no explanation, no other text.

Pipeline name: %s
Description: %s

Example output: ["run %s", "execute %s", "launch %s pipeline"]

Respond with ONLY a JSON array:`, name, description, name, name, name)
}

// parsePhrasesResponse extracts the first JSON array from raw LLM output,
// sanitizes each phrase, and returns up to 8 non-empty strings.
func parsePhrasesResponse(raw string) ([]string, error) {
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("phrases: no JSON array in response")
	}

	var phrases []string
	if err := json.Unmarshal([]byte(raw[start:end+1]), &phrases); err != nil {
		return nil, fmt.Errorf("phrases: parse response: %w", err)
	}

	out := make([]string, 0, len(phrases))
	for _, p := range phrases {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
		if len(out) == 8 {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("phrases: no valid phrases in response")
	}
	return out, nil
}
