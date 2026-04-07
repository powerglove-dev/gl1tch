package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/8op-org/gl1tch/internal/brainrag"
	"github.com/8op-org/gl1tch/internal/esearch"
)

func init() {
	builtinRegistry["builtin.index_code"] = builtinIndexCode
}

// recommendEmbedModel returns the recommended Ollama embedding model and a
// human-readable rationale based on the number of source files to be indexed.
//
// Tiers:
//   - ≤500 files:   nomic-embed-text  — fast, low memory, sufficient for small repos
//   - ≤5 000 files: nomic-embed-text  — still fine; note indexing will take a minute or two
//   - ≤20 000 files: mxbai-embed-large — better recall justifies the extra time
//   - >20 000 files: mxbai-embed-large — warn user to narrow the path or increase chunk_size
func recommendEmbedModel(fileCount int) (model, rationale string) {
	switch {
	case fileCount <= 500:
		return "nomic-embed-text", "small repo — nomic-embed-text is fast and sufficient"
	case fileCount <= 5000:
		return "nomic-embed-text", fmt.Sprintf("%d files — nomic-embed-text works well; expect 1-3 min", fileCount)
	case fileCount <= 20000:
		return "mxbai-embed-large", fmt.Sprintf("%d files — mxbai-embed-large recommended for better recall at this scale", fileCount)
	default:
		return "mxbai-embed-large", fmt.Sprintf("%d files is a large corpus — consider narrowing 'path' to a subdirectory, or raising chunk_size to reduce chunk count. mxbai-embed-large recommended.", fileCount)
	}
}

// builtinIndexCode walks a path, chunks source files, embeds them, and stores
// the results in the RAG vector store (glitch-vectors index in Elasticsearch).
//
// Args:
//   - "path":            directory to walk (default ".")
//   - "extensions":      comma-separated list (default ".go,.ts,.py,.md")
//   - "model":           embedding model (default "nomic-embed-text"); Ollama compat
//   - "base_url":        Ollama base URL (default "http://localhost:11434"); Ollama compat
//   - "embed_provider":  "ollama" | "openai" | "voyage" (default "ollama")
//   - "embed_model":     provider-specific model (default: provider default)
//   - "embed_api_key":   literal key or "$ENV_VAR" (read from env if starts with "$")
//   - "embed_base_url":  Ollama base URL override (takes precedence over "base_url")
//   - "chunk_size":      max chars per chunk (default 1500)
func builtinIndexCode(ctx context.Context, args map[string]any, w io.Writer) (map[string]any, error) {
	root := toString(args["path"])
	if root == "" {
		root = "."
	}

	extStr := toString(args["extensions"])
	var exts []string
	if extStr == "" {
		exts = brainrag.DefaultCodeExtensions
	} else {
		for _, e := range strings.Split(extStr, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				exts = append(exts, e)
			}
		}
	}

	embedder, err := buildEmbedder(args)
	if err != nil {
		return nil, fmt.Errorf("builtin.index_code: %w", err)
	}

	// Keep "model" readable for the pre-scan recommendation logic.
	model := toString(args["model"])
	if model == "" {
		model = brainrag.DefaultEmbedModel
	}

	chunkSize := 1500
	if cs := toString(args["chunk_size"]); cs != "" {
		_, _ = fmt.Sscanf(cs, "%d", &chunkSize)
	}
	if chunkSize <= 0 {
		chunkSize = 1500
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = root
	}

	// Pre-scan: count eligible files so we can recommend a model and warn on large repos.
	prescanCount := brainrag.CountIndexableFiles(root, exts)
	recModel, recRationale := recommendEmbedModel(prescanCount)
	if w != nil {
		fmt.Fprintf(w, "pre-scan: %d source files found\n", prescanCount)
		fmt.Fprintf(w, "recommended model: %s (%s)\n", recModel, recRationale)
		if model != recModel {
			fmt.Fprintf(w, "note: using %q as specified; consider switching to %q for better results\n", model, recModel)
		}
		if prescanCount > 20000 {
			fmt.Fprintf(w, "warning: large corpus — consider narrowing 'path' to a subdirectory or increasing chunk_size to reduce indexing time\n")
		}
	}

	es, err := esearch.New("")
	if err != nil {
		return nil, fmt.Errorf("builtin.index_code: open es: %w", err)
	}
	if err := es.EnsureIndices(ctx); err != nil {
		return nil, fmt.Errorf("builtin.index_code: ensure es indices (is elasticsearch running?): %w", err)
	}

	rs := brainrag.NewRAGStoreForCWD(es, cwd)

	res, err := brainrag.IndexTree(ctx, brainrag.IndexTreeOptions{
		Root:       root,
		Extensions: exts,
		ChunkSize:  chunkSize,
		Embedder:   embedder,
		Store:      rs,
		Progress:   os.Stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("builtin.index_code: %w", err)
	}

	msg := fmt.Sprintf("indexed %d files, %d chunks", res.Files, res.Chunks)
	if w != nil {
		fmt.Fprintln(w, msg)
	}
	return map[string]any{"value": msg, "files": res.Files, "chunks": res.Chunks}, nil
}

// buildEmbedder constructs a brainrag.Embedder from pipeline action args.
// Arg resolution order for Ollama compat:
//   - "embed_provider" / "embed_model" / "embed_api_key" / "embed_base_url" take precedence.
//   - Legacy "model" and "base_url" map to the Ollama provider.
func buildEmbedder(args map[string]any) (brainrag.Embedder, error) {
	provider := toString(args["embed_provider"])
	embedModel := toString(args["embed_model"])
	rawKey := toString(args["embed_api_key"])
	embedBaseURL := toString(args["embed_base_url"])

	apiKey := rawKey
	if strings.HasPrefix(rawKey, "$") {
		apiKey = os.Getenv(rawKey[1:])
	}

	if provider == "" {
		provider = "ollama"
	}
	if embedBaseURL == "" {
		embedBaseURL = toString(args["base_url"])
	}
	if embedModel == "" && (provider == "ollama") {
		embedModel = toString(args["model"])
	}

	switch provider {
	case "", "ollama":
		return brainrag.NewOllamaEmbedder(embedBaseURL, embedModel), nil
	case "openai":
		if apiKey == "" {
			return nil, fmt.Errorf("embed_api_key is required for openai provider")
		}
		return brainrag.NewOpenAIEmbedder(apiKey, embedModel, 0), nil
	case "voyage":
		if apiKey == "" {
			return nil, fmt.Errorf("embed_api_key is required for voyage provider")
		}
		return brainrag.NewVoyageEmbedder(apiKey, embedModel), nil
	default:
		return nil, fmt.Errorf("unknown embed_provider %q", provider)
	}
}
