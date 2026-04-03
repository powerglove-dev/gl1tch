// Package brainrag provides vector embedding and RAG (retrieval-augmented generation)
// for brain notes using Ollama.
package brainrag

import "context"

// DefaultBaseURL is the default Ollama API base URL.
const DefaultBaseURL = "http://localhost:11434"

// DefaultEmbedModel is the default embedding model used for brain RAG.
const DefaultEmbedModel = "nomic-embed-text"

// embedRequest is the JSON body sent to Ollama /api/embeddings.
type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// embedResponse is the JSON response from Ollama /api/embeddings.
type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed calls Ollama /api/embeddings with the given text.
// This is a backward-compatibility shim; new code should use OllamaEmbedder directly.
func Embed(ctx context.Context, baseURL, model, text string) ([]float32, error) {
	return NewOllamaEmbedder(baseURL, model).Embed(ctx, text)
}
