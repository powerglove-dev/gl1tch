// Package esearch provides the Elasticsearch client and index management for
// gl1tch's observer system. All observations, summaries, and insights flow
// through this package.
package esearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// Index names used by gl1tch.
const (
	IndexEvents    = "glitch-events"
	IndexSummaries = "glitch-summaries"
	IndexPipelines = "glitch-pipelines"
	IndexInsights  = "glitch-insights"
)

// Client wraps the Elasticsearch client with gl1tch-specific operations.
type Client struct {
	es *elasticsearch.Client
}

// New creates a new ES client. addr defaults to http://localhost:9200.
func New(addr string) (*Client, error) {
	if addr == "" {
		addr = "http://localhost:9200"
	}
	cfg := elasticsearch.Config{
		Addresses: []string{addr},
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("esearch: new client: %w", err)
	}
	return &Client{es: es}, nil
}

// Ping checks connectivity to Elasticsearch.
func (c *Client) Ping(ctx context.Context) error {
	res, err := c.es.Ping(c.es.Ping.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("esearch: ping: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("esearch: ping: %s", res.Status())
	}
	return nil
}

// EnsureIndices creates all gl1tch indices if they don't already exist.
func (c *Client) EnsureIndices(ctx context.Context) error {
	indices := map[string]string{
		IndexEvents:    eventsMapping,
		IndexSummaries: summariesMapping,
		IndexPipelines: pipelinesMapping,
		IndexInsights:  insightsMapping,
	}
	for name, mapping := range indices {
		// Check if index exists.
		res, err := c.es.Indices.Exists([]string{name}, c.es.Indices.Exists.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("esearch: check index %s: %w", name, err)
		}
		res.Body.Close()
		if res.StatusCode == 200 {
			continue
		}

		// Create it.
		res, err = c.es.Indices.Create(
			name,
			c.es.Indices.Create.WithBody(strings.NewReader(mapping)),
			c.es.Indices.Create.WithContext(ctx),
		)
		if err != nil {
			return fmt.Errorf("esearch: create index %s: %w", name, err)
		}
		res.Body.Close()
		if res.IsError() {
			return fmt.Errorf("esearch: create index %s: %s", name, res.Status())
		}
		slog.Info("esearch: created index", "name", name)
	}
	return nil
}

// Index indexes a single document. If id is empty, ES auto-generates one.
func (c *Client) Index(ctx context.Context, index string, id string, doc any) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("esearch: marshal: %w", err)
	}

	opts := []func(*esapi.IndexRequest){
		c.es.Index.WithContext(ctx),
		c.es.Index.WithRefresh("false"),
	}
	if id != "" {
		opts = append(opts, c.es.Index.WithDocumentID(id))
	}

	res, err := c.es.Index(index, bytes.NewReader(body), opts...)
	if err != nil {
		return fmt.Errorf("esearch: index: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("esearch: index %s: %s: %s", index, res.Status(), b)
	}
	return nil
}

// BulkIndex indexes multiple documents in a single bulk request.
func (c *Client) BulkIndex(ctx context.Context, index string, docs []any) error {
	if len(docs) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, doc := range docs {
		// Action line.
		buf.WriteString(`{"index":{"_index":"` + index + `"}}`)
		buf.WriteByte('\n')
		// Document line.
		b, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("esearch: bulk marshal: %w", err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}

	res, err := c.es.Bulk(
		&buf,
		c.es.Bulk.WithContext(ctx),
		c.es.Bulk.WithRefresh("false"),
	)
	if err != nil {
		return fmt.Errorf("esearch: bulk: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("esearch: bulk: %s: %s", res.Status(), b)
	}
	return nil
}

// SearchResult holds one hit from a search query.
type SearchResult struct {
	ID     string          `json:"_id"`
	Index  string          `json:"_index"`
	Score  float64         `json:"_score"`
	Source json.RawMessage `json:"_source"`
}

// SearchResponse holds the parsed ES search response.
type SearchResponse struct {
	Total   int64          `json:"total"`
	Results []SearchResult `json:"results"`
}

// Search executes an ES query DSL and returns parsed results.
func (c *Client) Search(ctx context.Context, indices []string, query map[string]any) (*SearchResponse, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("esearch: marshal query: %w", err)
	}

	res, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(indices...),
		c.es.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("esearch: search: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		b, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("esearch: search: %s: %s", res.Status(), b)
	}

	var raw struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				ID     string          `json:"_id"`
				Index  string          `json:"_index"`
				Score  float64         `json:"_score"`
				Source json.RawMessage `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("esearch: decode: %w", err)
	}

	resp := &SearchResponse{Total: raw.Hits.Total.Value}
	for _, h := range raw.Hits.Hits {
		resp.Results = append(resp.Results, SearchResult{
			ID:     h.ID,
			Index:  h.Index,
			Score:  h.Score,
			Source: h.Source,
		})
	}
	return resp, nil
}

// IsAvailable returns true if ES is reachable.
func (c *Client) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return c.Ping(ctx) == nil
}
