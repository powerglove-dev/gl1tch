//go:build !integration

package router

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

func TestFeedbackLogger_Disabled(t *testing.T) {
	// Empty CacheDir → logger is disabled; Record is a no-op, no file created.
	l := NewFeedbackLogger("")
	l.Record("run git-pulse", &RouteResult{Method: "embedding", Confidence: 0.92, Pipeline: &pipeline.PipelineRef{Name: "git-pulse"}})
	// No assertion needed — must not panic or write any file.
}

func TestFeedbackLogger_WritesRecord(t *testing.T) {
	dir := t.TempDir()
	l := NewFeedbackLogger(dir)

	ref := &pipeline.PipelineRef{Name: "git-pulse"}
	result := &RouteResult{
		Pipeline:   ref,
		Confidence: 0.92,
		Method:     "embedding",
	}
	l.Record("run git-pulse", result)

	// Exactly one line should be in the file.
	lines := readJSONLLines(t, filepath.Join(dir, "routing-feedback.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 record, got %d", len(lines))
	}

	var rec FeedbackRecord
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	if rec.Prompt != "run git-pulse" {
		t.Errorf("Prompt = %q, want %q", rec.Prompt, "run git-pulse")
	}
	if rec.Pipeline != "git-pulse" {
		t.Errorf("Pipeline = %q, want %q", rec.Pipeline, "git-pulse")
	}
	if rec.Confidence != 0.92 {
		t.Errorf("Confidence = %f, want 0.92", rec.Confidence)
	}
	if rec.Method != "embedding" {
		t.Errorf("Method = %q, want %q", rec.Method, "embedding")
	}
	if rec.Timestamp == "" {
		t.Error("Timestamp must not be empty")
	}
}

func TestFeedbackLogger_NoMatch(t *testing.T) {
	dir := t.TempDir()
	l := NewFeedbackLogger(dir)

	result := &RouteResult{Method: "none", Confidence: 0}
	l.Record("why is the build failing?", result)

	lines := readJSONLLines(t, filepath.Join(dir, "routing-feedback.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 record, got %d", len(lines))
	}

	var rec FeedbackRecord
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	if rec.Pipeline != "" {
		t.Errorf("Pipeline should be empty for no-match, got %q", rec.Pipeline)
	}
	if rec.Method != "none" {
		t.Errorf("Method = %q, want %q", rec.Method, "none")
	}
}

func TestFeedbackLogger_NearMissRecorded(t *testing.T) {
	dir := t.TempDir()
	l := NewFeedbackLogger(dir)

	result := &RouteResult{
		Method:        "none",
		NearMiss:      &pipeline.PipelineRef{Name: "pr-review"},
		NearMissScore: 0.62,
	}
	l.Record("run something related", result)

	lines := readJSONLLines(t, filepath.Join(dir, "routing-feedback.jsonl"))
	var rec FeedbackRecord
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rec.NearMiss != "pr-review" {
		t.Errorf("NearMiss = %q, want %q", rec.NearMiss, "pr-review")
	}
}

func TestFeedbackLogger_Appends(t *testing.T) {
	dir := t.TempDir()
	l := NewFeedbackLogger(dir)

	for i := 0; i < 3; i++ {
		l.Record("run git-pulse", &RouteResult{Method: "llm", Pipeline: &pipeline.PipelineRef{Name: "git-pulse"}})
	}

	lines := readJSONLLines(t, filepath.Join(dir, "routing-feedback.jsonl"))
	if len(lines) != 3 {
		t.Errorf("expected 3 records after 3 calls, got %d", len(lines))
	}
}

func TestFeedbackLogger_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	l := NewFeedbackLogger(dir)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			l.Record("run git-pulse", &RouteResult{Method: "embedding", Pipeline: &pipeline.PipelineRef{Name: "git-pulse"}})
		}()
	}
	wg.Wait()

	lines := readJSONLLines(t, filepath.Join(dir, "routing-feedback.jsonl"))
	if len(lines) != n {
		t.Errorf("expected %d records after concurrent writes, got %d", n, len(lines))
	}
}

func TestFeedbackLogger_NilResult(t *testing.T) {
	dir := t.TempDir()
	l := NewFeedbackLogger(dir)
	// Must not panic on nil result.
	l.Record("run git-pulse", nil)
	// File should not exist (no record written).
	if _, err := os.Stat(filepath.Join(dir, "routing-feedback.jsonl")); err == nil {
		t.Error("no file should be created for nil result")
	}
}

// readJSONLLines reads a JSONL file and returns non-empty lines.
func readJSONLLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read feedback file: %v", err)
	}
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
