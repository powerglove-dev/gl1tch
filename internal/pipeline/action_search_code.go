package pipeline

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/8op-org/gl1tch/internal/brainrag"
	"github.com/8op-org/gl1tch/internal/esearch"
)

func init() {
	builtinRegistry["builtin.search_code"] = builtinSearchCode
}

// builtinSearchCode queries the RAG vector store for code chunks semantically
// similar to the given query string. Requires the codebase to have been indexed
// first with builtin.index_code.
//
// Args:
//   - "query":          text to embed and search for (required; no-ops if empty)
//   - "top_k":          number of chunks to return (default 6)
//   - "model":          embedding model (default "nomic-embed-text"); Ollama compat
//   - "base_url":       Ollama base URL (default "http://localhost:11434"); Ollama compat
//   - "embed_provider": "ollama" | "openai" | "voyage" (default "ollama")
//   - "embed_model":    provider-specific model (default: provider default)
//   - "embed_api_key":  literal key or "$ENV_VAR" (read from env if starts with "$")
//   - "embed_base_url": Ollama base URL override (takes precedence over "base_url")
//   - "cwd":            working directory scope used during indexing (default: current dir)
func builtinSearchCode(ctx context.Context, args map[string]any, w io.Writer) (map[string]any, error) {
	query := strings.TrimSpace(toString(args["query"]))
	if query == "" {
		return map[string]any{"value": "", "chunks": 0}, nil
	}

	topK := 6
	if v := strings.TrimSpace(toString(args["top_k"])); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			topK = n
		}
	}

	embedder, err := buildEmbedder(args)
	if err != nil {
		return nil, fmt.Errorf("builtin.search_code: %w", err)
	}

	cwd := toString(args["cwd"])

	es, err := esearch.New("")
	if err != nil {
		return nil, fmt.Errorf("builtin.search_code: open es: %w", err)
	}
	rs := brainrag.NewRAGStoreForCWD(es, cwd)

	entries, err := rs.QueryWithText(ctx, embedder, query, topK)
	if err != nil {
		return nil, fmt.Errorf("builtin.search_code: query: %w", err)
	}

	if len(entries) == 0 {
		return map[string]any{"value": "", "chunks": 0}, nil
	}

	var sb strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&sb, "=== %s ===\n%s\n\n", e.NoteID, e.Text)
	}

	result := strings.TrimRight(sb.String(), "\n")
	if w != nil {
		fmt.Fprintf(w, "found %d chunks for query %q\n", len(entries), query)
	}
	return map[string]any{"value": result, "chunks": len(entries)}, nil
}
