package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Prompt is a saved prompt stored in the prompts table.
//
// InputFormat / OutputFormat are optional hints about the shape this
// prompt expects to receive and the shape it produces. Empty string =
// "free-form text" — the default until the user opts in. The builder
// uses these to lint plane wiring without forcing every prompt author
// to commit to a schema upfront.
type Prompt struct {
	ID           int64
	Title        string
	Body         string
	ModelSlug    string
	LastResponse string
	CWD          string
	InputFormat  string
	OutputFormat string
	CreatedAt    int64 // Unix timestamp
	UpdatedAt    int64 // Unix timestamp
}

// InsertPrompt inserts a new prompt and returns the generated ID.
// created_at and updated_at are set to the current Unix timestamp.
func (s *Store) InsertPrompt(_ context.Context, p Prompt) (int64, error) {
	now := time.Now().Unix()
	var id int64
	err := s.writer.send(func(db *sql.DB) error {
		res, err := db.Exec(
			`INSERT INTO prompts (title, body, model_slug, cwd, input_format, output_format, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			p.Title, p.Body, p.ModelSlug, p.CWD, p.InputFormat, p.OutputFormat, now, now,
		)
		if err != nil {
			return fmt.Errorf("store: insert prompt: %w", err)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: insert prompt last id: %w", err)
		}
		return nil
	})
	return id, err
}

// UpdatePrompt updates the title, body, model_slug, and updated_at of an
// existing prompt by ID. Returns an error if no row was affected.
func (s *Store) UpdatePrompt(_ context.Context, p Prompt) error {
	now := time.Now().Unix()
	return s.writer.send(func(db *sql.DB) error {
		res, err := db.Exec(
			`UPDATE prompts
			    SET title = ?, body = ?, model_slug = ?, cwd = ?,
			        input_format = ?, output_format = ?, updated_at = ?
			  WHERE id = ?`,
			p.Title, p.Body, p.ModelSlug, p.CWD, p.InputFormat, p.OutputFormat, now, p.ID,
		)
		if err != nil {
			return fmt.Errorf("store: update prompt: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: update prompt rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("store: update prompt: no prompt with id %d", p.ID)
		}
		return nil
	})
}

// DeletePrompt removes a prompt by ID. Returns an error if no row was affected.
func (s *Store) DeletePrompt(_ context.Context, id int64) error {
	return s.writer.send(func(db *sql.DB) error {
		res, err := db.Exec(`DELETE FROM prompts WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("store: delete prompt: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: delete prompt rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("store: delete prompt: no prompt with id %d", id)
		}
		return nil
	})
}

// ListPrompts returns all prompts ordered by updated_at DESC.
// Returns a non-nil empty slice when there are no prompts.
func (s *Store) ListPrompts(ctx context.Context) ([]Prompt, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, body, model_slug, last_response, cwd, input_format, output_format, created_at, updated_at
		   FROM prompts
		  ORDER BY updated_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list prompts: %w", err)
	}
	defer rows.Close()

	prompts := []Prompt{}
	for rows.Next() {
		var p Prompt
		if err := rows.Scan(&p.ID, &p.Title, &p.Body, &p.ModelSlug, &p.LastResponse, &p.CWD, &p.InputFormat, &p.OutputFormat, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: list prompts scan: %w", err)
		}
		prompts = append(prompts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list prompts rows: %w", err)
	}
	return prompts, nil
}

// SearchPrompts returns prompts where title OR body contains query
// (case-insensitive LIKE), ordered by updated_at DESC.
func (s *Store) SearchPrompts(ctx context.Context, query string) ([]Prompt, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, body, model_slug, last_response, cwd, input_format, output_format, created_at, updated_at
		   FROM prompts
		  WHERE title LIKE ? OR body LIKE ?
		  ORDER BY updated_at DESC, id DESC`,
		pattern, pattern,
	)
	if err != nil {
		return nil, fmt.Errorf("store: search prompts: %w", err)
	}
	defer rows.Close()

	prompts := []Prompt{}
	for rows.Next() {
		var p Prompt
		if err := rows.Scan(&p.ID, &p.Title, &p.Body, &p.ModelSlug, &p.LastResponse, &p.CWD, &p.InputFormat, &p.OutputFormat, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: search prompts scan: %w", err)
		}
		prompts = append(prompts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: search prompts rows: %w", err)
	}
	return prompts, nil
}

// SavePromptResponse updates the last_response and updated_at fields for the
// prompt with the given ID. Returns an error if no row was affected.
func (s *Store) SavePromptResponse(_ context.Context, id int64, response string) error {
	now := time.Now().Unix()
	return s.writer.send(func(db *sql.DB) error {
		res, err := db.Exec(
			`UPDATE prompts SET last_response = ?, updated_at = ? WHERE id = ?`,
			response, now, id,
		)
		if err != nil {
			return fmt.Errorf("store: save prompt response: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: save prompt response rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("store: save prompt response: no prompt with id %d", id)
		}
		return nil
	})
}

// GetPromptByTitle returns a prompt by title (case-insensitive exact match).
// Returns sql.ErrNoRows if not found.
func (s *Store) GetPromptByTitle(ctx context.Context, title string) (Prompt, error) {
	var p Prompt
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, body, model_slug, last_response, cwd, input_format, output_format, created_at, updated_at
		   FROM prompts
		  WHERE lower(title) = lower(?) LIMIT 1`,
		title,
	).Scan(&p.ID, &p.Title, &p.Body, &p.ModelSlug, &p.LastResponse, &p.CWD, &p.InputFormat, &p.OutputFormat, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return Prompt{}, sql.ErrNoRows
	}
	if err != nil {
		return Prompt{}, fmt.Errorf("store: get prompt by title: %w", err)
	}
	return p, nil
}

// GetPrompt returns a prompt by ID. Returns sql.ErrNoRows if not found.
func (s *Store) GetPrompt(ctx context.Context, id int64) (Prompt, error) {
	var p Prompt
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, body, model_slug, last_response, cwd, input_format, output_format, created_at, updated_at
		   FROM prompts
		  WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.Title, &p.Body, &p.ModelSlug, &p.LastResponse, &p.CWD, &p.InputFormat, &p.OutputFormat, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return Prompt{}, sql.ErrNoRows
	}
	if err != nil {
		return Prompt{}, fmt.Errorf("store: get prompt: %w", err)
	}
	return p, nil
}
