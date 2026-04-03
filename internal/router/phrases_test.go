//go:build !integration

package router

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// ── parsePhrasesResponse ──────────────────────────────────────────────────────

func TestParsePhrasesResponse_Valid(t *testing.T) {
	raw := `["run git-pulse", "execute git-pulse", "launch git pulse pipeline"]`
	phrases, err := parsePhrasesResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(phrases) != 3 {
		t.Fatalf("expected 3 phrases, got %d: %v", len(phrases), phrases)
	}
	if phrases[0] != "run git-pulse" {
		t.Errorf("phrases[0] = %q, want %q", phrases[0], "run git-pulse")
	}
}

func TestParsePhrasesResponse_WithLeadingText(t *testing.T) {
	// LLM sometimes emits prose before the array.
	raw := `Here are the phrases: ["run git-pulse", "trigger git-pulse"]`
	phrases, err := parsePhrasesResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(phrases) != 2 {
		t.Errorf("expected 2 phrases, got %d", len(phrases))
	}
}

func TestParsePhrasesResponse_EmptyStringsFiltered(t *testing.T) {
	raw := `["run git-pulse", "", "  ", "launch git-pulse"]`
	phrases, err := parsePhrasesResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(phrases) != 2 {
		t.Errorf("expected 2 non-empty phrases, got %d: %v", len(phrases), phrases)
	}
}

func TestParsePhrasesResponse_CapAt8(t *testing.T) {
	// More than 8 entries should be capped.
	raw := `["a","b","c","d","e","f","g","h","i","j"]`
	phrases, err := parsePhrasesResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(phrases) != 8 {
		t.Errorf("expected cap of 8, got %d", len(phrases))
	}
}

func TestParsePhrasesResponse_NoArray(t *testing.T) {
	_, err := parsePhrasesResponse("Here are some phrases but no array")
	if err == nil {
		t.Error("expected error for no JSON array, got nil")
	}
}

func TestParsePhrasesResponse_AllEmpty(t *testing.T) {
	_, err := parsePhrasesResponse(`["", "  "]`)
	if err == nil {
		t.Error("expected error when all phrases are empty, got nil")
	}
}

// ── LLMPhraseGenerator ────────────────────────────────────────────────────────

func TestLLMPhraseGenerator_ReturnsGeneratedPhrases(t *testing.T) {
	mgr := executor.NewManager()
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "ollama",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := fmt.Fprint(w, `["run git-pulse", "execute git-pulse", "launch git pulse"]`)
			return err
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	gen := NewLLMPhraseGenerator(mgr, "test-model")
	phrases, err := gen.GeneratePhrases(context.Background(), "git-pulse", "Analyze git activity")
	if err != nil {
		t.Fatalf("GeneratePhrases error: %v", err)
	}
	if len(phrases) != 3 {
		t.Errorf("expected 3 phrases, got %d: %v", len(phrases), phrases)
	}
}

func TestLLMPhraseGenerator_LLMError(t *testing.T) {
	mgr := executor.NewManager()
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "ollama",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			return fmt.Errorf("llm unavailable")
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	gen := NewLLMPhraseGenerator(mgr, "test-model")
	_, err := gen.GeneratePhrases(context.Background(), "git-pulse", "Analyze git activity")
	if err == nil {
		t.Error("expected error when LLM fails, got nil")
	}
}

// ── PhraseGenerator integration with EmbeddingRouter ─────────────────────────

// stubPhraseGenerator implements PhraseGenerator for tests.
type stubPhraseGenerator struct {
	phrases []string
	calls   int
	err     error
}

func (g *stubPhraseGenerator) GeneratePhrases(_ context.Context, _, _ string) ([]string, error) {
	g.calls++
	return g.phrases, g.err
}

