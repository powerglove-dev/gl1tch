//go:build integration

package pipeline_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/picker"
	"github.com/powerglove-dev/gl1tch/internal/pipeline"
)

// checkModelAvailable skips the test if the named ollama model is not present.
func checkModelAvailable(t *testing.T, model string) {
	t.Helper()
	out, err := exec.Command("ollama", "list").Output()
	if err != nil {
		t.Skipf("ollama not available: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		// Each line looks like: "llama3.2:latest   <id>  <size>  ..."
		// Match on the model name prefix (before the colon or space).
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		// Strip tag if present.
		if idx := strings.Index(name, ":"); idx >= 0 {
			name = name[:idx]
		}
		if strings.EqualFold(name, model) {
			return
		}
	}
	t.Skipf("model not available: %s", model)
}

// buildManager constructs a executor.Manager with all providers registered.
func buildManager() *executor.Manager {
	providers := picker.BuildProviders()
	mgr := executor.NewManager()
	for _, prov := range providers {
		if prov.SidecarPath != "" {
			continue
		}
		binary := prov.Command
		if binary == "" {
			binary = prov.ID
		}
		_ = mgr.Register(executor.NewCliAdapter(prov.ID, prov.Label+" CLI adapter", binary, prov.PipelineArgs...))
	}
	return mgr
}

// collectPublisher is an EventPublisher that records all payloads published.
type collectPublisher struct {
	lines []string
}

func (c *collectPublisher) Publish(_ context.Context, _ string, payload []byte) error {
	c.lines = append(c.lines, string(payload))
	return nil
}

// ── YAML fixture tests ──────────────────────────────────────────────────────

func TestPipelineFullRun_Llama(t *testing.T) {
	checkModelAvailable(t, "llama3.2")

	f, err := os.Open("testdata/llama_pipeline.yaml")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	p, err := pipeline.Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	mgr := buildManager()
	pub := &collectPublisher{}

	result, err := pipeline.Run(context.Background(), p, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(result) == "" {
		t.Error("expected non-empty output from pipeline run")
	}
}

func TestPipelineFullRun_Qwen(t *testing.T) {
	checkModelAvailable(t, "qwen2.5")

	f, err := os.Open("testdata/qwen_pipeline.yaml")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	p, err := pipeline.Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	mgr := buildManager()
	pub := &collectPublisher{}

	result, err := pipeline.Run(context.Background(), p, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(result) == "" {
		t.Error("expected non-empty output from pipeline run")
	}
}

// ── In-memory single-step tests ─────────────────────────────────────────────

func TestAgentSingleStep_Llama(t *testing.T) {
	checkModelAvailable(t, "llama3.2")

	p := &pipeline.Pipeline{
		Name:    "agent-single-llama",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "ask",
				Executor: "ollama",
				Model:    "llama3.2",
				Prompt:   "Say hello in one sentence.",
			},
		},
	}

	mgr := buildManager()
	pub := &collectPublisher{}

	result, err := pipeline.Run(context.Background(), p, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(result) == "" {
		t.Error("expected non-empty output from in-memory pipeline run")
	}
}

func TestAgentSingleStep_Qwen(t *testing.T) {
	checkModelAvailable(t, "qwen2.5")

	p := &pipeline.Pipeline{
		Name:    "agent-single-qwen",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "ask",
				Executor: "ollama",
				Model:    "qwen2.5",
				Prompt:   "Say hello in one sentence.",
			},
		},
	}

	mgr := buildManager()
	pub := &collectPublisher{}

	result, err := pipeline.Run(context.Background(), p, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(result) == "" {
		t.Error("expected non-empty output from in-memory pipeline run")
	}
}

// ── Multi-step chain tests ───────────────────────────────────────────────────

// TestPipelineChain_Llama runs a multi-step DAG: model → builtin.assert → builtin.log.
// This exercises sequential step dependencies and builtin executors end-to-end.
func TestPipelineChain_Llama(t *testing.T) {
	checkModelAvailable(t, "llama3.2")

	f, err := os.Open("testdata/llama_chain_pipeline.yaml")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	p, err := pipeline.Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	mgr := buildManager()
	pub := &collectPublisher{}

	_, err = pipeline.Run(context.Background(), p, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestPipelineChain_Qwen is the same chain test using qwen2.5 built in-memory.
func TestPipelineChain_Qwen(t *testing.T) {
	checkModelAvailable(t, "qwen2.5")

	p := &pipeline.Pipeline{
		Name:        "test-qwen-chain",
		Version:     "1",
		MaxParallel: 2,
		Steps: []pipeline.Step{
			{
				ID:       "ask",
				Executor: "ollama",
				Model:    "qwen2.5:latest",
				Prompt:   "Say hello in one sentence.",
			},
			{
				ID:       "assert-non-empty",
				Executor: "builtin.assert",
				Needs:    []string{"ask"},
				Args: map[string]any{
					"condition": "not_empty",
					"value":     "{{step.ask.data.value}}",
					"message":   "qwen2.5 returned empty output",
				},
			},
			{
				ID:       "log-result",
				Executor: "builtin.log",
				Needs:    []string{"assert-non-empty"},
				Args: map[string]any{
					"message": "qwen2.5 said: {{step.ask.data.value}}",
				},
			},
		},
	}

	mgr := buildManager()
	pub := &collectPublisher{}

	_, err := pipeline.Run(context.Background(), p, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ── Parallel pipeline tests ──────────────────────────────────────────────────

// TestPipelineParallel_LlamaQwen runs llama3.2 and qwen2.5 in parallel, then logs
// both results. Verifies the DAG runner launches independent steps concurrently.
func TestPipelineParallel_LlamaQwen(t *testing.T) {
	checkModelAvailable(t, "llama3.2")
	checkModelAvailable(t, "qwen2.5")

	f, err := os.Open("testdata/parallel_pipeline.yaml")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	p, err := pipeline.Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	mgr := buildManager()
	pub := &collectPublisher{}

	_, err = pipeline.Run(context.Background(), p, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

