package capability

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/brainrag"
	"github.com/8op-org/gl1tch/internal/esearch"
)

// CodeIndexCapability walks one or more directories on an interval, chunks
// every source file matching its extension list, embeds each chunk via the
// configured embedder, and upserts the result into the glitch-vectors
// index. Each root path gets its own RAG scope ("cwd:<abs path>") so brain
// queries can target a single tree.
//
// Re-indexing is cheap: brainrag.RAGStore.IndexNote checks the stored
// SHA256 of each chunk before re-running the embedder, so unchanged files
// cost only a small ES read on every tick.
//
// This capability writes directly to glitch-vectors via brainrag.RAGStore
// rather than emitting Doc events through the channel. That's because the
// embedding pipeline needs per-chunk SHA-based dedup that the runner's
// generic BulkIndex path has no awareness of. The runner therefore sees
// zero Doc events from this capability — it's a "background worker" that
// tracks its own work via RecordRun heartbeats.
//
// Opt-in: only runs when the workspace config sets CodeIndex.Enabled and
// provides at least one path.
type CodeIndexCapability struct {
	Paths         []string
	Extensions    []string
	ChunkSize     int
	Interval      time.Duration
	EmbedProvider string
	EmbedModel    string
	EmbedBaseURL  string
	EmbedAPIKey   string
	WorkspaceID   string

	// ES is injected by the pod manager at construction time because the
	// capability talks to ES directly through brainrag, not through the
	// runner's Indexer. Kept as a field rather than an Invoke param so
	// the manifest stays the same shape as every other capability.
	ES *esearch.Client

	mu       sync.Mutex
	embedder brainrag.Embedder
}

func (c *CodeIndexCapability) Manifest() Manifest {
	every := c.Interval
	if every == 0 {
		every = 30 * time.Minute
	}
	return Manifest{
		Name:        "code-index",
		Description: "Semantic index over one or more source trees. Chunks every matching file, embeds each chunk via the configured provider (ollama, openai, or voyage), and upserts into glitch-vectors. Re-indexing is SHA-deduped per chunk.",
		Category:    "workspace.search",
		Trigger:     Trigger{Mode: TriggerInterval, Every: every},
		Sink:        Sink{}, // writes directly via brainrag.RAGStore
	}
}

func (c *CodeIndexCapability) Invoke(ctx context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event, 1)
	go func() {
		defer close(ch)
		if len(c.Paths) == 0 || c.ES == nil {
			return
		}
		c.mu.Lock()
		if c.embedder == nil {
			emb, err := c.buildEmbedder()
			if err != nil {
				c.mu.Unlock()
				ch <- Event{Kind: EventError, Err: err}
				return
			}
			c.embedder = emb
		}
		embedder := c.embedder
		c.mu.Unlock()

		for _, path := range c.Paths {
			if ctx.Err() != nil {
				return
			}
			abs := expandPath(path)
			if abs == "" {
				continue
			}
			if _, statErr := os.Stat(abs); statErr != nil {
				ch <- Event{Kind: EventError, Err: fmt.Errorf("path missing %s: %w", abs, statErr)}
				continue
			}
			store := brainrag.NewRAGStoreForCWD(c.ES, abs)
			res, err := brainrag.IndexTree(ctx, brainrag.IndexTreeOptions{
				Root:       abs,
				Extensions: c.Extensions,
				ChunkSize:  c.ChunkSize,
				Embedder:   embedder,
				Store:      store,
			})
			if err != nil {
				ch <- Event{Kind: EventError, Err: fmt.Errorf("index %s: %w", abs, err)}
				continue
			}
			slog.Info("code-index capability: indexed",
				"workspace", c.WorkspaceID,
				"path", filepath.Base(abs),
				"files", res.Files,
				"chunks", res.Chunks)
		}
	}()
	return ch, nil
}

func (c *CodeIndexCapability) buildEmbedder() (brainrag.Embedder, error) {
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

// expandPath resolves a "~"-prefixed path to the user's home directory and
// returns the absolute form. Empty input returns empty.
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