func TestEmbeddingRouter_AutoGeneratesPhrases(t *testing.T) {
	// Pipeline has no trigger phrases. Generator returns two phrases.
	// The router should use those phrases' centroid for embedding, not the description.
	embeddedTexts := make([]string, 0)
	embedFn := func(text string) []float32 {
		embeddedTexts = append(embeddedTexts, text)
		switch text {
		case "run the thing":
			return []float32{1, 0}
		case "execute thing":
			return []float32{0.9, 0.1}
		default:
			return []float32{0.5, 0.5}
		}
	}

	gen := &stubPhraseGenerator{
		phrases: []string{"run the thing", "execute thing"},
	}

	cfg := Config{
		AmbiguousThreshold: 0.5,
		PhraseGenerator:    gen,
	}
	router := newEmbeddingRouter(&funcEmbedder{fn: embedFn}, cfg)

	p := pipeline.PipelineRef{
		Name:        "thing-pipeline",
		Description: "does the thing",
		// No TriggerPhrases — generator should be called
	}

	result, err := router.Route(context.Background(), "run the thing", []pipeline.PipelineRef{p})
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline == nil {
		t.Fatal("expected pipeline match via generated phrases, got nil")
	}

	// Generator should have been called once.
	if gen.calls != 1 {
		t.Errorf("generator called %d times, want 1", gen.calls)
	}

	// Generated phrases (not description) should have been embedded.
	foundPhrase := false
	for _, text := range embeddedTexts {
		if text == "run the thing" || text == "execute thing" {
			foundPhrase = true
			break
		}
	}
	if !foundPhrase {
		t.Errorf("generated phrases not embedded; embeddedTexts = %v", embeddedTexts)
	}
}

func TestEmbeddingRouter_GeneratedPhrasesAreCached(t *testing.T) {
	// Generator should only be called once per pipeline version, even across multiple Route calls.
	gen := &stubPhraseGenerator{
		phrases: []string{"run alpha"},
	}

	cfg := Config{
		AmbiguousThreshold: 0.5,
		PhraseGenerator:    gen,
	}
	emb := &countingEmbedder{vec: []float32{1, 0}}
	router := newEmbeddingRouter(emb, cfg)

	p := pipeline.PipelineRef{Name: "alpha", Description: "alpha pipeline"}

	_, _ = router.Route(context.Background(), "q", []pipeline.PipelineRef{p})
	_, _ = router.Route(context.Background(), "q", []pipeline.PipelineRef{p})

	if gen.calls != 1 {
		t.Errorf("generator should be called once (cache hit on second call), got %d calls", gen.calls)
	}
}

func TestEmbeddingRouter_GeneratorError_FallsBackToDescription(t *testing.T) {
	// When the generator fails, the router must not error — fall back to description.
	gen := &stubPhraseGenerator{err: fmt.Errorf("generator unavailable")}

	embeddedTexts := make([]string, 0)
	embedFn := func(text string) []float32 {
		embeddedTexts = append(embeddedTexts, text)
		return []float32{1, 0}
	}

	cfg := Config{
		AmbiguousThreshold: 0.5,
		PhraseGenerator:    gen,
	}
	router := newEmbeddingRouter(&funcEmbedder{fn: embedFn}, cfg)

	p := pipeline.PipelineRef{Name: "alpha", Description: "alpha pipeline"}
	_, err := router.Route(context.Background(), "q", []pipeline.PipelineRef{p})
	if err != nil {
		t.Fatalf("Route should not error on generator failure, got: %v", err)
	}

	// Description should have been embedded as fallback.
	found := false
	for _, text := range embeddedTexts {
		if text == "alpha pipeline" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("description not embedded as fallback; embeddedTexts = %v", embeddedTexts)
	}
}

func TestEmbeddingRouter_ExplicitPhrasesWinOverGenerator(t *testing.T) {
	// When the pipeline has explicit TriggerPhrases, the generator must NOT be called.
	gen := &stubPhraseGenerator{
		phrases: []string{"generated phrase"},
	}

	cfg := Config{
		AmbiguousThreshold: 0.5,
		PhraseGenerator:    gen,
	}
	emb := &countingEmbedder{vec: []float32{1, 0}}
	router := newEmbeddingRouter(emb, cfg)

	p := pipeline.PipelineRef{
		Name:           "alpha",
		Description:    "alpha pipeline",
		TriggerPhrases: []string{"run alpha explicitly"},
	}

	_, _ = router.Route(context.Background(), "q", []pipeline.PipelineRef{p})

	if gen.calls != 0 {
		t.Errorf("generator must not be called when TriggerPhrases are set, got %d calls", gen.calls)
	}
}
