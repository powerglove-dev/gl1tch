package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// openTestStore creates a Store backed by a temporary directory.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRecordRunStart(t *testing.T) {
	s := openTestStore(t)

	id, err := s.RecordRunStart("pipeline", "my-pipeline", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}
	if id <= 0 {
		t.Errorf("want id > 0, got %d", id)
	}
}

func TestRecordRunStart_WithMetadata(t *testing.T) {
	s := openTestStore(t)

	id, err := s.RecordRunStart("pipeline", "meta-test", `{"cwd":"/tmp","pipeline_file":"/tmp/foo.yaml"}`)
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}

	runs, err := s.QueryRuns(1)
	if err != nil {
		t.Fatalf("QueryRuns: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("want 1 run, got 0")
	}
	if runs[0].ID != id {
		t.Errorf("want id %d, got %d", id, runs[0].ID)
	}
	if runs[0].Metadata != `{"cwd":"/tmp","pipeline_file":"/tmp/foo.yaml"}` {
		t.Errorf("want metadata blob, got %q", runs[0].Metadata)
	}
}

func TestRecordRunComplete(t *testing.T) {
	s := openTestStore(t)

	id, err := s.RecordRunStart("agent", "my-agent", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}

	if err := s.RecordRunComplete(id, 0, "hello stdout", "hello stderr"); err != nil {
		t.Fatalf("RecordRunComplete: %v", err)
	}

	runs, err := s.QueryRuns(10)
	if err != nil {
		t.Fatalf("QueryRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("want 1 run, got %d", len(runs))
	}
	r := runs[0]
	if r.ID != id {
		t.Errorf("want id %d, got %d", id, r.ID)
	}
	if r.ExitStatus == nil {
		t.Fatal("want exit_status set, got nil")
	}
	if *r.ExitStatus != 0 {
		t.Errorf("want exit_status 0, got %d", *r.ExitStatus)
	}
	if r.Stdout != "hello stdout" {
		t.Errorf("want stdout 'hello stdout', got %q", r.Stdout)
	}
	if r.Stderr != "hello stderr" {
		t.Errorf("want stderr 'hello stderr', got %q", r.Stderr)
	}
	if r.FinishedAt == nil {
		t.Error("want finished_at set, got nil")
	}
}

func TestQueryRuns(t *testing.T) {
	s := openTestStore(t)

	// Insert three runs with distinct timestamps.
	names := []string{"first", "second", "third"}
	for _, n := range names {
		id, err := s.RecordRunStart("pipeline", n, "")
		if err != nil {
			t.Fatalf("RecordRunStart(%s): %v", n, err)
		}
		if err := s.RecordRunComplete(id, 0, "", ""); err != nil {
			t.Fatalf("RecordRunComplete(%s): %v", n, err)
		}
		// Small sleep to ensure distinct started_at values.
		time.Sleep(2 * time.Millisecond)
	}

	runs, err := s.QueryRuns(10)
	if err != nil {
		t.Fatalf("QueryRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("want 3 runs, got %d", len(runs))
	}

	// Verify descending order: third should come first.
	if runs[0].Name != "third" {
		t.Errorf("want first result 'third', got %q", runs[0].Name)
	}
	if runs[2].Name != "first" {
		t.Errorf("want last result 'first', got %q", runs[2].Name)
	}

	// Verify limit is respected.
	limited, err := s.QueryRuns(2)
	if err != nil {
		t.Fatalf("QueryRuns(limit=2): %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("want 2 runs with limit=2, got %d", len(limited))
	}
}

func TestDeleteRun(t *testing.T) {
	s := openTestStore(t)

	id, err := s.RecordRunStart("pipeline", "to-delete", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}

	if err := s.DeleteRun(id); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}

	runs, err := s.QueryRuns(10)
	if err != nil {
		t.Fatalf("QueryRuns: %v", err)
	}
	for _, r := range runs {
		if r.ID == id {
			t.Errorf("run %d still present after DeleteRun", id)
		}
	}
}

func TestAutoPrune_ByAge(t *testing.T) {
	s := openTestStore(t)

	// Insert a run directly with a very old started_at (100 days ago in millis).
	// Since we are in the same package, we can access s.db directly.
	oldMillis := time.Now().Add(-100 * 24 * time.Hour).UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO runs (kind, name, started_at) VALUES ('pipeline', 'old-run', ?)`,
		oldMillis,
	)
	if err != nil {
		t.Fatalf("insert old run: %v", err)
	}

	// Insert a fresh run.
	_, freshErr := s.RecordRunStart("pipeline", "fresh-run", "")
	if freshErr != nil {
		t.Fatalf("RecordRunStart fresh: %v", freshErr)
	}

	// Prune with maxAgeDays=30 — the old run (100 days ago) should be removed.
	if err := s.AutoPrune(30, 10000); err != nil {
		t.Fatalf("AutoPrune: %v", err)
	}

	runs, err := s.QueryRuns(100)
	if err != nil {
		t.Fatalf("QueryRuns: %v", err)
	}
	for _, r := range runs {
		if r.Name == "old-run" {
			t.Error("old-run should have been pruned but still exists")
		}
	}
	found := false
	for _, r := range runs {
		if r.Name == "fresh-run" {
			found = true
		}
	}
	if !found {
		t.Error("fresh-run should still exist after pruning")
	}
}

func TestAutoPrune_ByCount(t *testing.T) {
	s := openTestStore(t)

	// Insert 15 runs.
	for i := 0; i < 15; i++ {
		id, err := s.RecordRunStart("pipeline", fmt.Sprintf("run-%02d", i), "")
		if err != nil {
			t.Fatalf("RecordRunStart run-%02d: %v", i, err)
		}
		if err := s.RecordRunComplete(id, 0, "", ""); err != nil {
			t.Fatalf("RecordRunComplete run-%02d: %v", i, err)
		}
	}

	// Prune with maxRows=10 — oldest 5 should be removed.
	if err := s.AutoPrune(3650, 10); err != nil {
		t.Fatalf("AutoPrune: %v", err)
	}

	runs, err := s.QueryRuns(100)
	if err != nil {
		t.Fatalf("QueryRuns: %v", err)
	}
	if len(runs) != 10 {
		t.Errorf("want 10 runs after prune, got %d", len(runs))
	}
}

func TestWALMode(t *testing.T) {
	s := openTestStore(t)

	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("want journal_mode=wal, got %q", mode)
	}
}

func TestConcurrentWrites(t *testing.T) {
	s := openTestStore(t)

	const goroutines = 10
	errs := make([]error, goroutines)
	ids := make([]int64, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			id, err := s.RecordRunStart("agent", fmt.Sprintf("concurrent-%d", i), "")
			ids[i] = id
			errs[i] = err
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: RecordRunStart error: %v", i, err)
		}
		if ids[i] <= 0 {
			t.Errorf("goroutine %d: want id > 0, got %d", i, ids[i])
		}
	}

	runs, err := s.QueryRuns(goroutines + 10)
	if err != nil {
		t.Fatalf("QueryRuns: %v", err)
	}
	if len(runs) != goroutines {
		t.Errorf("want %d runs, got %d", goroutines, len(runs))
	}
}

// ---------------------------------------------------------------------------
// RecordStepComplete tests
// ---------------------------------------------------------------------------

func TestRecordStepComplete_Append(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	runID, err := s.RecordRunStart("pipeline", "step-test", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}

	steps := []StepRecord{
		{ID: "fetch", Status: "done", StartedAt: "2024-01-01T00:00:00Z", FinishedAt: "2024-01-01T00:00:01Z", DurationMs: 1000, Output: map[string]any{"value": "hello"}},
		{ID: "process", Status: "done", StartedAt: "2024-01-01T00:00:01Z", FinishedAt: "2024-01-01T00:00:02Z", DurationMs: 500},
	}

	for _, step := range steps {
		if err := s.RecordStepComplete(ctx, runID, step); err != nil {
			t.Fatalf("RecordStepComplete(%s): %v", step.ID, err)
		}
	}

	runs, err := s.QueryRuns(1)
	if err != nil {
		t.Fatalf("QueryRuns: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("want 1 run, got 0")
	}
	r := runs[0]
	if len(r.Steps) != 2 {
		t.Fatalf("want 2 steps, got %d", len(r.Steps))
	}
	if r.Steps[0].ID != "fetch" {
		t.Errorf("want steps[0].ID = fetch, got %q", r.Steps[0].ID)
	}
	if r.Steps[1].ID != "process" {
		t.Errorf("want steps[1].ID = process, got %q", r.Steps[1].ID)
	}
	if r.Steps[0].DurationMs != 1000 {
		t.Errorf("want steps[0].DurationMs = 1000, got %d", r.Steps[0].DurationMs)
	}
}

func TestRecordStepComplete_Upsert(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	runID, err := s.RecordRunStart("pipeline", "upsert-test", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}

	// Write initial step record.
	initial := StepRecord{ID: "fetch", Status: "running", StartedAt: "2024-01-01T00:00:00Z"}
	if err := s.RecordStepComplete(ctx, runID, initial); err != nil {
		t.Fatalf("RecordStepComplete initial: %v", err)
	}

	// Upsert the same step ID with updated status.
	updated := StepRecord{ID: "fetch", Status: "done", StartedAt: "2024-01-01T00:00:00Z", FinishedAt: "2024-01-01T00:00:01Z", DurationMs: 1000}
	if err := s.RecordStepComplete(ctx, runID, updated); err != nil {
		t.Fatalf("RecordStepComplete update: %v", err)
	}

	runs, err := s.QueryRuns(1)
	if err != nil {
		t.Fatalf("QueryRuns: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("want 1 run, got 0")
	}
	r := runs[0]
	// Should still be exactly 1 step (upserted, not duplicated).
	if len(r.Steps) != 1 {
		t.Fatalf("want 1 step after upsert, got %d", len(r.Steps))
	}
	if r.Steps[0].Status != "done" {
		t.Errorf("want upserted step status = done, got %q", r.Steps[0].Status)
	}
	if r.Steps[0].FinishedAt != "2024-01-01T00:00:01Z" {
		t.Errorf("want upserted step finished_at, got %q", r.Steps[0].FinishedAt)
	}
}

func TestRecordStepComplete_TableDriven(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	runID, err := s.RecordRunStart("pipeline", "table-driven", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}

	cases := []struct {
		name       string
		step       StepRecord
		wantSteps  int
		wantStatus string
	}{
		{
			name:       "first step",
			step:       StepRecord{ID: "a", Status: "done", DurationMs: 100},
			wantSteps:  1,
			wantStatus: "done",
		},
		{
			name:       "second step",
			step:       StepRecord{ID: "b", Status: "done", DurationMs: 200},
			wantSteps:  2,
			wantStatus: "done",
		},
		{
			name:       "upsert first step",
			step:       StepRecord{ID: "a", Status: "failed", DurationMs: 150},
			wantSteps:  2, // still 2, not 3
			wantStatus: "failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.RecordStepComplete(ctx, runID, tc.step); err != nil {
				t.Fatalf("RecordStepComplete: %v", err)
			}
			runs, err := s.QueryRuns(1)
			if err != nil {
				t.Fatalf("QueryRuns: %v", err)
			}
			if len(runs) == 0 {
				t.Fatal("want 1 run, got 0")
			}
			r := runs[0]
			if len(r.Steps) != tc.wantSteps {
				t.Errorf("want %d steps, got %d", tc.wantSteps, len(r.Steps))
			}
			// Check the step that was just written/updated.
			var found *StepRecord
			for i := range r.Steps {
				if r.Steps[i].ID == tc.step.ID {
					found = &r.Steps[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("step %q not found in run steps", tc.step.ID)
			}
			if found.Status != tc.wantStatus {
				t.Errorf("want step %q status = %q, got %q", tc.step.ID, tc.wantStatus, found.Status)
			}
		})
	}
}

func TestRecordStepComplete_UnknownRunID(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	step := StepRecord{ID: "x", Status: "done"}
	err := s.RecordStepComplete(ctx, 99999, step)
	if err == nil {
		t.Fatal("want error for unknown run ID, got nil")
	}
}

// ---------------------------------------------------------------------------
// QueryRuns step population tests
// ---------------------------------------------------------------------------

func TestQueryRuns_StepsEmptyByDefault(t *testing.T) {
	s := openTestStore(t)

	_, err := s.RecordRunStart("pipeline", "no-steps", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}

	runs, err := s.QueryRuns(1)
	if err != nil {
		t.Fatalf("QueryRuns: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("want 1 run, got 0")
	}
	if runs[0].Steps == nil {
		t.Error("want Steps to be non-nil empty slice, got nil")
	}
	if len(runs[0].Steps) != 0 {
		t.Errorf("want 0 steps, got %d", len(runs[0].Steps))
	}
}

func TestQueryRuns_StepsPopulated(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	runID, err := s.RecordRunStart("pipeline", "with-steps", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}

	want := []StepRecord{
		{ID: "s1", Status: "done", StartedAt: "2024-06-01T10:00:00Z", FinishedAt: "2024-06-01T10:00:05Z", DurationMs: 5000, Output: map[string]any{"value": "ok"}},
		{ID: "s2", Status: "failed", StartedAt: "2024-06-01T10:00:05Z", FinishedAt: "2024-06-01T10:00:06Z", DurationMs: 1000},
	}
	for _, step := range want {
		if err := s.RecordStepComplete(ctx, runID, step); err != nil {
			t.Fatalf("RecordStepComplete(%s): %v", step.ID, err)
		}
	}

	runs, err := s.QueryRuns(1)
	if err != nil {
		t.Fatalf("QueryRuns: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("want 1 run, got 0")
	}
	r := runs[0]
	if len(r.Steps) != len(want) {
		t.Fatalf("want %d steps, got %d", len(want), len(r.Steps))
	}
	for i, w := range want {
		got := r.Steps[i]
		if got.ID != w.ID {
			t.Errorf("[%d] ID: want %q, got %q", i, w.ID, got.ID)
		}
		if got.Status != w.Status {
			t.Errorf("[%d] Status: want %q, got %q", i, w.Status, got.Status)
		}
		if got.DurationMs != w.DurationMs {
			t.Errorf("[%d] DurationMs: want %d, got %d", i, w.DurationMs, got.DurationMs)
		}
	}
}

// ---------------------------------------------------------------------------
// Prompt CRUD tests
// ---------------------------------------------------------------------------

func TestInsertPrompt(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		prompt  Prompt
		wantErr bool
	}{
		{
			name:   "basic insert returns positive id",
			prompt: Prompt{Title: "Hello", Body: "Say hello", ModelSlug: "gpt-4"},
		},
		{
			name:   "empty model_slug allowed",
			prompt: Prompt{Title: "No model", Body: "No model body", ModelSlug: ""},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := time.Now().Unix()
			id, err := s.InsertPrompt(ctx, tc.prompt)
			after := time.Now().Unix()

			if (err != nil) != tc.wantErr {
				t.Fatalf("InsertPrompt() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if id <= 0 {
				t.Errorf("want id > 0, got %d", id)
			}

			// Verify timestamps were set.
			got, err := s.GetPrompt(ctx, id)
			if err != nil {
				t.Fatalf("GetPrompt: %v", err)
			}
			if got.CreatedAt < before || got.CreatedAt > after {
				t.Errorf("created_at %d not in [%d, %d]", got.CreatedAt, before, after)
			}
			if got.UpdatedAt < before || got.UpdatedAt > after {
				t.Errorf("updated_at %d not in [%d, %d]", got.UpdatedAt, before, after)
			}
			if got.Title != tc.prompt.Title {
				t.Errorf("title: want %q, got %q", tc.prompt.Title, got.Title)
			}
			if got.ModelSlug != tc.prompt.ModelSlug {
				t.Errorf("model_slug: want %q, got %q", tc.prompt.ModelSlug, got.ModelSlug)
			}
		})
	}
}

func TestUpdatePrompt(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	id, err := s.InsertPrompt(ctx, Prompt{Title: "Original", Body: "Original body", ModelSlug: "gpt-3"})
	if err != nil {
		t.Fatalf("InsertPrompt: %v", err)
	}

	cases := []struct {
		name    string
		prompt  Prompt
		wantErr bool
	}{
		{
			name:   "updates existing prompt",
			prompt: Prompt{ID: id, Title: "Updated", Body: "Updated body", ModelSlug: "gpt-4"},
		},
		{
			name:    "error on missing id",
			prompt:  Prompt{ID: 99999, Title: "Ghost", Body: "Ghost body", ModelSlug: ""},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := time.Now().Unix()
			err := s.UpdatePrompt(ctx, tc.prompt)
			after := time.Now().Unix()

			if (err != nil) != tc.wantErr {
				t.Fatalf("UpdatePrompt() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			got, err := s.GetPrompt(ctx, tc.prompt.ID)
			if err != nil {
				t.Fatalf("GetPrompt after update: %v", err)
			}
			if got.Body != tc.prompt.Body {
				t.Errorf("body: want %q, got %q", tc.prompt.Body, got.Body)
			}
			if got.UpdatedAt < before || got.UpdatedAt > after {
				t.Errorf("updated_at %d not in [%d, %d]", got.UpdatedAt, before, after)
			}
		})
	}
}

func TestDeletePrompt(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	id, err := s.InsertPrompt(ctx, Prompt{Title: "To Delete", Body: "Delete me", ModelSlug: ""})
	if err != nil {
		t.Fatalf("InsertPrompt: %v", err)
	}

	cases := []struct {
		name    string
		id      int64
		wantErr bool
	}{
		{
			name: "deletes existing prompt",
			id:   id,
		},
		{
			name:    "error on missing id",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.DeletePrompt(ctx, tc.id)
			if (err != nil) != tc.wantErr {
				t.Fatalf("DeletePrompt() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			_, err = s.GetPrompt(ctx, tc.id)
			if err != sql.ErrNoRows {
				t.Errorf("GetPrompt after delete: want sql.ErrNoRows, got %v", err)
			}
		})
	}
}

func TestListPrompts(t *testing.T) {
	ctx := context.Background()

	t.Run("empty slice on empty table", func(t *testing.T) {
		s := openTestStore(t)
		prompts, err := s.ListPrompts(ctx)
		if err != nil {
			t.Fatalf("ListPrompts: %v", err)
		}
		if prompts == nil {
			t.Error("want non-nil empty slice, got nil")
		}
		if len(prompts) != 0 {
			t.Errorf("want 0 prompts, got %d", len(prompts))
		}
	})

	t.Run("returns newest-first by updated_at", func(t *testing.T) {
		s := openTestStore(t)

		// Insert three prompts; sleep briefly to ensure distinct updated_at.
		names := []string{"first", "second", "third"}
		for _, n := range names {
			_, err := s.InsertPrompt(ctx, Prompt{Title: n, Body: n + " body", ModelSlug: ""})
			if err != nil {
				t.Fatalf("InsertPrompt(%s): %v", n, err)
			}
			time.Sleep(2 * time.Millisecond)
		}

		prompts, err := s.ListPrompts(ctx)
		if err != nil {
			t.Fatalf("ListPrompts: %v", err)
		}
		if len(prompts) != 3 {
			t.Fatalf("want 3 prompts, got %d", len(prompts))
		}
		if prompts[0].Title != "third" {
			t.Errorf("want first result 'third', got %q", prompts[0].Title)
		}
		if prompts[2].Title != "first" {
			t.Errorf("want last result 'first', got %q", prompts[2].Title)
		}
	})
}

func TestSearchPrompts(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	_, err := s.InsertPrompt(ctx, Prompt{Title: "Translate English to French", Body: "You are a translator.", ModelSlug: ""})
	if err != nil {
		t.Fatalf("InsertPrompt: %v", err)
	}
	_, err = s.InsertPrompt(ctx, Prompt{Title: "Code reviewer", Body: "Review the Python code carefully.", ModelSlug: "gpt-4"})
	if err != nil {
		t.Fatalf("InsertPrompt: %v", err)
	}
	_, err = s.InsertPrompt(ctx, Prompt{Title: "Summarizer", Body: "Summarize the following text.", ModelSlug: ""})
	if err != nil {
		t.Fatalf("InsertPrompt: %v", err)
	}

	cases := []struct {
		name      string
		query     string
		wantCount int
		wantTitle string // first result title if wantCount > 0
	}{
		{
			name:      "matches by title",
			query:     "translate",
			wantCount: 1,
			wantTitle: "Translate English to French",
		},
		{
			name:      "matches by body",
			query:     "python",
			wantCount: 1,
			wantTitle: "Code reviewer",
		},
		{
			name:      "empty on no match",
			query:     "xyzzy",
			wantCount: 0,
		},
		{
			name:      "case-insensitive match",
			query:     "SUMMARIZE",
			wantCount: 1,
			wantTitle: "Summarizer",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results, err := s.SearchPrompts(ctx, tc.query)
			if err != nil {
				t.Fatalf("SearchPrompts(%q): %v", tc.query, err)
			}
			if len(results) != tc.wantCount {
				t.Errorf("want %d results, got %d", tc.wantCount, len(results))
			}
			if tc.wantCount > 0 && len(results) > 0 && results[0].Title != tc.wantTitle {
				t.Errorf("want first title %q, got %q", tc.wantTitle, results[0].Title)
			}
		})
	}
}

func TestSavePromptResponse(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	id, err := s.InsertPrompt(ctx, Prompt{Title: "Runner", Body: "Run this", ModelSlug: "gpt-4"})
	if err != nil {
		t.Fatalf("InsertPrompt: %v", err)
	}

	t.Run("saves response for existing id", func(t *testing.T) {
		if err := s.SavePromptResponse(ctx, id, "hello world"); err != nil {
			t.Fatalf("SavePromptResponse: %v", err)
		}
		got, err := s.GetPrompt(ctx, id)
		if err != nil {
			t.Fatalf("GetPrompt: %v", err)
		}
		if got.LastResponse != "hello world" {
			t.Errorf("LastResponse: want %q, got %q", "hello world", got.LastResponse)
		}
	})

	t.Run("overwrites previous response", func(t *testing.T) {
		if err := s.SavePromptResponse(ctx, id, "updated response"); err != nil {
			t.Fatalf("SavePromptResponse: %v", err)
		}
		got, err := s.GetPrompt(ctx, id)
		if err != nil {
			t.Fatalf("GetPrompt: %v", err)
		}
		if got.LastResponse != "updated response" {
			t.Errorf("LastResponse: want %q, got %q", "updated response", got.LastResponse)
		}
	})

	t.Run("returns error on missing id", func(t *testing.T) {
		err := s.SavePromptResponse(ctx, 99999, "ghost")
		if err == nil {
			t.Fatal("want error for unknown id, got nil")
		}
	})
}

func TestPromptCWDRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	const wantCWD = "/home/user/projects/myapp"
	id, err := s.InsertPrompt(ctx, Prompt{Title: "CWD Test", Body: "test body", ModelSlug: "gpt-4", CWD: wantCWD})
	if err != nil {
		t.Fatalf("InsertPrompt: %v", err)
	}

	got, err := s.GetPrompt(ctx, id)
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if got.CWD != wantCWD {
		t.Errorf("CWD: want %q, got %q", wantCWD, got.CWD)
	}
}

func TestGetPromptByTitle(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	_, err := s.InsertPrompt(ctx, Prompt{Title: "My Summarizer", Body: "Summarize the text.", ModelSlug: "gpt-4"})
	if err != nil {
		t.Fatalf("InsertPrompt: %v", err)
	}

	t.Run("found by exact title", func(t *testing.T) {
		got, err := s.GetPromptByTitle(ctx, "My Summarizer")
		if err != nil {
			t.Fatalf("GetPromptByTitle: %v", err)
		}
		if got.Title != "My Summarizer" {
			t.Errorf("Title: want %q, got %q", "My Summarizer", got.Title)
		}
		if got.Body != "Summarize the text." {
			t.Errorf("Body: want %q, got %q", "Summarize the text.", got.Body)
		}
	})

	t.Run("found case-insensitively", func(t *testing.T) {
		got, err := s.GetPromptByTitle(ctx, "MY SUMMARIZER")
		if err != nil {
			t.Fatalf("GetPromptByTitle case-insensitive: %v", err)
		}
		if got.Title != "My Summarizer" {
			t.Errorf("Title: want %q, got %q", "My Summarizer", got.Title)
		}
	})

	t.Run("error on missing title", func(t *testing.T) {
		_, err := s.GetPromptByTitle(ctx, "nonexistent title")
		if err != sql.ErrNoRows {
			t.Errorf("want sql.ErrNoRows, got %v", err)
		}
	})
}

func TestGetPrompt(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	inserted := Prompt{Title: "Fetch me", Body: "Full body text", ModelSlug: "claude-3"}
	id, err := s.InsertPrompt(ctx, inserted)
	if err != nil {
		t.Fatalf("InsertPrompt: %v", err)
	}

	cases := []struct {
		name    string
		id      int64
		wantErr error
	}{
		{
			name: "returns full struct",
			id:   id,
		},
		{
			name:    "error on missing id",
			id:      99999,
			wantErr: sql.ErrNoRows,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.GetPrompt(ctx, tc.id)
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("GetPrompt() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetPrompt() unexpected error: %v", err)
			}
			if got.ID != id {
				t.Errorf("ID: want %d, got %d", id, got.ID)
			}
			if got.Title != inserted.Title {
				t.Errorf("Title: want %q, got %q", inserted.Title, got.Title)
			}
			if got.Body != inserted.Body {
				t.Errorf("Body: want %q, got %q", inserted.Body, got.Body)
			}
			if got.ModelSlug != inserted.ModelSlug {
				t.Errorf("ModelSlug: want %q, got %q", inserted.ModelSlug, got.ModelSlug)
			}
		})
	}
}
