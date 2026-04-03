//go:build !integration

package router

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"sync/atomic"
	"testing"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// ── HybridRouter helpers ──────────────────────────────────────────────────────

// makeMgr creates a manager with an ollama stub that writes responseJSON.
func makeMgr(t *testing.T, responseJSON string) *executor.Manager {
	t.Helper()
	mgr := executor.NewManager()
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "ollama",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := fmt.Fprint(w, responseJSON)
			return err
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return mgr
}

// makeErrMgr creates a manager whose ollama stub always errors.
func makeErrMgr(t *testing.T) *executor.Manager {
	t.Helper()
	mgr := executor.NewManager()
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "ollama",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			return fmt.Errorf("llm unavailable")
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return mgr
}

// trackingEmbedder counts embed calls and delegates to inner.
type trackingEmbedder struct {
	inner Embedder
	calls int64
}

func (te *trackingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	atomic.AddInt64(&te.calls, 1)
	return te.inner.Embed(ctx, text)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestHybridRouter_EmptyPipelines(t *testing.T) {
	mgr := makeMgr(t, `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`)
	emb := &fixedEmbedder{vec: []float32{1, 0}}
	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, emb, cfg)

	result, err := r.Route(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline != nil {
		t.Errorf("expected nil pipeline for empty list, got %q", result.Pipeline.Name)
	}
	if result.Method != "none" {
		t.Errorf("expected method 'none', got %q", result.Method)
	}
}

func TestHybridRouter_EmbeddingFastPath(t *testing.T) {
	// Prompt vector exactly matches pipeline-a's description vector → cosine = 1.0 > 0.85
	// LLM stub should NOT be called.
	llmCalled := int64(0)
	mgr := executor.NewManager()
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "ollama",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			atomic.AddInt64(&llmCalled, 1)
			_, err := fmt.Fprint(w, `{"pipeline":"pipeline-b","confidence":0.90,"input":"","cron":""}`)
			return err
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	pipelines := []pipeline.PipelineRef{
		{Name: "pipeline-a", Description: "unit vector alpha"},
		{Name: "pipeline-b", Description: "unit vector beta"},
	}

	unitA := []float32{1, 0, 0}
	unitB := []float32{0, 1, 0}

	embedFn := func(text string) []float32 {
		switch text {
		case "unit vector alpha":
			return unitA
		case "unit vector beta":
			return unitB
		default: // query
			return unitA // identical to pipeline-a → cosine = 1.0
		}
	}
	emb := &funcEmbedder{fn: embedFn}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, emb, cfg)

	// "run pipeline-a" is an explicit invocation verb → isImperativeInput = true
	result, err := r.Route(context.Background(), "run pipeline-a", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline == nil {
		t.Fatal("expected pipeline match, got nil")
	}
	if result.Pipeline.Name != "pipeline-a" {
		t.Errorf("expected pipeline-a, got %q", result.Pipeline.Name)
	}
	if result.Method != "embedding" {
		t.Errorf("expected method 'embedding', got %q", result.Method)
	}
	if atomic.LoadInt64(&llmCalled) != 0 {
		t.Error("LLM was called even though embedding was confident — should not have been")
	}
}

func TestHybridRouter_NoCandidates_SkipsLLM(t *testing.T) {
	// Orthogonal vectors → cosine = 0 < CandidateGateThreshold (0.40) → negative
	// filter fires immediately. LLM must NOT be called; result is method="none".
	llmCalled := int64(0)
	mgr := executor.NewManager()
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "ollama",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			atomic.AddInt64(&llmCalled, 1)
			_, err := fmt.Fprint(w, `{"pipeline":"pipeline-a","confidence":0.80,"input":"","cron":""}`)
			return err
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	pipelines := []pipeline.PipelineRef{
		{Name: "pipeline-a", Description: "alpha"},
	}

	// Description gets one vector, query gets orthogonal vector → cosine = 0 < gate
	embedFn := func(text string) []float32 {
		if text == "alpha" {
			return []float32{1, 0}
		}
		return []float32{0, 1}
	}
	emb := &funcEmbedder{fn: embedFn}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, emb, cfg)

	result, err := r.Route(context.Background(), "something unrelated", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if atomic.LoadInt64(&llmCalled) != 0 {
		t.Error("LLM must not be called when no candidates clear the gate threshold")
	}
	if result.Method != "none" {
		t.Errorf("expected method 'none' from negative filter, got %q", result.Method)
	}
	if result.Pipeline != nil {
		t.Errorf("expected nil pipeline from negative filter, got %q", result.Pipeline.Name)
	}
}

func TestHybridRouter_LLMMatch(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes to remote"},
	}

	mgr := makeMgr(t, `{"pipeline":"git-push","confidence":0.92,"input":"feature branch","cron":""}`)

	embedFn := func(text string) []float32 {
		if text == "Push changes to remote" {
			return []float32{1, 0}
		}
		return []float32{0.6, 0.8} // cosine=0.6 > gate(0.40), < fast-path(0.85) → LLM
	}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	result, err := r.Route(context.Background(), "run git-push on feature branch", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline == nil {
		t.Fatal("expected pipeline match, got nil")
	}
	if result.Pipeline.Name != "git-push" {
		t.Errorf("expected git-push, got %q", result.Pipeline.Name)
	}
	if result.Confidence != 0.92 {
		t.Errorf("expected confidence 0.92, got %f", result.Confidence)
	}
	if result.Method != "llm" {
		t.Errorf("expected method 'llm', got %q", result.Method)
	}
}

func TestHybridRouter_LLMReturnsNONE(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes to remote"},
	}
	mgr := makeMgr(t, `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`)
	embedFn := func(text string) []float32 {
		if text == "Push changes to remote" {
			return []float32{1, 0}
		}
		return []float32{0.6, 0.8} // above gate → passes to LLM which returns NONE
	}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	result, err := r.Route(context.Background(), "unrelated question", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline != nil {
		t.Errorf("expected nil pipeline for NONE, got %q", result.Pipeline.Name)
	}
}

func TestHybridRouter_LLMError_ReturnsNone(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes"},
	}
	mgr := makeErrMgr(t)
	embedFn := func(text string) []float32 {
		if text == "Push changes" {
			return []float32{1, 0}
		}
		return []float32{0.6, 0.8} // above gate → candidate passes to LLM which errors
	}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	result, err := r.Route(context.Background(), "something", pipelines)
	if err != nil {
		t.Fatalf("Route should not surface LLM errors, got: %v", err)
	}
	if result.Pipeline != nil {
		t.Errorf("expected nil pipeline on LLM error, got %q", result.Pipeline.Name)
	}
	if result.Method != "none" {
		t.Errorf("expected method 'none' on LLM error, got %q", result.Method)
	}
}

