package capability

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/store"
)

// PipelineRunsCapability ports PipelineIndexer from
// internal/collector/pipeline.go. It tails the local store for completed
// pipeline runs and emits one document per new run into the glitch-pipelines
// index. Cursor (lastRunID) lives on the struct so successive ticks advance.
//
// The store is opened lazily on first invocation if not already supplied.
// On open failure the capability degrades to a no-op rather than failing the
// runner — matches the original "store unavailable, just don't index" stance
// for environments where SQLite isn't writable.
//
// Named "pipeline" so the brain popover keeps its existing key.
type PipelineRunsCapability struct {
	Store       *store.Store
	Interval    time.Duration
	WorkspaceID string

	mu        sync.Mutex
	lastRunID int64
	seeded    bool
	storeOK   bool
}

func (p *PipelineRunsCapability) Manifest() Manifest {
	every := p.Interval
	if every == 0 {
		every = 30 * time.Second
	}
	return Manifest{
		Name:        "pipeline",
		Description: "Indexes completed pipeline runs from the local store into glitch-pipelines. One document per run with status, exit code, duration, truncated stdout/stderr, and timestamps.",
		Category:    "runtime.pipeline",
		Trigger:     Trigger{Mode: TriggerInterval, Every: every},
		Sink:        Sink{Index: true},
		Invocation:  Invocation{Index: "glitch-pipelines"},
	}
}

func (p *PipelineRunsCapability) Invoke(ctx context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event, 64)
	go func() {
		defer close(ch)
		p.poll(ctx, ch)
	}()
	return ch, nil
}

func (p *PipelineRunsCapability) poll(_ context.Context, ch chan<- Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Store == nil && !p.storeOK {
		s, err := store.Open()
		if err != nil {
			slog.Info("pipeline capability: store unavailable", "err", err)
			return
		}
		p.Store = s
		p.storeOK = true
	}
	if p.Store == nil {
		return
	}

	// Seed cursor on first call so we don't backfill every historical run.
	if !p.seeded {
		runs, err := p.Store.QueryRuns(1)
		if err == nil && len(runs) > 0 {
			p.lastRunID = runs[0].ID
		}
		p.seeded = true
		return
	}

	// Workspace-aware query: empty WorkspaceID falls through to the
	// global path inside the store, matching the original collector.
	runs, err := p.Store.QueryRunsForWorkspace(p.WorkspaceID, 50)
	if err != nil {
		ch <- Event{Kind: EventError, Err: err}
		return
	}

	var maxID int64
	for _, run := range runs {
		if run.ID <= p.lastRunID {
			continue
		}
		if run.ID > maxID {
			maxID = run.ID
		}

		status := "success"
		exitCode := 0
		if run.ExitStatus != nil {
			exitCode = *run.ExitStatus
			if exitCode != 0 {
				status = "failure"
			}
		}

		ts := time.UnixMilli(run.StartedAt)

		var durationMs int64
		if run.FinishedAt != nil {
			durationMs = *run.FinishedAt - run.StartedAt
		}

		ch <- Event{Kind: EventDoc, Doc: map[string]any{
			"name":         run.Name,
			"status":       status,
			"workspace_id": p.WorkspaceID,
			"exit_code":    exitCode,
			"stdout":       truncateString(run.Stdout, 5000),
			"stderr":       truncateString(run.Stderr, 2000),
			"duration_ms":  durationMs,
			"timestamp":    ts,
		}}
	}

	if maxID > p.lastRunID {
		p.lastRunID = maxID
	}
}
