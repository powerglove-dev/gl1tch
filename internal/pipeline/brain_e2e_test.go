//go:build integration

package pipeline_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/powerglove-dev/gl1tch/internal/braincontext"
	"github.com/powerglove-dev/gl1tch/internal/brainrag"
	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/pipeline"
	"github.com/powerglove-dev/gl1tch/internal/store"
)

// TestBrainE2E_IndexAndQuery_Ollama uses real Ollama to index pipeline source
// files and verify that a BrainInjector provides relevant context.
func TestBrainE2E_IndexAndQuery_Ollama(t *testing.T) {
	checkModelAvailable(t, "nomic-embed-text")

	ctx := context.Background()
	s := openTestStore(t)
	rs := brainrag.NewRAGStore(s.DB(), "/test/e2e")

	// Index 5 Go source snippets from the orcai codebase.
	snippets := []struct {
		id   string
		text string
	}{
		{"snippet-pipeline", "Package pipeline provides the execution engine for orcai pipelines."},
		{"snippet-brain", "The brain system accumulates notes from pipeline steps for future context injection."},
		{"snippet-runner", "The runner executes steps sequentially or in parallel using the DAG engine."},
		{"snippet-store", "The store persists pipeline run results and brain notes in SQLite."},
		{"snippet-brainrag", "The brainrag package provides vector embedding and RAG for brain notes using Ollama."},
	}

	for _, snippet := range snippets {
		if err := rs.IndexNote(ctx, brainrag.DefaultBaseURL, brainrag.DefaultEmbedModel, snippet.id, snippet.text); err != nil {
			t.Fatalf("IndexNote %s: %v", snippet.id, err)
		}
	}

	// Create a store with the snippets as brain notes.
	for _, snippet := range snippets {
		_, _ = s.InsertBrainNote(ctx, store.BrainNote{
			RunID:  1,
			StepID: snippet.id,
			Body:   snippet.text,
		})
	}

	allNotes, err := s.AllBrainNotes(ctx)
	if err != nil {
		t.Fatalf("AllBrainNotes: %v", err)
	}

	// Re-index with proper IDs from store.
	rs2 := brainrag.NewRAGStore(s.DB(), "/test/e2e/store")
	for _, n := range allNotes {
		_ = rs2.IndexNote(ctx, brainrag.DefaultBaseURL, brainrag.DefaultEmbedModel,
			fmt.Sprintf("%d", n.ID), n.Body)
	}

	inj := &brainrag.BrainInjector{
		RAG:   rs2,
		Store: s,
		WorkspaceCtx: braincontext.WorkspaceContext{
			WorkspaceType: braincontext.WorkspacePipeline,
		},
		TopK:    3,
		BaseURL: brainrag.DefaultBaseURL,
		Model:   brainrag.DefaultEmbedModel,
	}

	// Run a pipeline step that will have brain context injected.
	mgr := executor.NewManager()
	var capturedPrompt string
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "capture",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			capturedPrompt = input
			_, err := w.Write([]byte("captured"))
			return err
		},
	})

	p := &pipeline.Pipeline{
		Name: "brain-e2e-test",
		Steps: []pipeline.Step{
			{ID: "s1", Executor: "capture", Prompt: "tell me about the brain system"},
		},
	}

	_, err = pipeline.Run(ctx, p, mgr, "",
		pipeline.WithRAGStore(rs2),
		pipeline.WithBrainRAGInjector(inj),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(capturedPrompt, "## Relevant Brain Notes") {
		t.Errorf("expected brain notes injected into prompt, got: %q", capturedPrompt[:min(200, len(capturedPrompt))])
	}
}