func TestHybridRouter_DisableEmbeddings(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes"},
	}

	emb := &trackingEmbedder{
		inner: &fixedEmbedder{vec: []float32{1, 0}},
	}

	mgr := makeMgr(t, `{"pipeline":"git-push","confidence":0.90,"input":"","cron":""}`)
	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
		DisableEmbeddings:  true,
	}
	r := New(mgr, emb, cfg)

	result, err := r.Route(context.Background(), "push my code", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	// Embedding was disabled — method must be "llm" not "embedding"
	if result.Method == "embedding" {
		t.Error("expected embedding stage to be skipped, but method is 'embedding'")
	}
	if atomic.LoadInt64(&emb.calls) > 0 {
		t.Errorf("expected 0 embed calls when DisableEmbeddings=true, got %d", emb.calls)
	}
}

func TestHybridRouter_ConfidenceThreshold(t *testing.T) {
	// Confidence 0.60 is below AmbiguousThreshold 0.65 — result should have nil pipeline
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes"},
	}
	mgr := makeMgr(t, `{"pipeline":"git-push","confidence":0.60,"input":"","cron":""}`)
	embedFn := func(text string) []float32 {
		if text == "Push changes" {
			return []float32{1, 0}
		}
		return []float32{0.6, 0.8} // above gate → passes to LLM (prompt ends with "?" → not fast path)
	}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	result, err := r.Route(context.Background(), "maybe push?", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline != nil {
		t.Errorf("expected nil pipeline below ambiguous threshold, got %q", result.Pipeline.Name)
	}
}

