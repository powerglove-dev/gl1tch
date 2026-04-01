//go:build integration

package pipeline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/pipeline"
)

// smokeModel returns the model to use for smoke tests.
// Override with GLITCH_SMOKE_MODEL (e.g. "llama3.2:1b" in CI).
func smokeModel() string {
	if m := os.Getenv("GLITCH_SMOKE_MODEL"); m != "" {
		return m
	}
	return "llama3.2"
}

// smokeModelBase strips the tag so checkModelAvailable can match "llama3.2:1b" → "llama3.2".
func smokeModelBase(full string) string {
	if idx := strings.Index(full, ":"); idx >= 0 {
		return full[:idx]
	}
	return full
}

// ollamaGenerateStub returns a StubPlugin that calls the ollama HTTP API directly,
// bypassing the discovery/config layer so the smoke test runs on a fresh CI runner
// without a configured orcai installation.
func ollamaGenerateStub(model string) *executor.StubExecutor {
	return &executor.StubExecutor{
		ExecutorName: "ollama",
		PluginDesc: "ollama smoke stub",
		ExecuteFn: func(ctx context.Context, input string, _ map[string]string, w io.Writer) error {
			body, _ := json.Marshal(map[string]any{
				"model":  model,
				"prompt": input,
				"stream": false,
			})
			resp, err := http.Post("http://localhost:11434/api/generate", "application/json", bytes.NewReader(body))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			var result struct {
				Response string `json:"response"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return err
			}
			_, err = io.WriteString(w, result.Response)
			return err
		},
	}
}

// TestSmokePipeline_SingleStep verifies a one-step ollama pipeline runs without error
// and produces non-empty output. Intentionally minimal — just confirms the pipeline
// executor dispatches correctly and ollama is reachable.
func TestSmokePipeline_SingleStep(t *testing.T) {
	model := smokeModel()
	checkModelAvailable(t, smokeModelBase(model))

	mgr := executor.NewManager()
	if err := mgr.Register(ollamaGenerateStub(model)); err != nil {
		t.Fatalf("register stub: %v", err)
	}

	p := &pipeline.Pipeline{
		Name:    "smoke",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "ping",
				Executor: "ollama",
				Model:    model,
				Prompt:   `Reply with the single word "ok".`,
			},
		},
	}

	pub := &collectPublisher{}
	result, err := pipeline.Run(context.Background(), p, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	if strings.TrimSpace(result) == "" {
		t.Error("expected non-empty output")
	}
}
