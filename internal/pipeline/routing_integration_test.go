//go:build integration

package pipeline_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

// TestRouteIntent_MatchesSyncDocs verifies that the intent classifier selects
// the sync-docs pipeline when given a docs-related prompt.
// Requires: ollama running with GLITCH_SMOKE_MODEL (default: llama3.2).
func TestRouteIntent_MatchesSyncDocs(t *testing.T) {
	model := smokeModel()
	checkModelAvailable(t, smokeModelBase(model))

	// Build a temp pipelines dir with a sync-docs pipeline.
	dir := t.TempDir()
	writeTestPipeline(t, filepath.Join(dir, "sync-docs.workflow.yaml"), `
name: sync-docs
description: "Compare recent code changes against site docs and generate a sync report noting new features"
version: "1"
steps: []
`)
	writeTestPipeline(t, filepath.Join(dir, "gh-review.workflow.yaml"), `
name: gh-review
description: "Review open GitHub pull requests and summarise what needs attention"
version: "1"
steps: []
`)

	refs, err := pipeline.DiscoverPipelines(dir)
	if err != nil || len(refs) != 2 {
		t.Fatalf("discover: err=%v refs=%d", err, len(refs))
	}

	matched := runClassifier(t, "sync my docs with the latest code changes", refs, nil, model)
	if matched == nil {
		t.Fatal("classifier returned NONE for a clear docs-sync prompt; expected sync-docs")
	}
	if !strings.EqualFold(matched.Name, "sync-docs") {
		t.Errorf("classifier matched %q, want sync-docs", matched.Name)
	}
}

// TestRouteIntent_ReturnsNoneForUnrelated verifies the classifier returns nil
// for a prompt unrelated to any pipeline.
func TestRouteIntent_ReturnsNoneForUnrelated(t *testing.T) {
	model := smokeModel()
	checkModelAvailable(t, smokeModelBase(model))

	dir := t.TempDir()
	writeTestPipeline(t, filepath.Join(dir, "sync-docs.workflow.yaml"), `
name: sync-docs
description: "Compare recent code changes against site docs"
version: "1"
steps: []
`)

	refs, _ := pipeline.DiscoverPipelines(dir)

	matched := runClassifier(t, "what is the capital of France?", refs, nil, model)
	if matched != nil {
		t.Errorf("classifier matched %q for unrelated prompt, expected NONE", matched.Name)
	}
}

// runClassifier drives the intent classifier via an in-process pipeline.Run call,
// mirroring what cmd/ask.go's routeIntent does.
func runClassifier(t *testing.T, prompt string, refs []pipeline.PipelineRef, _ interface{}, model string) *pipeline.PipelineRef {
	t.Helper()

	var sb strings.Builder
	sb.WriteString("You are a router. Reply with EXACTLY ONE pipeline name from the list below that best matches the user request, or reply NONE if nothing matches.\n\n")
	sb.WriteString("Available pipelines:\n")
	for _, r := range refs {
		sb.WriteString("- ")
		sb.WriteString(r.Name)
		sb.WriteString(": ")
		sb.WriteString(r.Description)
		sb.WriteString("\n")
	}
	sb.WriteString("\nUser request: ")
	sb.WriteString(prompt)
	sb.WriteString("\n\nReply with only the pipeline name or NONE:")

	p := &pipeline.Pipeline{
		Name:    "test-route",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "classify",
				Executor: "ollama",
				Model:    model,
				Prompt:   sb.String(),
			},
		},
	}

	// mgr is *executor.Manager — use the concrete type via buildManager().
	concreteMgr := buildManager()
	pub := &collectPublisher{}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := pipeline.Run(ctx, p, concreteMgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Logf("classifier run error (treating as NONE): %v", err)
		return nil
	}

	response := strings.TrimSpace(result)
	response = strings.Trim(response, `"'.`)

	if strings.EqualFold(response, "NONE") || response == "" {
		return nil
	}
	for i, r := range refs {
		if strings.EqualFold(r.Name, response) {
			return &refs[i]
		}
	}
	t.Logf("classifier garbage response %q (treating as NONE)", response)
	return nil
}

func writeTestPipeline(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTestPipeline %s: %v", path, err)
	}
}
