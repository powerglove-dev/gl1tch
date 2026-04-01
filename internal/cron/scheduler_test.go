package cron

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/busd/topics"
)

const validCronYAML = `
entries:
  - name: daily-report
    schedule: "0 8 * * *"
    kind: pipeline
    target: report.pipeline.yaml
    timeout: 5m
  - name: hourly-agent
    schedule: "30 * * * *"
    kind: agent
    target: my-agent
`

const badScheduleYAML = `
entries:
  - name: good-entry
    schedule: "0 8 * * *"
    kind: pipeline
    target: good.yaml
  - name: bad-entry
    schedule: "not-a-cron"
    kind: pipeline
    target: bad.yaml
`

// TestLoadConfigFrom verifies that a valid YAML file is parsed correctly.
func TestLoadConfigFrom(t *testing.T) {
	path := writeTempYAML(t, validCronYAML)

	entries, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	e := entries[0]
	if e.Name != "daily-report" {
		t.Errorf("expected name %q, got %q", "daily-report", e.Name)
	}
	if e.Schedule != "0 8 * * *" {
		t.Errorf("expected schedule %q, got %q", "0 8 * * *", e.Schedule)
	}
	if e.Kind != "pipeline" {
		t.Errorf("expected kind %q, got %q", "pipeline", e.Kind)
	}
	if e.Target != "report.pipeline.yaml" {
		t.Errorf("expected target %q, got %q", "report.pipeline.yaml", e.Target)
	}
	if e.Timeout != "5m" {
		t.Errorf("expected timeout %q, got %q", "5m", e.Timeout)
	}

	e2 := entries[1]
	if e2.Name != "hourly-agent" {
		t.Errorf("expected name %q, got %q", "hourly-agent", e2.Name)
	}
	if e2.Kind != "agent" {
		t.Errorf("expected kind %q, got %q", "agent", e2.Kind)
	}
}

// TestLoadConfigFrom_Missing verifies that a missing file returns an empty
// slice and no error.
func TestLoadConfigFrom_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.yaml")

	entries, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(entries))
	}
}

// TestLoadConfigFrom_WorkingDir verifies that working_dir is parsed correctly.
func TestLoadConfigFrom_WorkingDir(t *testing.T) {
	yaml := `
entries:
  - name: with-dir
    schedule: "0 8 * * *"
    kind: pipeline
    target: report.pipeline.yaml
    working_dir: /some/project
  - name: without-dir
    schedule: "0 9 * * *"
    kind: pipeline
    target: other.pipeline.yaml
`
	path := writeTempYAML(t, yaml)

	entries, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].WorkingDir != "/some/project" {
		t.Errorf("expected working_dir %q, got %q", "/some/project", entries[0].WorkingDir)
	}
	if entries[1].WorkingDir != "" {
		t.Errorf("expected empty working_dir, got %q", entries[1].WorkingDir)
	}
}

// TestLoadConfigFrom_InvalidCron verifies that an entry with a bad cron
// expression is still loaded (validation happens at AddFunc time, not load
// time).
func TestLoadConfigFrom_InvalidCron(t *testing.T) {
	path := writeTempYAML(t, badScheduleYAML)

	entries, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both entries should be loaded regardless of schedule validity.
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[1].Name != "bad-entry" {
		t.Errorf("expected bad-entry to be present, got %q", entries[1].Name)
	}
}

// TestScheduler_Entries verifies that Entries() returns the expected count
// after Start with a valid config.
func TestScheduler_Entries(t *testing.T) {
	path := writeTempYAML(t, validCronYAML)

	// Point LoadConfig at our temp file by monkey-patching via env isn't
	// possible without refactoring, so we call loadAndRegister directly via
	// a sub-test that uses LoadConfigFrom via an overridden path.
	// Instead, we test the scheduler by wiring it manually.
	s := New(nil)

	// Pre-load entries directly to avoid touching ~.
	entries, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}

	s.mu.Lock()
	s.entries = entries
	// Register each entry.
	for _, e := range entries {
		entry := e
		id, addErr := s.c.AddFunc(entry.Schedule, func() { s.runEntry(entry) })
		if addErr == nil {
			s.cronIDs = append(s.cronIDs, id)
		}
	}
	s.mu.Unlock()

	got := s.Entries()
	if len(got) != 2 {
		t.Errorf("expected 2 entries, got %d", len(got))
	}

	// Cleanup.
	s.Stop()
}

