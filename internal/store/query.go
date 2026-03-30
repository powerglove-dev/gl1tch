package store

import (
	"database/sql"
	"encoding/json"
)

// QueryRuns returns up to limit runs ordered by started_at descending.
// Reads use s.db directly since WAL mode allows concurrent readers.
func (s *Store) QueryRuns(limit int) ([]Run, error) {
	rows, err := s.db.Query(
		`SELECT id, kind, name, started_at, finished_at, exit_status, stdout, stderr, metadata, steps
		   FROM runs
		  ORDER BY started_at DESC
		  LIMIT ?`,
		limit,
	)
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
			&r.ID, &r.Kind, &r.Name, &r.StartedAt,
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
		`SELECT id, kind, name, started_at, finished_at, exit_status, stdout, stderr, metadata, steps
		   FROM runs WHERE id = ?`,
		id,
	)
	var r Run
	var finishedAt sql.NullInt64
	var exitStatus sql.NullInt64
	var stdout, stderr, metadata, steps sql.NullString
	if err := row.Scan(
		&r.ID, &r.Kind, &r.Name, &r.StartedAt,
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
