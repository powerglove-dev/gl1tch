package brainrag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// VoyageDefaultModel is the default Voyage embedding model.
const VoyageDefaultModel = "voyage-code-3"

// VoyageEmbedder calls the Voyage AI embeddings API.
type VoyageEmbedder struct {
	APIKey string
	Model  string
}

// NewVoyageEmbedder returns a VoyageEmbedder with the given config.
func NewVoyageEmbedder(apiKey, model string) *VoyageEmbedder {
	if model == "" {
		model = VoyageDefaultModel
	}
	return &VoyageEmbedder{APIKey: apiKey, Model: model}
}

// ID returns "voyage:{Model}".
func (e *VoyageEmbedder) ID() string {
	return "voyage:" + e.Model
}

// Embed calls the Voyage AI embeddings API with the given text.
func (e *VoyageEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := map[string]any{
		"model": e.Model,
		"input": []string{text},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("brainrag: marshal voyage embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.voyageai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("brainrag: create voyage embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brainrag: voyage embed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brainrag: voyage embed: unexpected status %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("brainrag: decode voyage embed response: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("brainrag: empty embedding returned from voyage model %q", e.Model)
	}

	raw := result.Data[0].Embedding
	vec := make([]float32, len(raw))
	for i, v := range raw {
		vec[i] = float32(v)
	}
	return vec, nil
}
