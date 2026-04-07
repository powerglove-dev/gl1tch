//go:build integration

package brainrag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// mockOllama creates a test HTTP server that returns pre-set embeddings per call.
// embeddings is a slice of vectors; each successive call returns the next one (cyclically).
func mockOllama(t *testing.T, embeddings [][]float32) *httptest.Server {
	t.Helper()
	idx := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vec := embeddings[idx%len(embeddings)]
		idx++
		resp := map[string]any{"embedding": vec}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// openTestES dials the local Elasticsearch and skips the test if it
// isn't reachable. Each test gets a unique scope so parallel runs can
// share the same backing index without interference.
func openTestES(t *testing.T) *esearch.Client {
	t.Helper()
	es, err := esearch.New("")
	if err != nil {
		t.Skipf("esearch: %v", err)
	}
	if err := es.Ping(context.Background()); err != nil {
		t.Skipf("elasticsearch not available: %v", err)
	}
	if err := es.EnsureIndices(context.Background()); err != nil {
		t.Fatalf("ensure indices: %v", err)
	}
	return es
}

func TestRAGStore_IndexAndQuery(t *testing.T) {
	// Three distinct embeddings: v0, v1, v2.
	// v0 and v2 are similar (parallel), v1 is orthogonal.
	v0 := []float32{1, 0, 0}
	v1 := []float32{0, 1, 0}
	v2 := []float32{2, 0, 0} // parallel to v0

	// Call sequence for the mock server:
	// Index note-0 → v0
	// Index note-1 → v1
	// Index note-2 → v2
	// Query (embed query) → v0  (so note-0 and note-2 should rank higher than note-1)
	embeddings := [][]float32{v0, v1, v2, v0}
	srv := mockOllama(t, embeddings)
	emb := NewOllamaEmbedder(srv.URL, "test-model")

	es := openTestES(t)
	scope := "test:" + t.Name()
	rs := NewRAGStore(es, scope)

	ctx := context.Background()
	if err := rs.IndexNote(ctx, emb, "note-0", "text about Go"); err != nil {
		t.Fatalf("IndexNote 0: %v", err)
	}
	if err := rs.IndexNote(ctx, emb, "note-1", "text about Python"); err != nil {
		t.Fatalf("IndexNote 1: %v", err)
	}
	if err := rs.IndexNote(ctx, emb, "note-2", "text about Go interfaces"); err != nil {
		t.Fatalf("IndexNote 2: %v", err)
	}

	ids, err := rs.Query(ctx, emb, "query about Go", 2, nil)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(ids), ids)
	}
	// note-0 and note-2 should be in top-2 (both parallel to the query vector).
	idSet := map[string]bool{ids[0]: true, ids[1]: true}
	if !idSet["note-0"] || !idSet["note-2"] {
		t.Errorf("expected note-0 and note-2 in top-2, got: %v", ids)
	}
}

func TestRAGStore_IndexAndQuery_Filter(t *testing.T) {
	v0 := []float32{1, 0, 0}
	v1 := []float32{0, 1, 0}
	v2 := []float32{2, 0, 0}
	embeddings := [][]float32{v0, v1, v2, v0}
	srv := mockOllama(t, embeddings)
	emb := NewOllamaEmbedder(srv.URL, "test-model")

	es := openTestES(t)
	rs := NewRAGStore(es, "test:"+t.Name())

	ctx := context.Background()
	_ = rs.IndexNote(ctx, emb, "note-0", "text 0")
	_ = rs.IndexNote(ctx, emb, "note-1", "text 1")
	_ = rs.IndexNote(ctx, emb, "note-2", "text 2")

	// Filter to only note-1 even though query vector aligns with note-0 and note-2.
	ids, err := rs.Query(ctx, emb, "query", 5, []string{"note-1"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(ids) != 1 || ids[0] != "note-1" {
		t.Errorf("expected [note-1], got %v", ids)
	}
}

func TestRAGStore_IdempotentIndex(t *testing.T) {
	// Indexing the same note twice with the same content should not call Ollama twice.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := map[string]any{"embedding": []float32{1, 0}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	es := openTestES(t)
	rs := NewRAGStore(es, "test:"+t.Name())
	ctx := context.Background()
	emb := NewOllamaEmbedder(srv.URL, "test-model")

	_ = rs.IndexNote(ctx, emb, "note-x", "same text")
	_ = rs.IndexNote(ctx, emb, "note-x", "same text")

	if callCount != 1 {
		t.Errorf("expected 1 Ollama call for idempotent index, got %d", callCount)
	}
}