func TestHybridRouter_ExtractsInput(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "docs-improve", Description: "Improve documentation"},
	}
	mgr := makeMgr(t, `{"pipeline":"docs-improve","confidence":0.88,"input":"executor package","cron":""}`)
	embedFn := func(text string) []float32 {
		if text == "Improve documentation" {
			return []float32{1, 0}
		}
		return []float32{0.6, 0.8} // above gate, below fast-path → LLM
	}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	result, err := r.Route(context.Background(), "run docs-improve on executor package", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Input != "executor package" {
		t.Errorf("expected input %q, got %q", "executor package", result.Input)
	}
}

func TestHybridRouter_ExtractsCron(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "docs-improve", Description: "Improve documentation"},
	}
	mgr := makeMgr(t, `{"pipeline":"docs-improve","confidence":0.88,"input":"","cron":"0 */2 * * *"}`)
	embedFn := func(text string) []float32 {
		if text == "Improve documentation" {
			return []float32{1, 0}
		}
		return []float32{0.6, 0.8} // above gate, below fast-path → LLM
	}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	result, err := r.Route(context.Background(), "run docs-improve every 2 hours", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.CronExpr != "0 */2 * * *" {
		t.Errorf("expected cron %q, got %q", "0 */2 * * *", result.CronExpr)
	}
}

func TestHybridRouter_DefaultThresholds(t *testing.T) {
	// Zero Config → New() fills in all three default thresholds.
	mgr := makeMgr(t, `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`)
	emb := &fixedEmbedder{vec: []float32{1, 0}}
	r := New(mgr, emb, Config{Model: "test-model"})
	if r.cfg.ConfidentThreshold != DefaultConfidentThreshold {
		t.Errorf("ConfidentThreshold = %f, want %f", r.cfg.ConfidentThreshold, DefaultConfidentThreshold)
	}
	if r.cfg.AmbiguousThreshold != DefaultAmbiguousThreshold {
		t.Errorf("AmbiguousThreshold = %f, want %f", r.cfg.AmbiguousThreshold, DefaultAmbiguousThreshold)
	}
	if r.cfg.CandidateGateThreshold != DefaultCandidateGateThreshold {
		t.Errorf("CandidateGateThreshold = %f, want %f", r.cfg.CandidateGateThreshold, DefaultCandidateGateThreshold)
	}
}

