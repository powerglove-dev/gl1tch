// Package router provides intent routing for gl1tch prompts to matching pipelines.
// It implements a two-stage hybrid strategy:
//   - Stage 1 (embedding fast path): cosine similarity against cached pipeline
//     description vectors. If similarity >= ConfidentThreshold, dispatch immediately.
//   - Stage 2 (LLM fallback): a single structured JSON LLM call returning
//     {pipeline, confidence, input, cron}. Used when embeddings are ambiguous.
package router

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

var (
	routerTracer        = otel.Tracer("gl1tch/router")
	routerSimilarity    metric.Float64Histogram
	routerStrategyUsed  metric.Int64Counter
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

// DefaultConfidentThreshold is the cosine similarity above which a match is
// treated as definitive and the LLM stage is skipped.
const DefaultConfidentThreshold = 0.85

// DefaultAmbiguousThreshold is the minimum cosine similarity (or LLM confidence)
// below which a match is rejected.
const DefaultAmbiguousThreshold = 0.65

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
	// ConfidentThreshold: >= this score → dispatch without LLM (default 0.85).
	ConfidentThreshold float64
	// AmbiguousThreshold: >= this score (but < ConfidentThreshold) → dispatch with confirm (default 0.65).
	AmbiguousThreshold float64
	// CacheDir is the directory for the on-disk embedding cache.
	// If empty, the disk cache is disabled (in-memory only).
	CacheDir string
	// DisableEmbeddings skips the embedding fast-path entirely.
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

// HybridRouter implements the two-stage embedding + LLM routing strategy.
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
	return &HybridRouter{
		embedRouter: newEmbeddingRouter(embedder, cfg),
		classifier:  NewLLMClassifier(mgr, cfg),
		cfg:         cfg,
	}
}

// Route routes prompt to the best matching pipeline.
//
// Algorithm:
//  1. If DisableEmbeddings is false, try embedding fast path.
//     If cosine similarity >= ConfidentThreshold → return result immediately.
//  2. Fall through to LLM classifier.
//  3. On LLM error, return safe no-match result (no error surfaced to caller).
func (r *HybridRouter) Route(ctx context.Context, prompt string, pipelines []pipeline.PipelineRef) (*RouteResult, error) {
	if len(pipelines) == 0 {
		return &RouteResult{Method: "none"}, nil
	}

	ctx, span := routerTracer.Start(ctx, "router.classify")
	defer span.End()

	// Stage 1: embedding fast path.
	var embedNearMiss *pipeline.PipelineRef
	var embedNearMissScore float64
	if !r.cfg.DisableEmbeddings {
		embedResult, err := r.embedRouter.Route(ctx, prompt, pipelines)
		if err == nil && embedResult != nil && embedResult.Pipeline != nil {
			routerSimilarity.Record(ctx, embedResult.Confidence)
			if embedResult.Confidence >= r.cfg.ConfidentThreshold {
				embedResult.Method = "embedding"
				routerStrategyUsed.Add(ctx, 1, metric.WithAttributes(attribute.String("strategy", "embedding")))
				matched := ""
				if embedResult.Pipeline != nil {
					matched = embedResult.Pipeline.Name
				}
				span.SetAttributes(
					attribute.String("router.strategy", "embedding"),
					attribute.String("router.matched_pipeline", matched),
					attribute.Float64("router.confidence", embedResult.Confidence),
				)
				span.SetStatus(codes.Ok, "")
				return embedResult, nil
			}
			// Track near-miss candidates for caller awareness.
			if embedResult.Confidence >= NearMissThreshold {
				embedNearMiss = embedResult.Pipeline
				embedNearMissScore = embedResult.Confidence
			}
		}
		// If embedding returned a result above AmbiguousThreshold (but below
		// ConfidentThreshold), we still fall through to LLM for confirmation.
	}

	// Stage 2: LLM classifier.
	routerStrategyUsed.Add(ctx, 1, metric.WithAttributes(attribute.String("strategy", "llm")))
	result, err := r.classifier.Classify(ctx, prompt, pipelines)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		// LLM errors are non-fatal — return safe no-match with any near-miss we found.
		return &RouteResult{Method: "none", NearMiss: embedNearMiss, NearMissScore: embedNearMissScore}, nil
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
	// If LLM found no match but we have an embedding near-miss, attach it.
	if result != nil && result.Pipeline == nil && embedNearMiss != nil {
		result.NearMiss = embedNearMiss
		result.NearMissScore = embedNearMissScore
	}
	return result, nil
}
