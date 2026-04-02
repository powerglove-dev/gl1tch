package router

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

// ── cosine similarity ────────────────────────────────────────────────────────

func TestCosineSimilarity_Identical(t *testing.T) {
	v := []float32{1, 2, 3}
	got := cosineSimilarity(v, v)
	if got < 0.9999 || got > 1.0001 {
		t.Errorf("identical vectors: want ~1.0, got %f", got)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	got := cosineSimilarity(a, b)
	if got > 0.0001 || got < -0.0001 {
		t.Errorf("orthogonal vectors: want ~0.0, got %f", got)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{-1, 0, 0}
	got := cosineSimilarity(a, b)
	if got > -0.9999 {
		t.Errorf("opposite vectors: want ~-1.0, got %f", got)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	got := cosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("zero vector: want 0, got %f", got)
	}
}

func TestCosineSimilarity_DifferentLength(t *testing.T) {
	a := []float32{1, 2}
	b := []float32{1, 2, 3}
	got := cosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("different length vectors: want 0, got %f", got)
	}
}

// ── hash ─────────────────────────────────────────────────────────────────────

func TestHashDescription_Deterministic(t *testing.T) {
	h1 := hashDescription("git push pipeline")
	h2 := hashDescription("git push pipeline")
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q vs %q", h1, h2)
	}
	h3 := hashDescription("something else")
	if h1 == h3 {
		t.Error("different inputs produced same hash")
	}
}

// ── countingEmbedder ─────────────────────────────────────────────────────────

// countingEmbedder tracks how many times Embed is called and returns a fixed vector.
type countingEmbedder struct {
	vec   []float32
	count int
}

func (c *countingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	c.count++
	return c.vec, nil
}

// ── EmbeddingRouter tests ─────────────────────────────────────────────────────

func TestEmbeddingRouter_BestMatch(t *testing.T) {
	// Pipeline A: unit vector along axis 0
	// Pipeline B: unit vector along axis 1
	// Query: close to axis 0 → should match A
	pipelines := []pipeline.PipelineRef{
		{Name: "pipeline-a", Description: "axis zero pipeline"},
		{Name: "pipeline-b", Description: "axis one pipeline"},
	}

	embedCalls := 0
	embedFn := func(text string) []float32 {
		embedCalls++
		switch text {
		case "axis zero pipeline":
			return []float32{1, 0, 0}
		case "axis one pipeline":
			return []float32{0, 1, 0}
		default:
			// query — close to axis 0
			return []float32{0.99, 0.01, 0}
		}
	}

	emb := &funcEmbedder{fn: embedFn}
	cfg := Config{AmbiguousThreshold: 0.5}
	router := newEmbeddingRouter(emb, cfg)

	result, err := router.Route(context.Background(), "query", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline == nil {
		t.Fatal("expected a pipeline match, got nil")
	}
	if result.Pipeline.Name != "pipeline-a" {
		t.Errorf("expected pipeline-a, got %q", result.Pipeline.Name)
	}
}

func TestEmbeddingRouter_BelowThreshold(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "pipeline-a", Description: "some pipeline"},
	}

	// Make query orthogonal to description
	embedFn := func(text string) []float32 {
		if text == "some pipeline" {
			return []float32{1, 0}
		}
		return []float32{0, 1} // orthogonal → cosine = 0
	}

	emb2 := &funcEmbedder{fn: embedFn}
	cfg := Config{AmbiguousThreshold: 0.5}
	router := newEmbeddingRouter(emb2, cfg)

	result, err := router.Route(context.Background(), "unrelated query", pipelines)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline != nil {
		t.Errorf("expected nil pipeline (below threshold), got %q", result.Pipeline.Name)
	}
}

