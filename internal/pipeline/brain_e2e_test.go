//go:build integration

package pipeline_test

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adam-stokes/orcai/internal/braincontext"
	"github.com/adam-stokes/orcai/internal/brainrag"
	"github.com/adam-stokes/orcai/internal/pipeline"
	"github.com/adam-stokes/orcai/internal/plugin"
	"github.com/adam-stokes/orcai/internal/store"
)

// TestBrainE2E_IndexAndQuery_Ollama uses real Ollama to index pipeline source
// files and verify that a BrainInjector provides relevant context.
func TestBrainE2E_IndexAndQuery_Ollama(t *testing.T) {
	checkModelAvailable(t, "nomic-embed-text")

	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "brain.vectors.json")
	rs, err := brainrag.NewRAGStore(storePath)
	if err != nil {
		t.Fatalf("NewRAGStore: %v", err)
	}

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

	for _, s := range snippets {
		if err := rs.IndexNote(ctx, brainrag.DefaultBaseURL, brainrag.DefaultEmbedModel, s.id, s.text); err != nil {
			t.Fatalf("IndexNote %s: %v", s.id, err)
		}
	}

	// Create a store with the snippets as brain notes.
	s := openTestStore(t)
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
	storePath2 := filepath.Join(t.TempDir(), "brain2.vectors.json")
	rs2, _ := brainrag.NewRAGStore(storePath2)
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
	mgr := plugin.NewManager()
	var capturedPrompt string
	_ = mgr.Register(&plugin.StubPlugin{
		PluginName: "capture",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			capturedPrompt = input
			_, err := w.Write([]byte("captured"))
			return err
		},
	})

	p := &pipeline.Pipeline{
		Name: "brain-e2e-test",
		Steps: []pipeline.Step{
			{ID: "s1", Plugin: "capture", Prompt: "tell me about the brain system"},
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
