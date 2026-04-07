package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// QueryRuns returns up to limit runs ordered by started_at descending.
// Reads use s.db directly since WAL mode allows concurrent readers.
//
// Equivalent to QueryRunsForWorkspace with workspaceID="" (no filter).
func (s *Store) QueryRuns(limit int) ([]Run, error) {
	return s.QueryRunsForWorkspace("", limit)
}

// QueryRunsForWorkspace is the workspace-aware variant of QueryRuns.
// When workspaceID is non-empty, only runs whose workspace_id column
// matches are returned. The PipelineIndexer uses this to scope its
// indexing to one workspace's runs so workspace A's pod doesn't
// re-index workspace B's pipeline runs.
//
// Empty workspaceID returns the unfiltered list — preserves the
// legacy behavior for callers that don't have a workspace context.
func (s *Store) QueryRunsForWorkspace(workspaceID string, limit int) ([]Run, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if workspaceID == "" {
		rows, err = s.db.Query(
			`SELECT id, kind, name, workspace_id, started_at, finished_at, exit_status, stdout, stderr, metadata, steps
			   FROM runs
			  ORDER BY started_at DESC
			  LIMIT ?`,
			limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, kind, name, workspace_id, started_at, finished_at, exit_status, stdout, stderr, metadata, steps
			   FROM runs
			  WHERE workspace_id = ?
			  ORDER BY started_at DESC
			  LIMIT ?`,
			workspaceID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var r Run
		var finishedAt sql.NullInt64
		var exitStatus sql.NullInt64
		var stdout, stderr, metadata, steps sql.NullString

		if err := rows.Scan(
			&r.ID, &r.Kind, &r.Name, &r.WorkspaceID, &r.StartedAt,
			&finishedAt, &exitStatus,
			&stdout, &stderr, &metadata, &steps,
		); err != nil {
			return nil, err
		}

		if finishedAt.Valid {
			r.FinishedAt = &finishedAt.Int64
		}
		if exitStatus.Valid {
			v := int(exitStatus.Int64)
			r.ExitStatus = &v
		}
		r.Stdout = stdout.String
		r.Stderr = stderr.String
		r.Metadata = metadata.String

		// Parse steps JSON; silently return empty slice on failure.
		if steps.Valid && steps.String != "" {
			var sr []StepRecord
			if err := json.Unmarshal([]byte(steps.String), &sr); err == nil {
				r.Steps = sr
			}
		}
		if r.Steps == nil {
			r.Steps = []StepRecord{}
		}

		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// GetRun returns the run with the given id, or an error if not found.
func (s *Store) GetRun(id int64) (*Run, error) {
	row := s.db.QueryRow(
		`SELECT id, kind, name, workspace_id, started_at, finished_at, exit_status, stdout, stderr, metadata, steps
		   FROM runs WHERE id = ?`,
		id,
	)
	var r Run
	var finishedAt sql.NullInt64
	var exitStatus sql.NullInt64
	var stdout, stderr, metadata, steps sql.NullString
	if err := row.Scan(
		&r.ID, &r.Kind, &r.Name, &r.WorkspaceID, &r.StartedAt,
		&finishedAt, &exitStatus,
		&stdout, &stderr, &metadata, &steps,
	); err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		r.FinishedAt = &finishedAt.Int64
	}
	if exitStatus.Valid {
		v := int(exitStatus.Int64)
		r.ExitStatus = &v
	}
	r.Stdout = stdout.String
	r.Stderr = stderr.String
	r.Metadata = metadata.String
	if steps.Valid && steps.String != "" {
		var sr []StepRecord
		if err := json.Unmarshal([]byte(steps.String), &sr); err == nil {
			r.Steps = sr
		}
	}
	if r.Steps == nil {
		r.Steps = []StepRecord{}
	}
	return &r, nil
}

// AppendRunStdout appends additional text to the stdout column of the run.
func (s *Store) AppendRunStdout(id int64, additional string) error {
	return s.writer.send(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE runs SET stdout = stdout || ? WHERE id = ?`, additional, id)
		return err
	})
}

// DeleteRun removes the run with the given id.
func (s *Store) DeleteRun(id int64) error {
	return s.writer.send(func(db *sql.DB) error {
		_, err := db.Exec(`DELETE FROM runs WHERE id = ?`, id)
		return err
	})
}

