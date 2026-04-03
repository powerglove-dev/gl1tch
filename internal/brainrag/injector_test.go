package brainrag

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/braincontext"
	"github.com/8op-org/gl1tch/internal/store"
)

// openTestStore opens a fresh SQLite store in a temp directory.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("openTestStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestBrainInjector_Unavailable(t *testing.T) {
	// Point injector at a non-existent server — Ollama unavailable.
	rs := NewRAGStore(openTestStore(t).DB(), "/test/cwd")
	s := openTestStore(t)

	inj := &BrainInjector{
		RAG:     rs,
		Store:   s,
		BaseURL: "http://127.0.0.1:19999", // nothing listening here
		Model:   "nomic-embed-text",
	}

	originalPrompt := "what is the meaning of life?"
	got, err := inj.InjectInto(context.Background(), originalPrompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != originalPrompt {
		t.Errorf("expected original prompt unchanged when Ollama unavailable, got: %q", got)
	}
}

func TestBrainInjector_FilterByLinkedNotes(t *testing.T) {
	ctx := context.Background()

	// Create two notes in the store.
	s := openTestStore(t)
	id1, _ := s.InsertBrainNote(ctx, store.BrainNote{RunID: 1, StepID: "s1", Body: "Go is great"})
	id2, _ := s.InsertBrainNote(ctx, store.BrainNote{RunID: 1, StepID: "s2", Body: "Python is cool"})

	// Both vectors are the same direction. We'll use filter to select only note id2.
	v := []float32{1, 0, 0}
	// 3 calls: index note1, index note2, embed query
	srv := mockOllama(t, [][]float32{v, v, v})

	rs := NewRAGStore(s.DB(), "/test/cwd")

	id1str := fmt.Sprintf("%d", id1)
	id2str := fmt.Sprintf("%d", id2)

	emb := NewOllamaEmbedder(srv.URL, "test-model")
	_ = rs.IndexNote(ctx, emb, id1str, "Go is great")
	_ = rs.IndexNote(ctx, emb, id2str, "Python is cool")

	inj := &BrainInjector{
		RAG:   rs,
		Store: s,
		WorkspaceCtx: braincontext.WorkspaceContext{
			LinkedNoteIDs: []string{id2str}, // only return note 2
		},
		TopK:     5,
		Embedder: emb,
	}

	result, err := inj.InjectInto(ctx, "tell me about Python")
	if err != nil {
		t.Fatalf("InjectInto: %v", err)
	}

	if !strings.Contains(result, "Python is cool") {
		t.Errorf("expected injected note body, got: %q", result)
	}
	if strings.Contains(result, "Go is great") {
		t.Errorf("filtered note should not appear, got: %q", result)
	}
}