func TestHybridRouter_EmbeddingAtExactThreshold(t *testing.T) {
	// A unit vector with cosine exactly 0.85 against [1,0] should use the embedding
	// fast path (>= threshold), NOT fall through to LLM.
	llmCalled := int64(0)
	mgr := executor.NewManager()
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "ollama",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			atomic.AddInt64(&llmCalled, 1)
			_, err := fmt.Fprint(w, `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`)
			return err
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	pipelines := []pipeline.PipelineRef{
		{Name: "alpha-pipeline", Description: "alpha unit"},
	}

	// cos([0.85, b], [1,0]) = 0.85 when b = sqrt(1 - 0.85²) (unit vector)
	b := float32(math.Sqrt(float64(1 - 0.85*0.85)))
	embedFn := func(text string) []float32 {
		if text == "alpha unit" {
			return []float32{1, 0}
		}
		return []float32{0.85, b}
	}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	// "run alpha-pipeline" starts with explicit verb → isImperativeInput = true
	result, err := r.Route(context.Background(), "run alpha-pipeline", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline == nil {
		t.Fatal("expected pipeline match at exact threshold, got nil")
	}
	if result.Method != "embedding" {
		t.Errorf("expected method 'embedding' at exact threshold, got %q", result.Method)
	}
	if atomic.LoadInt64(&llmCalled) != 0 {
		t.Errorf("LLM should not be called when embedding meets exact threshold, called %d times", llmCalled)
	}
}

func TestHybridRouter_MultiplePipelines_EmbeddingPicksBest(t *testing.T) {
	// Three orthogonal pipelines; query is identical to beta → cosine=1.0 → picks beta.
	pipelines := []pipeline.PipelineRef{
		{Name: "alpha", Description: "alpha desc"},
		{Name: "beta", Description: "beta desc"},
		{Name: "gamma", Description: "gamma desc"},
	}

	embedFn := func(text string) []float32 {
		switch text {
		case "alpha desc":
			return []float32{1, 0, 0}
		case "beta desc":
			return []float32{0, 1, 0}
		case "gamma desc":
			return []float32{0, 0, 1}
		default: // query
			return []float32{0, 1, 0} // identical to beta → cosine=1.0
		}
	}

	mgr := makeMgr(t, `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`)
	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	result, err := r.Route(context.Background(), "run beta", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline == nil {
		t.Fatal("expected pipeline match, got nil")
	}
	if result.Pipeline.Name != "beta" {
		t.Errorf("expected best match 'beta', got %q", result.Pipeline.Name)
	}
	if result.Method != "embedding" {
		t.Errorf("expected method 'embedding', got %q", result.Method)
	}
}

func TestHybridRouter_LLMHallucination(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes"},
	}
	// LLM returns a pipeline name not in the list
	mgr := makeMgr(t, `{"pipeline":"made-up-pipeline","confidence":0.90,"input":"","cron":""}`)
	embedFn := func(text string) []float32 {
		if text == "Push changes" {
			return []float32{1, 0}
		}
		return []float32{0.6, 0.8} // above gate → passes to LLM which hallucinates
	}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	result, err := r.Route(context.Background(), "do something weird", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline != nil {
		t.Errorf("expected nil pipeline for hallucinated name, got %q", result.Pipeline.Name)
	}
}

func TestHybridRouter_NonImperative_NeverDispatchesPipeline(t *testing.T) {
	// Non-imperative inputs (questions, observations, generic task requests) must
	// NEVER result in a pipeline dispatch. The LLM may be called (it applies its
	// own intent gate), but the result must always have Pipeline == nil.
	//
	// Note: the global isImperativeInput gate was removed. These prompts now reach
	// the LLM stage, which correctly returns NONE via its own Step 1 intent gate.
	mgr := makeMgr(t, `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`)

	pipelines := []pipeline.PipelineRef{
		{Name: "pipeline-a", Description: "alpha unit"},
	}

	embedFn := func(text string) []float32 {
		return []float32{1, 0} // cosine=1.0 — topic is relevant, but intent is not imperative
	}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
	}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	inputs := []string{
		"why is pipeline-a failing?",
		"please review this PR",
		"fix the thing",
		"looks like pipeline-a is stuck",
	}
	for _, prompt := range inputs {
		result, err := r.Route(context.Background(), prompt, pipelines)
		if err != nil {
			t.Fatalf("Route error: %v", err)
		}
		if result.Pipeline != nil {
			t.Errorf("non-imperative input %q must not route to pipeline, got %q", prompt, result.Pipeline.Name)
		}
	}
}

func TestHybridRouter_NaturalPhrasing_CanDispatch(t *testing.T) {
	// Natural-language dispatch ("can you run X?", "please run X") must now reach
	// the LLM and route correctly. These prompts previously failed the global gate
	// but the LLM correctly identifies them as explicit pipeline invocations.
	pipelines := []pipeline.PipelineRef{
		{Name: "pr-review", Description: "Review pull requests"},
	}

	cases := []struct {
		prompt      string
		llmResponse string
		wantName    string
	}{
		{
			"can you run pr-review on this PR?",
			`{"pipeline":"pr-review","confidence":0.91,"input":"","cron":""}`,
			"pr-review",
		},
		{
			"please run the pr-review pipeline",
			`{"pipeline":"pr-review","confidence":0.88,"input":"","cron":""}`,
			"pr-review",
		},
		{
			"could you launch pr-review for me?",
			`{"pipeline":"pr-review","confidence":0.85,"input":"","cron":""}`,
			"pr-review",
		},
	}

	for _, tc := range cases {
		t.Run(tc.prompt, func(t *testing.T) {
			mgr := makeMgr(t, tc.llmResponse)
			embedFn := func(text string) []float32 {
				if text == "Review pull requests" {
					return []float32{1, 0}
				}
				return []float32{0.7, 0.3} // above gate, below fast-path → LLM
			}
			cfg := Config{ConfidentThreshold: 0.85, AmbiguousThreshold: 0.65, Model: "test-model"}
			r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

			result, err := r.Route(context.Background(), tc.prompt, pipelines)
			if err != nil {
				t.Fatalf("Route error: %v", err)
			}
			if result.Pipeline == nil {
				t.Fatalf("natural phrasing %q should dispatch, got nil", tc.prompt)
			}
			if result.Pipeline.Name != tc.wantName {
				t.Errorf("pipeline=%q, want %q", result.Pipeline.Name, tc.wantName)
			}
		})
	}
}

func TestHybridRouter_EmbeddingFastPath_PopulatesInput(t *testing.T) {
	// The embedding fast-path (cosine >= ConfidentThreshold) must extract Input
	// from the prompt so pipelines receive their {{param.input}} variable even
	// when the LLM stage is skipped.
	pipelines := []pipeline.PipelineRef{
		{Name: "pr-review", Description: "Review pull requests"},
	}

	llmCalled := int64(0)
	mgr := executor.NewManager()
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "ollama",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			atomic.AddInt64(&llmCalled, 1)
			_, err := fmt.Fprint(w, `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`)
			return err
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Both description and query embed to {1,0} → cosine=1.0 → fast-path.
	embedFn := func(_ string) []float32 { return []float32{1, 0} }
	cfg := Config{ConfidentThreshold: 0.85, AmbiguousThreshold: 0.65, Model: "test-model"}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	result, err := r.Route(context.Background(), "run pr-review on https://github.com/org/repo/pull/42", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline == nil {
		t.Fatal("expected pipeline match, got nil")
	}
	if result.Method != "embedding" {
		t.Errorf("expected fast-path method 'embedding', got %q", result.Method)
	}
	if atomic.LoadInt64(&llmCalled) != 0 {
		t.Error("LLM must not be called on fast-path")
	}
	if result.Input != "https://github.com/org/repo/pull/42" {
		t.Errorf("Input = %q, want URL", result.Input)
	}
}

func TestHybridRouter_EmbeddingFastPath_PopulatesCron(t *testing.T) {
	// The embedding fast-path must extract CronExpr from the prompt so scheduled
	// pipelines work correctly without an LLM call.
	pipelines := []pipeline.PipelineRef{
		{Name: "docs-improve", Description: "Improve documentation"},
	}
	mgr := makeMgr(t, `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`)
	embedFn := func(_ string) []float32 { return []float32{1, 0} } // cosine=1.0 → fast-path

	cfg := Config{ConfidentThreshold: 0.85, AmbiguousThreshold: 0.65, Model: "test-model"}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	result, err := r.Route(context.Background(), "run docs-improve every 2 hours", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline == nil {
		t.Fatal("expected pipeline match, got nil")
	}
	if result.CronExpr != "0 */2 * * *" {
		t.Errorf("CronExpr = %q, want %q", result.CronExpr, "0 */2 * * *")
	}
}

func TestHybridRouter_FeedbackLog_WrittenOnMatch(t *testing.T) {
	// Every Route call must append a record to the feedback log.
	dir := t.TempDir()
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes"},
	}
	mgr := makeMgr(t, `{"pipeline":"git-push","confidence":0.92,"input":"","cron":""}`)
	embedFn := func(text string) []float32 {
		if text == "Push changes" {
			return []float32{1, 0}
		}
		return []float32{0.6, 0.8} // above gate, below fast-path → LLM
	}

	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
		CacheDir:           dir,
	}
	r := New(mgr, &funcEmbedder{fn: embedFn}, cfg)

	_, _ = r.Route(context.Background(), "run git-push on feature branch", pipelines)

	logPath := dir + "/routing-feedback.jsonl"
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("feedback log not created: %v", err)
	}
}
