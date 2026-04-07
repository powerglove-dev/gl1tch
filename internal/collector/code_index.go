package collector

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/brainrag"
	"github.com/8op-org/gl1tch/internal/esearch"
)

// CodeIndexCollector walks one or more directories on an interval, chunks
// every source file matching its extension list, embeds each chunk via the
// configured embedder, and upserts the result into the glitch-vectors
// index. Each root path gets its own RAG scope ("cwd:<abs path>") so brain
// queries can target a single tree.
//
// Re-indexing is cheap: brainrag.RAGStore.IndexNote checks the stored
// SHA256 of each chunk before re-running the embedder, so unchanged files
// cost only a small ES read on every tick.
//
// Collector is opt-in — BuildCollectorsFromConfig only instantiates one
// when cfg.CodeIndex.Enabled and at least one path is configured.
type CodeIndexCollector struct {
	// Paths is the list of absolute directories to walk. Each becomes
	// its own RAG scope.
	Paths []string
	// Extensions limits which files are indexed (e.g. ".go").
	// Empty falls back to brainrag.DefaultCodeExtensions.
	Extensions []string
	// ChunkSize is the max chars per chunk. Zero falls back to 1500.
	ChunkSize int
	// Interval is how often to re-walk every path. Defaults to 30m.
	Interval time.Duration
	// EmbedProvider selects the backend: "ollama" (default), "openai",
	// or "voyage".
	EmbedProvider string
	// EmbedModel is the provider-specific model name. Empty falls back
	// to provider default.
	EmbedModel string
	// EmbedBaseURL overrides the Ollama base URL. Ignored for cloud
	// providers.
	EmbedBaseURL string
	// EmbedAPIKey is a literal key, or "$ENV_VAR" to read from the
	// environment. Required for openai and voyage.
	EmbedAPIKey string
	// WorkspaceID is informational; the embeddings are scoped per path
	// rather than per workspace, but this is set so the run heartbeat
	// can be attributed correctly.
	WorkspaceID string
}

func (c *CodeIndexCollector) Name() string { return "code-index" }

func (c *CodeIndexCollector) Start(ctx context.Context, es *esearch.Client) error {
	if c.Interval == 0 {
		c.Interval = 30 * time.Minute
	}
	if len(c.Paths) == 0 {
		// Nothing to do — exit cleanly so the supervisor doesn't keep a
		// goroutine spinning on an empty config.
		slog.Info("code-index collector: no paths configured, exiting",
			"workspace", c.WorkspaceID)
		return nil
	}

	slog.Info("code-index collector: started",
		"workspace", c.WorkspaceID,
		"paths", len(c.Paths),
		"interval", c.Interval,
		"provider", c.EmbedProvider,
		"model", c.EmbedModel)
	for _, p := range c.Paths {
		slog.Debug("code-index collector: watching path",
			"workspace", c.WorkspaceID, "path", p)
	}

	embedder, err := c.buildEmbedder()
	if err != nil {
		return fmt.Errorf("code-index collector: %w", err)
	}

	// Index immediately on start so users see results without waiting
	// a full Interval; subsequent runs are time-based.
	slog.Debug("code-index collector: initial run", "workspace", c.WorkspaceID)
	initCtx, initDone := startTickSpan(ctx, "code-index", c.WorkspaceID)
	initN, initErr := c.runOnce(initCtx, es, embedder)
	initDone(initN, initErr)

	ticker := time.NewTicker(c.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			slog.Debug("code-index collector: tick", "workspace", c.WorkspaceID)
			tickCtx, tickDone := startTickSpan(ctx, "code-index", c.WorkspaceID)
			n, err := c.runOnce(tickCtx, es, embedder)
			tickDone(n, err)
		}
	}
}

// runOnce walks every configured path and indexes all matching files.
// Errors on individual paths are logged but don't abort the rest of the
// run; the heartbeat captures the last error for the brain UI.
//
// Returns (totalChunks, lastErr) so the caller can feed both into its
// per-tick span for the APM Transactions view — callers that don't
// care can ignore both values.
func (c *CodeIndexCollector) runOnce(ctx context.Context, es *esearch.Client, embedder brainrag.Embedder) (int, error) {
	start := time.Now()
	totalChunks := 0
	var lastErr error

	for _, path := range c.Paths {
		if ctx.Err() != nil {
			return totalChunks, lastErr
		}
		abs := expandPath(path)
		if abs == "" {
			continue
		}
		if _, statErr := os.Stat(abs); statErr != nil {
			slog.Warn("code-index collector: path missing",
				"workspace", c.WorkspaceID, "path", abs, "err", statErr)
			lastErr = statErr
			continue
		}

		slog.Debug("code-index collector: indexing path",
			"workspace", c.WorkspaceID, "path", abs)
		store := brainrag.NewRAGStoreForCWD(es, abs)
		res, err := brainrag.IndexTree(ctx, brainrag.IndexTreeOptions{
			Root:       abs,
			Extensions: c.Extensions,
			ChunkSize:  c.ChunkSize,
			Embedder:   embedder,
			Store:      store,
			Progress:   nil, // collector logs via slog instead
		})
		if err != nil {
			slog.Warn("code-index collector: index failed",
				"workspace", c.WorkspaceID, "path", abs, "err", err)
			lastErr = err
			continue
		}
		totalChunks += res.Chunks
		slog.Info("code-index collector: indexed",
			"workspace", c.WorkspaceID,
			"path", filepath.Base(abs),
			"files", res.Files,
			"chunks", res.Chunks)
	}

	slog.Debug("code-index collector: run done",
		"workspace", c.WorkspaceID,
		"total_chunks", totalChunks,
		"dur", time.Since(start))
	RecordRun("code-index", start, totalChunks, lastErr)
	return totalChunks, lastErr
}

// buildEmbedder mirrors pipeline.buildEmbedder but reads from the
// collector's typed fields rather than a generic args map. Kept here so
// the collector package doesn't have to import internal/pipeline (which
// would tangle the dependency graph).
func (c *CodeIndexCollector) buildEmbedder() (brainrag.Embedder, error) {
	provider := c.EmbedProvider
	if provider == "" {
		provider = "ollama"
	}

	apiKey := c.EmbedAPIKey
	if strings.HasPrefix(apiKey, "$") {
		apiKey = os.Getenv(apiKey[1:])
	}

	switch provider {
	case "ollama":
		return brainrag.NewOllamaEmbedder(c.EmbedBaseURL, c.EmbedModel), nil
	case "openai":
		if apiKey == "" {
			return nil, fmt.Errorf("embed_api_key is required for openai provider")
		}
		return brainrag.NewOpenAIEmbedder(apiKey, c.EmbedModel, 0), nil
	case "voyage":
		if apiKey == "" {
			return nil, fmt.Errorf("embed_api_key is required for voyage provider")
		}
		return brainrag.NewVoyageEmbedder(apiKey, c.EmbedModel), nil
	default:
		return nil, fmt.Errorf("unknown embed_provider %q", provider)
	}
}

// expandPath resolves a "~"-prefixed path to the user's home directory
// and returns the absolute form. Empty input returns empty.
func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}
