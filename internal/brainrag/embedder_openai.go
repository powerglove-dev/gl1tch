package brainrag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OpenAIDefaultModel is the default OpenAI embedding model.
const OpenAIDefaultModel = "text-embedding-3-small"

// OpenAIEmbedder calls the OpenAI embeddings API.
type OpenAIEmbedder struct {
	APIKey     string
	Model      string
	Dimensions int // optional; if > 0, truncated dimensions are requested
}

// NewOpenAIEmbedder returns an OpenAIEmbedder with the given config.
func NewOpenAIEmbedder(apiKey, model string, dimensions int) *OpenAIEmbedder {
	if model == "" {
		model = OpenAIDefaultModel
	}
	return &OpenAIEmbedder{APIKey: apiKey, Model: model, Dimensions: dimensions}
}

// ID returns "openai:{Model}".
func (e *OpenAIEmbedder) ID() string {
	return "openai:" + e.Model
}

// Embed calls the OpenAI embeddings API with the given text.
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := map[string]any{
		"model": e.Model,
		"input": text,
	}
	if e.Dimensions > 0 {
		reqBody["dimensions"] = e.Dimensions
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("brainrag: marshal openai embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("brainrag: create openai embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brainrag: openai embed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brainrag: openai embed: unexpected status %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("brainrag: decode openai embed response: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("brainrag: empty embedding returned from openai model %q", e.Model)
	}

	raw := result.Data[0].Embedding
	vec := make([]float32, len(raw))
	for i, v := range raw {
		vec[i] = float32(v)
	}
	return vec, nil
}
