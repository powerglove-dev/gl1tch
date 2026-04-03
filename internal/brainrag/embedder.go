package brainrag

import "context"

// Embedder is the interface implemented by all embedding backends.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	// ID returns a stable "provider:model" string used to detect provider switches.
	// e.g. "ollama:nomic-embed-text", "openai:text-embedding-3-small", "voyage:voyage-code-3"
	ID() string
}
