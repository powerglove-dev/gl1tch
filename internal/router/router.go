// Package router provides intent routing for gl1tch prompts to matching pipelines.
// It implements a two-stage hybrid strategy:
//   - Stage 1 (embedding negative filter): cosine similarity against cached pipeline
//     vectors. If NO pipeline clears the candidate gate threshold, return none immediately
//     (skip LLM). If at least one clears it, check for fast-path or fall through to LLM.
//   - Stage 2 (LLM classifier): a single structured JSON call that gates on intent
//     type (command vs. question/observation) before selecting a pipeline.
//
// The embedding stage is intentionally a negative filter — it only short-circuits on
// the "nothing relevant" case, never on a positive match alone. This prevents
// topic-overlap misfires where questions about a topic route to a pipeline covering
// that topic. The fast path (skip LLM) fires only when embedding confidence is very
// high AND the input is syntactically imperative.
package router

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

var (
	routerTracer       = otel.Tracer("gl1tch/router")
	routerSimilarity   metric.Float64Histogram
	routerStrategyUsed metric.Int64Counter
)

func init() {
	meter := otel.Meter("gl1tch/router")
	routerSimilarity, _ = meter.Float64Histogram("gl1tch.router.similarity_score",
		metric.WithDescription("Cosine similarity score from embedding routing"),
	)
	routerStrategyUsed, _ = meter.Int64Counter("gl1tch.router.strategy_used",
		metric.WithDescription("Number of times each routing strategy was used"),
	)
}

// DefaultConfidentThreshold is the minimum cosine similarity at which a match may
// use the embedding fast path — but only when isImperativeInput is also true.
const DefaultConfidentThreshold = 0.85

// DefaultAmbiguousThreshold is the minimum LLM confidence for a match to be accepted.
const DefaultAmbiguousThreshold = 0.65

// DefaultCandidateGateThreshold is the minimum cosine similarity to admit a pipeline
// as a candidate for LLM classification. Pipelines below this threshold are considered
// topically unrelated and are excluded. When no pipeline clears this gate, the LLM
// call is skipped entirely — this is the primary misfire prevention mechanism.
const DefaultCandidateGateThreshold = 0.40

// DefaultEmbeddingModel is the local Ollama model used for fast-path routing.
const DefaultEmbeddingModel = "nomic-embed-text"

// RouteResult is the outcome of a routing decision.
type RouteResult struct {
	// Pipeline is the matched PipelineRef, or nil when no pipeline was found.
	Pipeline *pipeline.PipelineRef
	// Confidence is the similarity/confidence score in [0, 1].
	Confidence float64
	// Input is the extracted focus/topic for {{param.input}}, or "".
	Input string
	// CronExpr is a validated 5-field cron expression, or "".
	CronExpr string
	// Method records which stage produced the result: "embedding", "llm", or "none".
	Method string
	// NearMiss is the closest pipeline ref when the score was between NearMissThreshold
	// and the AmbiguousThreshold — i.e., a probable match that didn't clear the bar.
	NearMiss *pipeline.PipelineRef
	// NearMissScore is the similarity score for NearMiss.
	NearMissScore float64
}

// NearMissThreshold is the minimum score to report a near-miss candidate.
// Scores below this are treated as noise.
const NearMissThreshold = 0.60

// Config controls routing behavior.
type Config struct {
	// Model is the Ollama model used for LLM classification.
	Model string
	// EmbeddingModel is the Ollama embedding model. Used by the OllamaEmbedder helper.
	EmbeddingModel string
	// OllamaBaseURL is the base URL for Ollama (defaults to http://localhost:11434).
	OllamaBaseURL string
	// ConfidentThreshold: combined with isImperativeInput, score >= this → fast path (default 0.85).
	ConfidentThreshold float64
	// AmbiguousThreshold: minimum LLM confidence for a match to be accepted (default 0.65).
	AmbiguousThreshold float64
	// CandidateGateThreshold: minimum cosine similarity to admit a pipeline as an LLM
	// candidate. When no pipeline clears this, the LLM call is skipped (default 0.40).
	CandidateGateThreshold float64
	// CacheDir is the directory for the on-disk embedding cache.
	// If empty, the disk cache is disabled (in-memory only).
	CacheDir string
	// DisableEmbeddings skips the embedding stage entirely and always uses the LLM.
	DisableEmbeddings bool
}

// Embedder abstracts embedding computation so tests can inject stubs.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Router routes prompts to pipelines.
type Router interface {
	Route(ctx context.Context, prompt string, pipelines []pipeline.PipelineRef) (*RouteResult, error)
}

// HybridRouter implements the two-stage negative-filter + LLM routing strategy.
type HybridRouter struct {
	embedRouter *EmbeddingRouter
	classifier  *LLMClassifier
	cfg         Config
}

// New creates a HybridRouter with the given executor manager, embedder, and config.
// Thresholds are set to defaults if zero.
func New(mgr *executor.Manager, embedder Embedder, cfg Config) *HybridRouter {
	if cfg.ConfidentThreshold == 0 {
		cfg.ConfidentThreshold = DefaultConfidentThreshold
	}
	if cfg.AmbiguousThreshold == 0 {
		cfg.AmbiguousThreshold = DefaultAmbiguousThreshold
	}
	if cfg.CandidateGateThreshold == 0 {
		cfg.CandidateGateThreshold = DefaultCandidateGateThreshold
	}
	return &HybridRouter{
		embedRouter: newEmbeddingRouter(embedder, cfg),
		classifier:  NewLLMClassifier(mgr, cfg),
		cfg:         cfg,
	}
}