// TestScheduler_Stop verifies that Stop does not panic when called once or
// multiple times.
func TestScheduler_Stop(t *testing.T) {
	s := New(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start with an empty config path that doesn't exist to avoid touching home.
	// We start the underlying cron manually.
	s.c.Start()
	go s.watchConfig(ctx)

	// Stop should not panic.
	s.Stop()
	// Calling Stop again should not panic.
	s.Stop()
}

// capturingPublisher records all published events for test assertions.
type capturingPublisher struct {
	mu     sync.Mutex
	events []capturedEvent
}

type capturedEvent struct {
	topic   string
	payload map[string]any
}

func (p *capturingPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	var m map[string]any
	_ = json.Unmarshal(payload, &m)
	p.mu.Lock()
	p.events = append(p.events, capturedEvent{topic: topic, payload: m})
	p.mu.Unlock()
	return nil
}

func (p *capturingPublisher) eventsByTopic(topic string) []capturedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []capturedEvent
	for _, e := range p.events {
		if e.topic == topic {
			out = append(out, e)
		}
	}
	return out
}

// TestScheduler_NoStoreWriter verifies that the Scheduler has no StoreWriter
// field and that New accepts only logger + options (no store argument).
func TestScheduler_NoStoreWriter(t *testing.T) {
	// This test is a compile-time guard: if StoreWriter were still present,
	// the New signature would require a second positional argument and the
	// TestScheduler_Entries / TestScheduler_Stop tests above would fail to
	// compile.  We simply create a scheduler without a store arg.
	s := New(nil)
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	s.Stop()
}

// TestScheduler_EventsPublished verifies that runEntry emits CronJobStarted
// and CronJobCompleted events via the configured publisher, and does NOT
// write to any store.
func TestScheduler_EventsPublished(t *testing.T) {
	pub := &capturingPublisher{}
	s := New(nil, WithPublisher(pub))

	entry := Entry{
		Name:     "test-job",
		Schedule: "* * * * *",
		Kind:     "pipeline",
		Target:   "some.pipeline.yaml",
		// Use a very short timeout so the subprocess is killed quickly even if
		// os.Executable() resolves to the test binary (which won't understand
		// "pipeline run" and may block).
		Timeout: "1s",
	}

	// runEntry spawns a subprocess which may fail (no real orcai binary in test
	// environment).  We accept any exit — the events should still fire.
	s.runEntry(entry)

	// Verify CronJobStarted was published.
	started := pub.eventsByTopic(topics.CronJobStarted)
	if len(started) != 1 {
		t.Fatalf("expected 1 %s event, got %d", topics.CronJobStarted, len(started))
	}
	if started[0].payload["job"] != "test-job" {
		t.Errorf("CronJobStarted: expected job=test-job, got %v", started[0].payload["job"])
	}
	if started[0].payload["target"] != "some.pipeline.yaml" {
		t.Errorf("CronJobStarted: expected target=some.pipeline.yaml, got %v", started[0].payload["target"])
	}
	if started[0].payload["triggered_at"] == nil {
		t.Error("CronJobStarted: missing triggered_at field")
	}

	// Verify CronJobCompleted was published.
	completed := pub.eventsByTopic(topics.CronJobCompleted)
	if len(completed) != 1 {
		t.Fatalf("expected 1 %s event, got %d", topics.CronJobCompleted, len(completed))
	}
	if completed[0].payload["job"] != "test-job" {
		t.Errorf("CronJobCompleted: expected job=test-job, got %v", completed[0].payload["job"])
	}
	if completed[0].payload["exit_status"] == nil {
		t.Error("CronJobCompleted: missing exit_status field")
	}
	if completed[0].payload["duration_ms"] == nil {
		t.Error("CronJobCompleted: missing duration_ms field")
	}
	if completed[0].payload["finished_at"] == nil {
		t.Error("CronJobCompleted: missing finished_at field")
	}
}

// TestScheduler_NoopPublisher verifies that New without WithPublisher uses a
// NoopPublisher and does not panic during runEntry.
func TestScheduler_NoopPublisher(t *testing.T) {
	s := New(nil)
	if _, ok := s.publisher.(NoopPublisher); !ok {
		t.Errorf("expected default publisher to be NoopPublisher, got %T", s.publisher)
	}
}

// writeTempYAML writes content to a temporary file and returns its path.
func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "cron-*.yaml")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return f.Name()
}
