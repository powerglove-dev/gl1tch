package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/store"
)

// PipelineIndexer watches the store for completed runs and indexes them to ES.
type PipelineIndexer struct {
	Store    *store.Store
	Interval time.Duration
}

func (p *PipelineIndexer) Name() string { return "pipeline" }

func (p *PipelineIndexer) Start(ctx context.Context, es *esearch.Client) error {
	if p.Interval == 0 {
		p.Interval = 30 * time.Second
	}

	// Open our own store connection if none was provided.
	if p.Store == nil {
		s, err := store.Open()
		if err != nil {
			slog.Info("pipeline indexer: store unavailable", "err", err)
			return nil
		}
		p.Store = s
		defer s.Close()
	}

	var lastRunID int64

	// Seed to latest run so we don't backfill everything.
	runs, err := p.Store.QueryRuns(1)
	if err == nil && len(runs) > 0 {
		lastRunID = runs[0].ID
		slog.Info("pipeline indexer: seeded to run", "id", lastRunID)
	}

	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			start := time.Now()
			newLast, err := p.poll(ctx, es, lastRunID)
			indexed := 0
			if newLast > lastRunID {
				indexed = int(newLast - lastRunID)
			}
			RecordRun("pipeline", start, indexed, err)
			if err != nil {
				slog.Warn("pipeline indexer: poll error", "err", err)
				continue
			}
			lastRunID = newLast
		}
	}
}

func (p *PipelineIndexer) poll(ctx context.Context, es *esearch.Client, afterID int64) (int64, error) {
	// Query recent runs and filter for those after our cursor.
	runs, err := p.Store.QueryRuns(50)
	if err != nil {
		return afterID, err
	}

	var docs []any
	var maxID int64

	for _, run := range runs {
		if run.ID <= afterID {
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

		doc := esearch.PipelineRun{
			Name:       run.Name,
			Status:     status,
			ExitCode:   exitCode,
			Stdout:     truncate(run.Stdout, 5000),
			Stderr:     truncate(run.Stderr, 2000),
			DurationMs: durationMs,
			Timestamp:  ts,
		}

		docs = append(docs, doc)
	}

	if len(docs) > 0 {
		slog.Info("pipeline indexer: new runs", "count", len(docs))
		if err := es.BulkIndex(ctx, esearch.IndexPipelines, docs); err != nil {
			return afterID, err
		}
	}

	if maxID == 0 {
		return afterID, nil
	}
	return maxID, nil
}
