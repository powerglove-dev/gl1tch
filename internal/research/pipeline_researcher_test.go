package research

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// writeMinimalPipeline creates a syntactically valid pipeline YAML on disk so
// PipelineResearcher's loadPipelineFile path is exercised. The actual run is
// stubbed via SetPipelineRunner so we never spawn an executor.
func writeMinimalPipeline(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name+".pipeline.yaml")
	yaml := "name: " + name + "\nsteps:\n  - id: in\n    type: input\n  - id: out\n    type: output\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write pipeline: %v", err)
	}
	return path
}

// stubRunner returns a pipelineRunFn that always yields the given output.
// captures the userInput so tests can assert the question was forwarded.
func stubRunner(out string, capturedInput *string) func(ctx context.Context, p *pipeline.Pipeline, mgr *executor.Manager, userInput string) (string, error) {
	return func(_ context.Context, _ *pipeline.Pipeline, _ *executor.Manager, userInput string) (string, error) {
		if capturedInput != nil {
			*capturedInput = userInput
		}
		return out, nil
	}
}

func TestPipelineResearcherValidEvidence(t *testing.T) {
	body := `{"schema_version":1,"source":"github-prs","title":"recent PRs","body":"PR #412 open","refs":["https://github.com/8op-org/gl1tch/pull/412"]}`
	path := writeMinimalPipeline(t, "github-prs")

	r := NewPipelineResearcher("github-prs", "lists open PRs", path, nil)
	var captured string
	r.SetPipelineRunner(stubRunner(body, &captured))

	ev, err := r.Gather(context.Background(), ResearchQuery{Question: "any open PRs?"}, EvidenceBundle{})
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if ev.Source != "github-prs" {
		t.Errorf("Source = %q, want github-prs", ev.Source)
	}
	if !strings.Contains(ev.Body, "PR #412") {
		t.Errorf("Body = %q", ev.Body)
	}
	if len(ev.Refs) != 1 || !strings.Contains(ev.Refs[0], "/pull/412") {
		t.Errorf("Refs = %v", ev.Refs)
	}
	if captured != "any open PRs?" {
		t.Errorf("question not forwarded to pipeline: got %q", captured)
	}
}

func TestPipelineResearcherSourceOverriddenByName(t *testing.T) {
	// The pipeline self-reports source "from-pipeline" but the researcher
	// is registered as "github-prs". The researcher's name must win so the
	// planner gets unambiguous attribution.
	body := `{"schema_version":1,"source":"from-pipeline","title":"t","body":"b"}`
	path := writeMinimalPipeline(t, "p")

	r := NewPipelineResearcher("github-prs", "x", path, nil)
	r.SetPipelineRunner(stubRunner(body, nil))

	ev, err := r.Gather(context.Background(), ResearchQuery{Question: "q"}, EvidenceBundle{})
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if ev.Source != "github-prs" {
		t.Errorf("Source = %q, want github-prs (researcher name wins)", ev.Source)
	}
}

func TestPipelineResearcherTolerantOfNoiseAroundJSON(t *testing.T) {
	// Real pipelines often print log lines before/after the JSON object. The
	// parser must extract the object cleanly.
	out := "INFO starting run\n" +
		`{"schema_version":1,"source":"x","title":"t","body":"b"}` + "\n" +
		"INFO done\n"
	path := writeMinimalPipeline(t, "noisy")

	r := NewPipelineResearcher("noisy", "noisy pipeline", path, nil)
	r.SetPipelineRunner(stubRunner(out, nil))

	ev, err := r.Gather(context.Background(), ResearchQuery{}, EvidenceBundle{})
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if ev.Body != "b" {
		t.Errorf("Body = %q, want b", ev.Body)
	}
}

func TestPipelineResearcherMalformedOutputIsAnError(t *testing.T) {
	path := writeMinimalPipeline(t, "broken")

	r := NewPipelineResearcher("broken", "broken", path, nil)
	r.SetPipelineRunner(stubRunner("oops not json", nil))

	_, err := r.Gather(context.Background(), ResearchQuery{Question: "q"}, EvidenceBundle{})
	if err == nil {
		t.Fatal("expected error for malformed pipeline output, got nil")
	}
	if !errors.Is(err, ErrEvidenceMalformed) {
		t.Errorf("expected wrapped ErrEvidenceMalformed, got %v", err)
	}
	if !strings.Contains(err.Error(), "preview=") {
		t.Errorf("error should include preview of pipeline output: %v", err)
	}
}

func TestPipelineResearcherSchemaMismatch(t *testing.T) {
	path := writeMinimalPipeline(t, "futurev")

	r := NewPipelineResearcher("futurev", "future schema", path, nil)
	r.SetPipelineRunner(stubRunner(`{"schema_version":99,"source":"x","title":"t","body":"b"}`, nil))

	_, err := r.Gather(context.Background(), ResearchQuery{Question: "q"}, EvidenceBundle{})
	if err == nil || !errors.Is(err, ErrEvidenceSchemaMismatch) {
		t.Errorf("expected schema mismatch error, got %v", err)
	}
}

func TestPipelineResearcherFileNotFound(t *testing.T) {
	r := NewPipelineResearcher("missing", "x", "/nonexistent/path.pipeline.yaml", nil)
	_, err := r.Gather(context.Background(), ResearchQuery{Question: "q"}, EvidenceBundle{})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestPipelineResearcherRunErrorPropagates(t *testing.T) {
	want := errors.New("pipeline blew up")
	path := writeMinimalPipeline(t, "boom")

	r := NewPipelineResearcher("boom", "boom", path, nil)
	r.SetPipelineRunner(func(_ context.Context, _ *pipeline.Pipeline, _ *executor.Manager, _ string) (string, error) {
		return "", want
	})

	_, err := r.Gather(context.Background(), ResearchQuery{Question: "q"}, EvidenceBundle{})
	if err == nil || !strings.Contains(err.Error(), "pipeline blew up") {
		t.Errorf("expected wrapped run error, got %v", err)
	}
}
