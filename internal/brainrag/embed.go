// Package brainrag provides vector embedding and RAG (retrieval-augmented generation)
// for brain notes using Ollama.
package brainrag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

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
// Returns an error if Ollama is unavailable or returns a non-200 response.
func Embed(ctx context.Context, baseURL, model, text string) ([]float32, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if model == "" {
		model = DefaultEmbedModel
	}

	body, err := json.Marshal(embedRequest{Model: model, Prompt: text})
	if err != nil {
		return nil, fmt.Errorf("brainrag: marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("brainrag: create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brainrag: embed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brainrag: embed: unexpected status %d: %s", resp.StatusCode, string(data))
	}

	var er embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, fmt.Errorf("brainrag: decode embed response: %w", err)
	}
	if len(er.Embedding) == 0 {
		return nil, fmt.Errorf("brainrag: empty embedding returned for model %q", model)
	}
	return er.Embedding, nil
}
