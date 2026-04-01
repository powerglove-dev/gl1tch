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

// TestBrainE2E_CodebaseUpdate_HighConfidence demonstrates brain-guided codebase
// modification using RAG context from the orcai codebase.
func TestBrainE2E_CodebaseUpdate_HighConfidence(t *testing.T) {
	checkModelAvailable(t, "nomic-embed-text")
	checkModelAvailable(t, "llama3.2")

	ctx := context.Background()

	// Step 1: index_code for internal/pipeline/*.go files.
	args := map[string]any{
		"path":       "../../internal/pipeline",
		"extensions": ".go",
		"model":      brainrag.DefaultEmbedModel,
		"base_url":   brainrag.DefaultBaseURL,
		"chunk_size": "1500",
	}

	mgr := executor.NewManager()

	// Run the index_code builtin directly (not via pipeline).
	// We run a pipeline that uses builtin.index_code.
	indexPipeline := &pipeline.Pipeline{
		Name: "index-pipeline-code",
		Steps: []pipeline.Step{
			{
				ID:       "index",
				Executor: "builtin.index_code",
				Args:     args,
			},
		},
	}
	_, err := pipeline.Run(ctx, indexPipeline, mgr, "")
	if err != nil {
		t.Fatalf("index_code pipeline: %v", err)
	}

	// Step 2: set up store and RAG injector.
	s := openTestStore(t)

	// Index the brain.go file content as a brain note.
	_, _ = s.InsertBrainNote(ctx, store.BrainNote{
		RunID:  1,
		StepID: "brain-file",
		Body:   "The brainWriteInstruction constant in brain.go instructs the AI to write a <brain_notes> block. It is appended to prompts when write_brain is active.",
	})

	allNotes, _ := s.AllBrainNotes(ctx)
	rs := brainrag.NewRAGStore(s.DB(), t.TempDir())
	for _, n := range allNotes {
		_ = rs.IndexNote(ctx, brainrag.DefaultBaseURL, brainrag.DefaultEmbedModel,
			fmt.Sprintf("%d", n.ID), n.Body)
	}

	inj := &brainrag.BrainInjector{
		RAG:          rs,
		Store:        s,
		WorkspaceCtx: braincontext.Empty(),
		TopK:         3,
		BaseURL:      brainrag.DefaultBaseURL,
		Model:        brainrag.DefaultEmbedModel,
	}

	// Step 3: run a write_brain pipeline step that uses the RAG context.
	trueVal := true
	var capturedOutput string

	// Register a capture plugin to capture llama3.2 output (or use existing).
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "capture-output",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			// Simulate model output for testing purposes.
			output := `const brainWriteInstruction = ` + "`" + `
// brainWriteInstruction is appended to prompts to request a brain note output block.
---
BRAIN NOTE INSTRUCTION: Include a <brain_notes> block somewhere in your response.
` + "`"
			capturedOutput = output
			_, err := w.Write([]byte(output))
			return err
		},
	})

	p := &pipeline.Pipeline{
		Name:       "codebase-update-test",
		WriteBrain: true,
		Steps: []pipeline.Step{
			{
				ID:         "query-step",
				Executor:     "capture-output",
				WriteBrain: &trueVal,
				Prompt: `Based on the code context, add a single-line comment improvement to brain.go:
the brainWriteInstruction constant. Output ONLY the exact replacement constant
declaration with the improved doc comment, nothing else.`,
			},
		},
	}

	_, err = pipeline.Run(ctx, p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithRAGStore(rs),
		pipeline.WithBrainRAGInjector(inj),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Step 4: verify the output is a valid Go const declaration.
	if !strings.Contains(capturedOutput, "const brainWriteInstruction") {
		t.Errorf("expected const brainWriteInstruction in output, got: %q",
			capturedOutput[:min(200, len(capturedOutput))])
	}
}