func TestEmbeddingRouter_CacheInvalidation(t *testing.T) {
	ce := &countingEmbedder{vec: []float32{1, 0}}
	cfg := Config{AmbiguousThreshold: 0.5}
	router := newEmbeddingRouter(ce, cfg)

	pipelines := []pipeline.PipelineRef{
		{Name: "p1", Description: "original description"},
	}

	// First call: embeds the description
	_, _ = router.Route(context.Background(), "q", pipelines)
	countAfterFirst := ce.count

	// Second call with same description: should NOT re-embed (cache hit)
	_, _ = router.Route(context.Background(), "q", pipelines)
	countAfterSecond := ce.count

	// The query is always re-embedded, but description should be cached.
	// countAfterFirst includes: 1 description embed + 1 query embed = 2
	// countAfterSecond should only add 1 more (just the query).
	descEmbedsFirst := countAfterFirst - 1 // subtract 1 for the query
	if descEmbedsFirst != 1 {
		t.Errorf("expected 1 description embed on first call, got %d", descEmbedsFirst)
	}
	descEmbedsSecond := countAfterSecond - countAfterFirst - 1 // subtract query re-embed
	if descEmbedsSecond != 0 {
		t.Errorf("expected 0 description re-embeds on second call (cache hit), got %d", descEmbedsSecond)
	}

	// Now change the description — should trigger re-embedding
	pipelinesUpdated := []pipeline.PipelineRef{
		{Name: "p1", Description: "completely new description"},
	}
	countBefore := ce.count
	_, _ = router.Route(context.Background(), "q", pipelinesUpdated)
	countAfterUpdate := ce.count
	newDescEmbeds := countAfterUpdate - countBefore - 1 // subtract query
	if newDescEmbeds != 1 {
		t.Errorf("expected 1 re-embed after description change, got %d", newDescEmbeds)
	}
}

func TestEmbeddingRouter_DiskCache(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "routing-index.json")

	vec := []float32{1, 0, 0}
	ce := &countingEmbedder{vec: vec}
	cfg := Config{
		AmbiguousThreshold: 0.5,
		CacheDir:           dir,
	}

	pipelines := []pipeline.PipelineRef{
		{Name: "disk-p1", Description: "disk cache test pipeline"},
	}

	// First router: embeds and writes cache
	r1 := newEmbeddingRouter(ce, cfg)
	_, _ = r1.Route(context.Background(), "q", pipelines)

	// Cache file should exist
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}

	// Verify it's valid JSON with our pipeline
	data, _ := os.ReadFile(cachePath)
	var cache map[string]pipelineEmbedding
	if err := json.Unmarshal(data, &cache); err != nil {
		t.Fatalf("cache file is not valid JSON: %v", err)
	}
	if _, ok := cache["disk-p1"]; !ok {
		t.Error("cache file missing pipeline entry")
	}

	// Second router loaded fresh — should read from disk cache
	countBefore := ce.count
	r2 := newEmbeddingRouter(ce, cfg)
	_, _ = r2.Route(context.Background(), "q", pipelines)
	// Should only embed the query, not the description (loaded from disk)
	added := ce.count - countBefore
	if added > 1 {
		t.Errorf("expected at most 1 embed call (query only) when loading from disk cache, got %d", added)
	}
}

// ── TriggerPhrases ────────────────────────────────────────────────────────────

func TestEmbeddingRouter_TriggerPhrases_EmbeddedAsCentroid(t *testing.T) {
	// Pipeline has two trigger phrases whose embeddings are axis-0 and axis-1 unit
	// vectors. The centroid is {0.5, 0.5}, so a query of {0.707, 0.707} has cosine
	// ≈ 1.0 against the centroid.
	axis0 := []float32{1, 0}
	axis1 := []float32{0, 1}

	p := pipeline.PipelineRef{
		Name:           "tp-pipeline",
		Description:    "should not be embedded",
		TriggerPhrases: []string{"phrase one", "phrase two"},
	}

	embedFn := func(text string) []float32 {
		switch text {
		case "phrase one":
			return axis0
		case "phrase two":
			return axis1
		default: // query — close to centroid direction
			return []float32{0.707, 0.707}
		}
	}

	emb := &funcEmbedder{fn: embedFn}
	cfg := Config{AmbiguousThreshold: 0.5}
	router := newEmbeddingRouter(emb, cfg)

	result, err := router.Route(context.Background(), "query", []pipeline.PipelineRef{p})
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if result.Pipeline == nil {
		t.Fatal("expected pipeline match via trigger phrase centroid, got nil")
	}
	if result.Pipeline.Name != "tp-pipeline" {
		t.Errorf("expected tp-pipeline, got %q", result.Pipeline.Name)
	}
}