// GameStatsQuery returns aggregate behavioral stats from score_events over the
// last sinceDays calendar days. Returns a zero-value GameStats (no error) when
// no events exist within the window.
func (s *Store) GameStatsQuery(ctx context.Context, sinceDays int) (GameStats, error) {
	cutoff := time.Now().AddDate(0, 0, -sinceDays).UnixMilli()

	var gs GameStats
	gs.ProviderRunCounts = make(map[string]int64)

	// ── aggregate stats ──────────────────────────────────────────────────────
	row := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(AVG(CASE WHEN (input_tokens + cache_creation_tokens) > 0
				THEN CAST(output_tokens AS REAL) / CAST(input_tokens + cache_creation_tokens AS REAL)
				ELSE 0 END), 0),
			COALESCE(AVG(cache_read_tokens), 0),
			COALESCE(AVG(cost_usd), 0),
			COALESCE(SUM(CASE WHEN xp = 0 THEN 1 ELSE 0 END), 0)
		FROM score_events
		WHERE created_at >= ?`, cutoff)

	var totalRuns, zeroXPRuns int64
	if err := row.Scan(&totalRuns, &gs.AvgOutputRatio, &gs.AvgCacheReadTokens, &gs.AvgCostUSD, &zeroXPRuns); err != nil {
		return GameStats{}, nil //nolint:nilerr — return zero on empty
	}
	gs.TotalRuns = totalRuns
	gs.RunsSinceDate = totalRuns
	if totalRuns > 0 {
		gs.StepFailureRate = float64(zeroXPRuns) / float64(totalRuns)
	}

	if totalRuns == 0 {
		gs.UnlockedAchievementIDs = []string{}
		return gs, nil
	}

	// ── p50 output ratio ──────────────────────────────────────────────────────
	p50offset := (totalRuns - 1) / 2
	rowP50 := s.db.QueryRowContext(ctx, `
		SELECT CAST(output_tokens AS REAL) / CAST(input_tokens + cache_creation_tokens + 0.001 AS REAL)
		FROM score_events
		WHERE created_at >= ?
		ORDER BY CAST(output_tokens AS REAL) / CAST(input_tokens + cache_creation_tokens + 0.001 AS REAL)
		LIMIT 1 OFFSET ?`, cutoff, p50offset)
	_ = rowP50.Scan(&gs.P50OutputRatio)

	// ── p90 output ratio ──────────────────────────────────────────────────────
	p90offset := int64(float64(totalRuns-1) * 0.9)
	rowP90 := s.db.QueryRowContext(ctx, `
		SELECT CAST(output_tokens AS REAL) / CAST(input_tokens + cache_creation_tokens + 0.001 AS REAL)
		FROM score_events
		WHERE created_at >= ?
		ORDER BY CAST(output_tokens AS REAL) / CAST(input_tokens + cache_creation_tokens + 0.001 AS REAL)
		LIMIT 1 OFFSET ?`, cutoff, p90offset)
	_ = rowP90.Scan(&gs.P90OutputRatio)

	// ── p90 cache read tokens ─────────────────────────────────────────────────
	rowP90Cache := s.db.QueryRowContext(ctx, `
		SELECT cache_read_tokens
		FROM score_events
		WHERE created_at >= ?
		ORDER BY cache_read_tokens
		LIMIT 1 OFFSET ?`, cutoff, p90offset)
	_ = rowP90Cache.Scan(&gs.P90CacheReadTokens)

	// ── provider run counts ───────────────────────────────────────────────────
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider, COUNT(*) FROM score_events
		WHERE created_at >= ?
		GROUP BY provider`, cutoff)
	if err != nil {
		return GameStats{}, nil //nolint:nilerr
	}
	defer rows.Close()
	for rows.Next() {
		var provider string
		var count int64
		if err := rows.Scan(&provider, &count); err != nil {
			continue
		}
		gs.ProviderRunCounts[provider] = count
	}
	if err := rows.Err(); err != nil {
		return GameStats{}, nil //nolint:nilerr
	}

	// ── unlocked achievements ─────────────────────────────────────────────────
	achRows, err := s.db.QueryContext(ctx,
		`SELECT achievement_id FROM achievements ORDER BY unlocked_at ASC`)
	if err != nil {
		gs.UnlockedAchievementIDs = []string{}
		return gs, nil //nolint:nilerr
	}
	defer achRows.Close()
	gs.UnlockedAchievementIDs = []string{}
	for achRows.Next() {
		var id string
		if err := achRows.Scan(&id); err != nil {
			continue
		}
		gs.UnlockedAchievementIDs = append(gs.UnlockedAchievementIDs, id)
	}
	_ = achRows.Err()

	return gs, nil
}
