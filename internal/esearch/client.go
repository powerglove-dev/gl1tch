// Package esearch provides the Elasticsearch client and index management for
// gl1tch's observer system. All observations, summaries, and insights flow
// through this package.
package esearch

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
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
	// IndexVectors stores embedding vectors for brain notes and code
	// chunks. Replaces the SQLite-backed brainrag store. Uses a
	// dense_vector mapping with kNN search enabled.
	IndexVectors = "glitch-vectors"
	// IndexBrainDecisions is the per-decision audit log the brain
	// emits every time it routes a chain to a provider. Powers Kibana
	// dashboards for "confidence over time" and "local vs paid escalation
	// rate". Written to from pkg/glitchd/chain.go after each chain run.
	IndexBrainDecisions = "glitch-brain-decisions"
	// IndexTraces is the destination for OTel spans shipped by the
	// custom SpanExporter in internal/telemetry/elasticsearch_exporter.go.
	// Every span produced by the gl1tch process (pipeline runs, brain
	// cycles, executor dispatches, collector pod startup, refine loops)
	// lands here as one document for "what's happening right now in
	// this process" queries in Kibana Discover.
	IndexTraces = "glitch-traces"
	// IndexLogs is the destination for slog records teed out of the
	// in-process LogBuffer (pkg/glitchd/logbuffer.go) so log history
	// survives across restarts and wails dev rebuilds. Powers the
	// "show me the last ten workspace-pod startups" Kibana query that
	// makes screenshot round-trips unnecessary.
	IndexLogs = "glitch-logs"
	// IndexAnalyses is the destination for the deep-analysis loop's
	// per-event LLM overviews. Each doc carries the originating
	// event_key, source, repo, the model name used, the markdown the
	// LLM produced, and a workspace_id for scoping. The activity
	// sidebar fetches recent rows here when the user expands an
	// "analysis" entry; future Kibana dashboards can chart "what's
	// been analyzed today" without touching glitch-events.
	IndexAnalyses = "glitch-analyses"
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
		IndexEvents:         eventsMapping,
		IndexSummaries:      summariesMapping,
		IndexPipelines:      pipelinesMapping,
		IndexInsights:       insightsMapping,
		IndexVectors:        vectorsMapping,
		IndexBrainDecisions: brainDecisionsMapping,
		IndexTraces:         tracesMapping,
		IndexLogs:           logsMapping,
		IndexAnalyses:       analysesMapping,
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