func TestEmbeddingRouter_TriggerPhrases_CacheInvalidates(t *testing.T) {
	// Changing trigger phrases must invalidate the cached embedding.
	ce := &countingEmbedder{vec: []float32{1, 0}}
	cfg := Config{AmbiguousThreshold: 0.5}
	router := newEmbeddingRouter(ce, cfg)

	p1 := pipeline.PipelineRef{
		Name:           "tp-p",
		Description:    "desc",
		TriggerPhrases: []string{"invoke alpha"},
	}
	_, _ = router.Route(context.Background(), "q", []pipeline.PipelineRef{p1})
	countAfterFirst := ce.count

	// Same trigger phrases — cache hit, no re-embed.
	_, _ = router.Route(context.Background(), "q", []pipeline.PipelineRef{p1})
	if ce.count != countAfterFirst+1 { // only query re-embeds
		t.Errorf("expected only query embed on cache hit, got %d extra calls", ce.count-countAfterFirst)
	}

	// Changed trigger phrase — must re-embed.
	p2 := pipeline.PipelineRef{
		Name:           "tp-p",
		Description:    "desc",
		TriggerPhrases: []string{"invoke beta"},
	}
	countBefore := ce.count
	_, _ = router.Route(context.Background(), "q", []pipeline.PipelineRef{p2})
	added := ce.count - countBefore
	if added < 2 { // at least: 1 trigger phrase + 1 query
		t.Errorf("expected re-embed after trigger phrase change, got only %d new embed calls", added)
	}
}

func TestEmbeddingRouter_FindCandidates_BelowGate(t *testing.T) {
	// cosine = 0 < gate (0.40) → FindCandidates returns empty slice.
	embedFn := func(text string) []float32 {
		if text == "alpha pipeline" {
			return []float32{1, 0}
		}
		return []float32{0, 1} // orthogonal → cosine = 0
	}
	emb := &funcEmbedder{fn: embedFn}
	cfg := Config{CandidateGateThreshold: 0.40}
	router := newEmbeddingRouter(emb, cfg)

	pipelines := []pipeline.PipelineRef{
		{Name: "alpha", Description: "alpha pipeline"},
	}
	candidates, err := router.FindCandidates(context.Background(), "query", pipelines, 0.40)
	if err != nil {
		t.Fatalf("FindCandidates error: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates below gate, got %d", len(candidates))
	}
}

func TestEmbeddingRouter_FindCandidates_MultipleAboveGate(t *testing.T) {
	// Three pipelines; query vector is {1,0,0}.
	// "high match" is identical → cosine=1.0.
	// "partial match" is cos60 away → cosine=0.5 (above gate 0.40).
	// "no match" is orthogonal → cosine=0 (below gate).
	// Returned candidates must be sorted descending by score: high before partial.
	embedFn := func(text string) []float32 {
		switch text {
		case "high match":
			return []float32{1, 0, 0}
		case "partial match":
			return []float32{0.5, 0.866, 0} // cosine=0.5 with query {1,0,0}
		case "no match":
			return []float32{0, 0, 1} // orthogonal to query
		default: // query
			return []float32{1, 0, 0}
		}
	}
	emb := &funcEmbedder{fn: embedFn}
	cfg := Config{}
	router := newEmbeddingRouter(emb, cfg)

	pipelines := []pipeline.PipelineRef{
		{Name: "high", Description: "high match"},
		{Name: "partial", Description: "partial match"},
		{Name: "none", Description: "no match"},
	}
	candidates, err := router.FindCandidates(context.Background(), "query", pipelines, 0.40)
	if err != nil {
		t.Fatalf("FindCandidates error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates above gate, got %d", len(candidates))
	}
	if candidates[0].Score < candidates[1].Score {
		t.Error("candidates must be sorted descending by score")
	}
	if candidates[0].Ref.Name != "high" {
		t.Errorf("expected top candidate 'high', got %q", candidates[0].Ref.Name)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// fixedEmbedder always returns the same vector.
type fixedEmbedder struct{ vec []float32 }

func (f *fixedEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return f.vec, nil
}

// funcEmbedder calls an arbitrary function per text.
type funcEmbedder struct {
	fn func(text string) []float32
}

func (f *funcEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	return f.fn(text), nil
}
