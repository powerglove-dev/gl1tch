package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

// pipelineEmbedding is the on-disk (and in-memory) cache entry for one pipeline.
type pipelineEmbedding struct {
	// DescHash is the SHA256 of (name + description + trigger_phrases); used to detect staleness.
	DescHash string    `json:"desc_hash"`
	Vector   []float32 `json:"vector"`
	// GeneratedPhrases caches auto-generated trigger phrases so the LLM is called at most
	// once per pipeline version. Populated by EmbeddingRouter when PhraseGenerator is set.
	GeneratedPhrases []string `json:"generated_phrases,omitempty"`
}

// ScoredCandidate pairs a pipeline reference with its cosine similarity score.
type ScoredCandidate struct {
	Ref   pipeline.PipelineRef
	Score float64
}

// EmbeddingRouter computes cosine similarity between the prompt embedding and
// cached pipeline representative vectors. It maintains an in-memory cache
// (invalidated by description+trigger hash) and optionally persists to disk.
type EmbeddingRouter struct {
	embedder  Embedder
	cfg       Config
	generator PhraseGenerator // optional; nil disables auto-generation

	mu    sync.Mutex
	cache map[string]pipelineEmbedding // keyed by pipeline name
}

// newEmbeddingRouter creates an EmbeddingRouter. It loads any existing disk
// cache from cfg.CacheDir on construction.
func newEmbeddingRouter(embedder Embedder, cfg Config) *EmbeddingRouter {
	r := &EmbeddingRouter{
		embedder:  embedder,
		cfg:       cfg,
		generator: cfg.PhraseGenerator,
		cache:     make(map[string]pipelineEmbedding),
	}
	r.loadDiskCache()
	return r
}

// FindCandidates returns all pipelines whose representative vector has cosine
// similarity >= threshold with the prompt embedding, sorted by score descending.
// Used as a negative gate: when the result is empty the caller can skip the LLM
// stage entirely, since no pipeline is topically relevant to the prompt.
func (r *EmbeddingRouter) FindCandidates(ctx context.Context, prompt string, pipelines []pipeline.PipelineRef, threshold float64) ([]ScoredCandidate, error) {
	if len(pipelines) == 0 {
		return nil, nil
	}
	if err := r.ensureEmbedded(ctx, pipelines); err != nil {
		return nil, err
	}
	promptVec, err := r.embedder.Embed(ctx, prompt)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	var candidates []ScoredCandidate
	for i := range pipelines {
		entry, ok := r.cache[pipelines[i].Name]
		if !ok {
			continue
		}
		score := cosineSimilarity(promptVec, entry.Vector)
		if score >= threshold {
			candidates = append(candidates, ScoredCandidate{Ref: pipelines[i], Score: score})
		}
	}
	r.mu.Unlock()

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	return candidates, nil
}

// Route computes similarity between the prompt and each pipeline's representative
// vector, returning the best match above AmbiguousThreshold (or nil pipeline if
// none qualify). It delegates to FindCandidates internally.
func (r *EmbeddingRouter) Route(ctx context.Context, prompt string, pipelines []pipeline.PipelineRef) (*RouteResult, error) {
	candidates, err := r.FindCandidates(ctx, prompt, pipelines, r.cfg.AmbiguousThreshold)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return &RouteResult{Method: "none", Confidence: 0}, nil
	}
	best := candidates[0]
	return &RouteResult{
		Pipeline:   &best.Ref,
		Confidence: best.Score,
		Method:     "embedding",
	}, nil
}