// Route routes prompt to the best matching pipeline.
//
// Algorithm:
//  1. Empty pipeline list → none.
//  2. Embedding negative filter (unless DisableEmbeddings):
//     a. Find all candidates with cosine >= CandidateGateThreshold.
//     b. Empty candidates → return none immediately (no LLM call).
//     c. Top candidate >= ConfidentThreshold AND isImperativeInput(prompt) → fast path.
//     d. Otherwise fall through to LLM with the candidate subset.
//  3. LLM classifier on candidate subset (intent gate + pipeline selection).
//     LLM errors are non-fatal — return safe no-match result.
func (r *HybridRouter) Route(ctx context.Context, prompt string, pipelines []pipeline.PipelineRef) (*RouteResult, error) {
	if len(pipelines) == 0 {
		return &RouteResult{Method: "none"}, nil
	}

	ctx, span := routerTracer.Start(ctx, "router.classify")
	defer span.End()

	// candidatePipelines is what gets passed to the LLM. Starts as the full list
	// (for DisableEmbeddings) but is narrowed to candidates when embeddings run.
	candidatePipelines := pipelines
	var nearMiss *pipeline.PipelineRef
	var nearMissScore float64

	if !r.cfg.DisableEmbeddings {
		candidates, err := r.embedRouter.FindCandidates(ctx, prompt, pipelines, r.cfg.CandidateGateThreshold)
		if err == nil {
			if len(candidates) == 0 {
				// Negative filter: no pipeline is topically relevant — skip LLM entirely.
				routerSimilarity.Record(ctx, 0)
				routerStrategyUsed.Add(ctx, 1, metric.WithAttributes(attribute.String("strategy", "embedding-negative")))
				span.SetAttributes(
					attribute.String("router.strategy", "embedding-negative"),
					attribute.Float64("router.confidence", 0),
				)
				span.SetStatus(codes.Ok, "")
				return &RouteResult{Method: "none"}, nil
			}

			topScore := candidates[0].Score
			routerSimilarity.Record(ctx, topScore)

			// Fast path: high confidence AND clearly imperative — skip LLM for latency.
			if topScore >= r.cfg.ConfidentThreshold && isImperativeInput(prompt) {
				best := &candidates[0].Ref
				routerStrategyUsed.Add(ctx, 1, metric.WithAttributes(attribute.String("strategy", "embedding")))
				span.SetAttributes(
					attribute.String("router.strategy", "embedding"),
					attribute.String("router.matched_pipeline", best.Name),
					attribute.Float64("router.confidence", topScore),
				)
				span.SetStatus(codes.Ok, "")
				return &RouteResult{
					Pipeline:   best,
					Confidence: topScore,
					Method:     "embedding",
				}, nil
			}

			// Narrow the LLM candidate list to only topically relevant pipelines.
			// This reduces hallucination surface and focuses the LLM on real choices.
			candidatePipelines = make([]pipeline.PipelineRef, len(candidates))
			for i, c := range candidates {
				candidatePipelines[i] = c.Ref
			}

			// Track near-miss: candidate that almost made it but fell below AmbiguousThreshold.
			if topScore >= NearMissThreshold && topScore < r.cfg.AmbiguousThreshold {
				nearMiss = &candidates[0].Ref
				nearMissScore = topScore
			}
		}
	}

	// Stage 2: LLM classifier.
	routerStrategyUsed.Add(ctx, 1, metric.WithAttributes(attribute.String("strategy", "llm")))
	result, err := r.classifier.Classify(ctx, prompt, candidatePipelines)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		// LLM errors are non-fatal — return safe no-match with any near-miss we found.
		return &RouteResult{Method: "none", NearMiss: nearMiss, NearMissScore: nearMissScore}, nil
	}

	matched := ""
	if result != nil && result.Pipeline != nil {
		matched = result.Pipeline.Name
	}
	confidence := 0.0
	if result != nil {
		confidence = result.Confidence
	}
	span.SetAttributes(
		attribute.String("router.strategy", "llm"),
		attribute.String("router.matched_pipeline", matched),
		attribute.Float64("router.confidence", confidence),
	)
	span.SetStatus(codes.Ok, "")

	// Attach near-miss to LLM no-match results for caller awareness.
	if result != nil && result.Pipeline == nil && nearMiss != nil {
		result.NearMiss = nearMiss
		result.NearMissScore = nearMissScore
	}
	return result, nil
}

// isImperativeInput returns false when the prompt is clearly a question or observation,
// blocking the embedding fast path in those cases. It is deliberately conservative:
// when in doubt it returns true, keeping the fast path available. The LLM stage handles
// all ambiguous intent classification regardless.
func isImperativeInput(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	// Explicit question mark at end of prompt.
	if strings.HasSuffix(s, "?") {
		return false
	}
	// Interrogative and observational starters that signal non-command intent.
	questionStarters := []string{
		"what ", "what's ", "why ", "how ", "is ", "are ", "was ", "were ",
		"can ", "could ", "should ", "would ", "will ", "do ", "does ", "did ",
		"looks like", "it looks", "seems like", "i think", "i noticed",
		"any idea", "any thoughts",
	}
	for _, prefix := range questionStarters {
		if strings.HasPrefix(s, prefix) {
			return false
		}
	}
	return true
}
