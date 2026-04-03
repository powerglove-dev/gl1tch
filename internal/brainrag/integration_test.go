//go:build integration

package brainrag

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// checkModelAvailable skips the test if the named ollama model is not present.
func checkModelAvailable(t *testing.T, model string) {
	t.Helper()
	out, err := exec.Command("ollama", "list").Output()
	if err != nil {
		t.Skipf("ollama not available: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if idx := strings.Index(name, ":"); idx >= 0 {
			name = name[:idx]
		}
		if strings.EqualFold(name, model) {
			return
		}
	}
	t.Skipf("model not available: %s", model)
}

// TestEmbed_RealOllama calls real Ollama and verifies a non-empty vector.
func TestEmbed_RealOllama(t *testing.T) {
	checkModelAvailable(t, "nomic-embed-text")

	vec, err := Embed(context.Background(), DefaultBaseURL, DefaultEmbedModel, "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) == 0 {
		t.Fatal("expected non-empty embedding vector")
	}
	t.Logf("embedding dimension: %d", len(vec))
}

// TestRAGStore_RealIndexAndQuery indexes 3 short texts and queries for the most similar.
func TestRAGStore_RealIndexAndQuery(t *testing.T) {
	checkModelAvailable(t, "nomic-embed-text")

	s := openTestStore(t)
	rs := NewRAGStore(s.DB(), "/test/cwd")
	emb := NewOllamaEmbedder(DefaultBaseURL, DefaultEmbedModel)

	ctx := context.Background()
	texts := []struct {
		id   string
		text string
	}{
		{"note-go", "Go is a statically typed compiled programming language designed at Google."},
		{"note-py", "Python is an interpreted high-level general-purpose programming language."},
		{"note-rust", "Rust is a systems programming language focusing on safety, speed, and concurrency."},
	}

	for _, tt := range texts {
		if err := rs.IndexNote(ctx, emb, tt.id, tt.text); err != nil {
			t.Fatalf("IndexNote %s: %v", tt.id, err)
		}
	}

	// Query for something about Go.
	ids, err := rs.Query(ctx, emb, "compiled language designed at Google", 1, nil)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("expected at least one result")
	}
	if ids[0] != "note-go" {
		t.Errorf("expected top result to be note-go, got %q", ids[0])
	}
}
