package brainrag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OllamaEmbedder calls Ollama /api/embeddings to produce embeddings.
type OllamaEmbedder struct {
	BaseURL string
	Model   string
}

// NewOllamaEmbedder returns an OllamaEmbedder with defaults filled in.
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if model == "" {
		model = DefaultEmbedModel
	}
	return &OllamaEmbedder{BaseURL: baseURL, Model: model}
}

// ID returns "ollama:{Model}".
func (e *OllamaEmbedder) ID() string {
	return "ollama:" + e.Model
}

// Embed calls Ollama /api/embeddings with the given text.
func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(embedRequest{Model: e.Model, Prompt: text})
	if err != nil {
		return nil, fmt.Errorf("brainrag: marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/api/embeddings", bytes.NewReader(body))
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
		return nil, fmt.Errorf("brainrag: empty embedding returned for model %q", e.Model)
	}
	return er.Embedding, nil
}