// ensureEmbedded checks the in-memory cache for each pipeline and (re-)embeds
// those whose hash has changed.
//
// Embedding source priority:
//  1. Pipeline's own trigger_phrases (explicit YAML field) — imperative phrase centroid.
//  2. Auto-generated phrases from PhraseGenerator (cached in GeneratedPhrases) — centroid.
//  3. Description text fallback.
func (r *EmbeddingRouter) ensureEmbedded(ctx context.Context, pipelines []pipeline.PipelineRef) error {
	r.mu.Lock()
	var toEmbed []pipeline.PipelineRef
	for _, p := range pipelines {
		h := pipelineHash(p)
		if entry, ok := r.cache[p.Name]; ok && entry.DescHash == h {
			continue // cache hit
		}
		toEmbed = append(toEmbed, p)
	}
	r.mu.Unlock()

	if len(toEmbed) == 0 {
		return nil
	}

	for _, p := range toEmbed {
		var vec []float32
		var err error
		h := pipelineHash(p)

		phrases := p.TriggerPhrases

		// Auto-generate trigger phrases when none are defined and a generator is set.
		// Generated phrases are stored in the cache entry so the LLM is called at most once.
		if len(phrases) == 0 && r.generator != nil {
			generated, genErr := r.generator.GeneratePhrases(ctx, p.Name, p.Description)
			if genErr == nil && len(generated) > 0 {
				phrases = generated
				// Write generated phrases back into the cache entry below.
			}
			// Generation failure is non-fatal: fall through to description embedding.
		}

		if len(phrases) > 0 {
			// Embed each phrase and use the centroid as the representative vector.
			// Trigger phrases are imperative invocation patterns ("run git pulse", etc.)
			// so the embedding space is driven by command intent rather than behavior prose.
			vecs := make([][]float32, 0, len(phrases))
			for _, phrase := range phrases {
				v, embedErr := r.embedder.Embed(ctx, phrase)
				if embedErr != nil {
					return embedErr
				}
				vecs = append(vecs, v)
			}
			vec = centroid(vecs)
		} else {
			vec, err = r.embedder.Embed(ctx, p.Description)
			if err != nil {
				return err
			}
		}

		r.mu.Lock()
		entry := pipelineEmbedding{DescHash: h, Vector: vec}
		// Cache generated phrases (not the YAML ones — those come from the PipelineRef).
		if len(p.TriggerPhrases) == 0 && len(phrases) > 0 {
			entry.GeneratedPhrases = phrases
		}
		r.cache[p.Name] = entry
		r.mu.Unlock()
	}

	r.saveDiskCache()
	return nil
}

// ── disk cache ────────────────────────────────────────────────────────────────

func (r *EmbeddingRouter) cacheFilePath() string {
	if r.cfg.CacheDir == "" {
		return ""
	}
	return filepath.Join(r.cfg.CacheDir, "routing-index.json")
}

func (r *EmbeddingRouter) loadDiskCache() {
	path := r.cacheFilePath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // no cache yet — normal
	}
	var cache map[string]pipelineEmbedding
	if err := json.Unmarshal(data, &cache); err != nil {
		return // corrupted cache — ignore
	}
	r.mu.Lock()
	for k, v := range cache {
		r.cache[k] = v
	}
	r.mu.Unlock()
}

func (r *EmbeddingRouter) saveDiskCache() {
	path := r.cacheFilePath()
	if path == "" {
		return
	}
	r.mu.Lock()
	data, err := json.MarshalIndent(r.cache, "", "  ")
	r.mu.Unlock()
	if err != nil {
		return
	}
	// Write atomically via temp file.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// cosineSimilarity returns the cosine similarity between two float32 vectors.
// Returns 0 if lengths differ or either is the zero vector.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// centroid returns the element-wise average of the given vectors.
// Returns nil if vecs is empty. Assumes all vectors have the same length.
func centroid(vecs [][]float32) []float32 {
	if len(vecs) == 0 {
		return nil
	}
	n := len(vecs[0])
	result := make([]float32, n)
	for _, v := range vecs {
		for i, x := range v {
			result[i] += x
		}
	}
	count := float32(len(vecs))
	for i := range result {
		result[i] /= count
	}
	return result
}

// pipelineHash returns a SHA256 hex digest of the pipeline's name, description,
// and trigger phrases. Used to detect cache staleness when any of these change.
func pipelineHash(p pipeline.PipelineRef) string {
	return hashDescription(p.Name + p.Description + strings.Join(p.TriggerPhrases, "|"))
}

// hashDescription returns the SHA256 hex digest of s.
func hashDescription(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