// EnsureCustomIndex creates a single index with the caller-supplied
// mapping if it does not already exist. Used by subsystems (like the
// security capability) that own an index outside the core gl1tch set
// and don't want to touch the EnsureIndices map.
func (c *Client) EnsureCustomIndex(ctx context.Context, name, mapping string) error {
	res, err := c.es.Indices.Exists([]string{name}, c.es.Indices.Exists.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("esearch: check index %s: %w", name, err)
	}
	res.Body.Close()
	if res.StatusCode == 200 {
		return nil
	}
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
	slog.Info("esearch: created custom index", "name", name)
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
//
// For glitch-events the action line carries a deterministic _id
// derived from the doc's natural key (commit sha, issue/PR url,
// comment hash). That makes the index idempotent across collector
// re-runs — a git collector polling every 30s would otherwise
// pile up a fresh ES doc per tick for the same commit, which is
// exactly the "52 duplicates" bug we saw in the drill-in modal.
// See bulkEventID for the per-type key scheme.
//
// Other indices (analyses, summaries, pipelines, …) still rely on
// auto-generated ids because their natural key is already encoded
// in a payload field the caller wants versioned, not overwritten
// (e.g. each new analysis is its own historical record).
func (c *Client) BulkIndex(ctx context.Context, index string, docs []any) error {
	if len(docs) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, doc := range docs {
		b, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("esearch: bulk marshal: %w", err)
		}
		// Action line. For glitch-events we look up a deterministic
		// id; everything else falls through to auto-generated ids.
		if index == IndexEvents {
			if id := bulkEventID(b); id != "" {
				action := fmt.Sprintf(`{"index":{"_index":%q,"_id":%q}}`, index, id)
				buf.WriteString(action)
			} else {
				buf.WriteString(`{"index":{"_index":"` + index + `"}}`)
			}
		} else {
			buf.WriteString(`{"index":{"_index":"` + index + `"}}`)
		}
		buf.WriteByte('\n')
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

// bulkEventID derives a deterministic Elasticsearch document id
// for a glitch-events doc based on its type and natural key. The
// goal is idempotent re-indexing: the same underlying observation
// (a git commit, a github issue, a github PR comment) should land
// on the same _id every time, so collector re-runs overwrite the
// existing doc instead of creating a duplicate row.
//
// Key scheme, by type:
//
//	git.commit          → "git.commit:" + sha
//	git.push            → "git.push:" + sha (head of push)
//	github.issue        → "github.issue:" + metadata.url
//	github.pullrequest  → "github.pullrequest:" + metadata.url
//	github.issue_comment
//	github.pr_comment
//	github.pr_review
//	github.pr_check     → "<type>:" + repo + ":" + parent_number +
//	                      ":" + sha1(author+timestamp+body)[:16]
//	                      (these events don't carry per-object urls
//	                      in metadata, so we hash the fields that
//	                      together uniquely identify a comment /
//	                      review / check run)
//
// Anything else returns "" so BulkIndex falls back to the ES
// auto-generated id. That's the right behaviour for novel event
// types the collector adds later — they stay visible, and we add
// a specific id rule when the duplication actually hurts.
//
// doc is the already-marshalled JSON body so we only pay the
// json.Unmarshal cost when we actually need to look inside.
func bulkEventID(doc []byte) string {
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		return ""
	}
	typ, _ := m["type"].(string)
	if typ == "" {
		return ""
	}

	getStr := func(k string) string {
		s, _ := m[k].(string)
		return s
	}
	getMetaStr := func(k string) string {
		md, ok := m["metadata"].(map[string]any)
		if !ok {
			return ""
		}
		s, _ := md[k].(string)
		return s
	}
	getMetaInt := func(k string) int {
		md, ok := m["metadata"].(map[string]any)
		if !ok {
			return 0
		}
		switch v := md[k].(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
		return 0
	}

	switch typ {
	case "git.commit", "git.push":
		sha := getStr("sha")
		if sha == "" {
			return ""
		}
		return typ + ":" + sha

	case "github.issue", "github.pullrequest":
		url := getMetaStr("url")
		if url == "" {
			return ""
		}
		return typ + ":" + url

	case "github.issue_comment",
		"github.pr_comment",
		"github.pr_review",
		"github.pr_check":
		repo := getStr("repo")
		var parent int
		if typ == "github.issue_comment" {
			parent = getMetaInt("issue_number")
		} else {
			parent = getMetaInt("pr_number")
		}
		if repo == "" || parent == 0 {
			return ""
		}
		h := sha1.New()
		fmt.Fprintln(h, getStr("author"))
		fmt.Fprintln(h, getStr("message"))
		fmt.Fprintln(h, getStr("body"))
		// timestamp is a JSON string (RFC3339) when marshalled from
		// esearch.Event, or a time.Time value for tests; Fprintln
		// handles both via default formatting.
		fmt.Fprintln(h, m["timestamp"])
		sum := hex.EncodeToString(h.Sum(nil))[:16]
		return fmt.Sprintf("%s:%s:%d:%s", typ, repo, parent, sum)
	}

	return ""
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

// DeleteByQuery removes documents matching query from the given
// indices. Used by the brainrag migration to clear stale vectors when
// scope or embed_id changes. Returns the number of deleted docs.
func (c *Client) DeleteByQuery(ctx context.Context, indices []string, query map[string]any) (int64, error) {
	body, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		return 0, err
	}
	res, err := c.es.DeleteByQuery(
		indices,
		bytes.NewReader(body),
		c.es.DeleteByQuery.WithContext(ctx),
		c.es.DeleteByQuery.WithRefresh(true),
	)
	if err != nil {
		return 0, fmt.Errorf("esearch: delete-by-query: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		b, _ := io.ReadAll(res.Body)
		return 0, fmt.Errorf("esearch: delete-by-query: %s: %s", res.Status(), b)
	}
	var parsed struct {
		Deleted int64 `json:"deleted"`
	}
	_ = json.NewDecoder(res.Body).Decode(&parsed)
	return parsed.Deleted, nil
}

// VectorHit is a single result from VectorSearch.
type VectorHit struct {
	NoteID string  `json:"note_id"`
	Text   string  `json:"text"`
	Score  float64 `json:"score"`
}

// VectorSearch runs a kNN search against IndexVectors with the given
// query embedding, scoped to a single scope keyword and (optionally)
// filtered to a set of note_ids. Returns up to topK hits sorted by
// similarity score (highest first).
//
// Powers the new ES-backed brainrag.RAGStore. The "filter" path lets
// callers narrow the kNN search to notes linked to a specific
// workspace, matching the old SQLite store's behavior.
func (c *Client) VectorSearch(
	ctx context.Context,
	scope string,
	embedID string,
	query []float32,
	topK int,
	noteIDFilter []string,
) ([]VectorHit, error) {
	if topK <= 0 {
		topK = 5
	}

	// Build the bool filter: scope is required, embed_id is required
	// (so we never mix dimensions), note_id filter is optional.
	must := []map[string]any{
		{"term": map[string]any{"scope": scope}},
		{"term": map[string]any{"embed_id": embedID}},
	}
	if len(noteIDFilter) > 0 {
		must = append(must, map[string]any{
			"terms": map[string]any{"note_id": noteIDFilter},
		})
	}

	body := map[string]any{
		"size": topK,
		"_source": []string{"note_id", "text"},
		// num_candidates ~= 10x topK is the standard ES recommendation
		// for HNSW recall vs. latency tradeoff.
		"knn": map[string]any{
			"field":          "vector",
			"query_vector":   query,
			"k":              topK,
			"num_candidates": topK * 10,
			"filter": map[string]any{
				"bool": map[string]any{"must": must},
			},
		},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	res, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(IndexVectors),
		c.es.Search.WithBody(bytes.NewReader(raw)),
	)
	if err != nil {
		return nil, fmt.Errorf("esearch: vector search: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		b, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("esearch: vector search: %s: %s", res.Status(), b)
	}

	var parsed struct {
		Hits struct {
			Hits []struct {
				Score  float64 `json:"_score"`
				Source struct {
					NoteID string `json:"note_id"`
					Text   string `json:"text"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("esearch: decode vector search: %w", err)
	}

	out := make([]VectorHit, 0, len(parsed.Hits.Hits))
	for _, h := range parsed.Hits.Hits {
		out = append(out, VectorHit{
			NoteID: h.Source.NoteID,
			Text:   h.Source.Text,
			Score:  h.Score,
		})
	}
	return out, nil
}
