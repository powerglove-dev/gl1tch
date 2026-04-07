//go:build integration

package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// requireES skips the test unless a local Elasticsearch is reachable.
// builtin.index_code now writes to ES instead of SQLite, so chunking
// behavior can only be exercised end-to-end against a live cluster.
//
// NOTE: ES infers dense_vector dimensionality from the first indexed
// document, so a previous run that used a 3-dim mock vector will lock
// the glitch-vectors index at 3 dims and break subsequent runs with
// real 768-dim Ollama embeddings. We delete + recreate the index in
// the test setup to keep runs deterministic.
func requireES(t *testing.T) *esearch.Client {
	t.Helper()
	es, err := esearch.New("")
	if err != nil {
		t.Skipf("esearch: %v", err)
	}
	if err := es.Ping(context.Background()); err != nil {
		t.Skipf("elasticsearch not available: %v", err)
	}
	// Wipe + recreate so the dim-lock from any earlier run doesn't
	// poison this one.
	_, _ = es.DeleteByQuery(context.Background(), []string{esearch.IndexVectors}, map[string]any{"match_all": map[string]any{}})
	if err := es.EnsureIndices(context.Background()); err != nil {
		t.Fatalf("ensure indices: %v", err)
	}
	return es
}

func TestIndexCodeAction_Chunking(t *testing.T) {
	requireES(t)
	// Integration test: verify chunking logic with a mock Ollama server
	// against a real ES backend.

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := map[string]any{"embedding": []float32{1, 0, 0}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	dir := t.TempDir()

	// Create a Go file that is definitely larger than one chunk (chunkSize=200).
	var sb strings.Builder
	for i := range 50 {
		sb.WriteString(fmt.Sprintf("// Line %d: This is a comment in a Go file used for testing the indexer.\n", i))
	}
	goContent := sb.String()

	goFile := dir + "/test.go"
	if err := os.WriteFile(goFile, []byte(goContent), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	storePath := dir + "/brain.vectors.json"
	args := map[string]any{
		"path":       dir,
		"extensions": ".go",
		"model":      "nomic-embed-text",
		"base_url":   srv.URL,
		"chunk_size": "200", // small chunk to force multiple chunks
		"store_path": storePath,
	}

	out, err := builtinIndexCode(context.Background(), args, nil)
	if err != nil {
		t.Fatalf("builtinIndexCode: %v", err)
	}

	files, _ := out["files"].(int)
	chunks, _ := out["chunks"].(int)

	if files != 1 {
		t.Errorf("expected 1 file indexed, got %d", files)
	}
	if chunks < 2 {
		t.Errorf("expected at least 2 chunks for large file (content=%d chars, chunkSize=200), got %d", len(goContent), chunks)
	}
	if callCount != chunks {
		t.Errorf("expected %d Ollama calls (one per chunk), got %d", chunks, callCount)
	}
}

func TestIndexCodeAction_SkipDirs(t *testing.T) {
	requireES(t)
	dir := t.TempDir()

	// Create a vendor directory with a Go file that should be skipped.
	vendorDir := dir + "/vendor/pkg"
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatalf("mkdir vendor: %v", err)
	}
	if err := os.WriteFile(vendorDir+"/file.go", []byte("package pkg\n"), 0o644); err != nil {
		t.Fatalf("write vendor file: %v", err)
	}
	// Create a normal Go file.
	if err := os.WriteFile(dir+"/main.go", []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float32{1, 0}})
	}))
	defer srv.Close()

	out, err := builtinIndexCode(context.Background(), map[string]any{
		"path":       dir,
		"extensions": ".go",
		"base_url":   srv.URL,
		"store_path": dir + "/brain.vectors.json",
	}, nil)
	if err != nil {
		t.Fatalf("builtinIndexCode: %v", err)
	}

	files, _ := out["files"].(int)
	if files != 1 {
		t.Errorf("expected 1 file (vendor should be skipped), got %d", files)
	}
}
