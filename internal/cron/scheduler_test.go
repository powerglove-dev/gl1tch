package cron

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	s := New(nil, nil)

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
	s := New(nil, nil)
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
