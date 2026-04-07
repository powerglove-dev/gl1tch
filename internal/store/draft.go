package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// DraftKind enumerates the entity types a draft can target. Stored as
// the literal string in the drafts.kind column.
const (
	DraftKindPrompt     = "prompt"
	DraftKindWorkflow   = "workflow"
	DraftKindSkill      = "skill"
	DraftKindAgent      = "agent"
	DraftKindCollectors = "collectors"
)

// DraftTurn is one entry in a draft's refinement history. The user
// types, gl1tch responds with a new body, and we record the pair plus
// which model produced the new body so the popup can show provenance.
type DraftTurn struct {
	Role      string `json:"role"`              // "user" | "assistant"
	Text      string `json:"text"`              // user instruction or assistant rationale
	Body      string `json:"body,omitempty"`    // resulting draft body (assistant turns only)
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// Draft is a single in-progress prompt/workflow/skill/agent the user is
// iterating on. The body is the latest version; turns is the full
// refinement history.
//
// InputFormat / OutputFormat are optional shape hints carried alongside
// the body. They only apply to kind=prompt today (the editor popup
// hides them for other kinds), but living on the draft means a brand
// new prompt can capture format intent before it's ever promoted into
// the prompts table. Empty string = "free-form text".
type Draft struct {
	ID           int64
	WorkspaceID  string
	Kind         string
	Title        string
	Body         string
	InputFormat  string
	OutputFormat string
	Turns        []DraftTurn
	TargetID     int64  // 0 when unset
	TargetPath   string // "" when unset
	CreatedAt    int64
	UpdatedAt    int64
}

// CreateDraft inserts a new draft and returns the generated ID.
// Turns is serialized to JSON; created_at and updated_at are set to now.
func (s *Store) CreateDraft(_ context.Context, d Draft) (int64, error) {
	now := time.Now().Unix()
	turnsJSON, err := marshalTurns(d.Turns)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.writer.send(func(db *sql.DB) error {
		res, err := db.Exec(
			`INSERT INTO drafts (workspace_id, kind, title, body, input_format, output_format, turns_json, target_id, target_path, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.WorkspaceID, d.Kind, d.Title, d.Body, d.InputFormat, d.OutputFormat, turnsJSON,
			nullableInt(d.TargetID), d.TargetPath, now, now,
		)
		if err != nil {
			return fmt.Errorf("store: insert draft: %w", err)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: insert draft last id: %w", err)
		}
		return nil
	})
	return id, err
}

// UpdateDraft replaces the title, body, target_id, and target_path of
// an existing draft. Turns are NOT touched here — use AppendDraftTurn
// for refinement history. Updates updated_at.
func (s *Store) UpdateDraft(_ context.Context, d Draft) error {
	now := time.Now().Unix()
	return s.writer.send(func(db *sql.DB) error {
		res, err := db.Exec(
			`UPDATE drafts
			    SET title = ?, body = ?, input_format = ?, output_format = ?,
			        target_id = ?, target_path = ?, updated_at = ?
			  WHERE id = ?`,
			d.Title, d.Body, d.InputFormat, d.OutputFormat,
			nullableInt(d.TargetID), d.TargetPath, now, d.ID,
		)
		if err != nil {
			return fmt.Errorf("store: update draft: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: update draft rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("store: update draft: no draft with id %d", d.ID)
		}
		return nil
	})
}

// AppendDraftTurn appends a single turn to the draft's history and
// updates body + updated_at in the same write. This is the hot path
// the refinement loop hits on every chat message; we keep it as one
// SELECT-then-UPDATE on the writer goroutine to avoid races between
// concurrent refines on the same draft.
func (s *Store) AppendDraftTurn(_ context.Context, draftID int64, turn DraftTurn, newBody string) error {
	now := time.Now().Unix()
	return s.writer.send(func(db *sql.DB) error {
		var raw string
		err := db.QueryRow(`SELECT turns_json FROM drafts WHERE id = ?`, draftID).Scan(&raw)
		if err == sql.ErrNoRows {
			return fmt.Errorf("store: append draft turn: no draft with id %d", draftID)
		}
		if err != nil {
			return fmt.Errorf("store: append draft turn lookup: %w", err)
		}
		var turns []DraftTurn
		if raw != "" {
			if jerr := json.Unmarshal([]byte(raw), &turns); jerr != nil {
				// Corrupt history shouldn't block the user — start fresh.
				turns = nil
			}
		}
		turns = append(turns, turn)
		next, jerr := json.Marshal(turns)
		if jerr != nil {
			return fmt.Errorf("store: append draft turn marshal: %w", jerr)
		}
		_, err = db.Exec(
			`UPDATE drafts SET turns_json = ?, body = ?, updated_at = ? WHERE id = ?`,
			string(next), newBody, now, draftID,
		)
		if err != nil {
			return fmt.Errorf("store: append draft turn update: %w", err)
		}
		return nil
	})
}

// GetDraft returns a draft by ID. Returns sql.ErrNoRows if not found.
func (s *Store) GetDraft(ctx context.Context, id int64) (Draft, error) {
	var d Draft
	var targetID sql.NullInt64
	var turnsRaw string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, kind, title, body, input_format, output_format, turns_json, target_id, target_path, created_at, updated_at
		   FROM drafts
		  WHERE id = ?`,
		id,
	).Scan(&d.ID, &d.WorkspaceID, &d.Kind, &d.Title, &d.Body, &d.InputFormat, &d.OutputFormat, &turnsRaw, &targetID, &d.TargetPath, &d.CreatedAt, &d.UpdatedAt)
	if err == sql.ErrNoRows {
		return Draft{}, sql.ErrNoRows
	}
	if err != nil {
		return Draft{}, fmt.Errorf("store: get draft: %w", err)
	}
	if targetID.Valid {
		d.TargetID = targetID.Int64
	}
	d.Turns = unmarshalTurns(turnsRaw)
	return d, nil
}

// ListDraftsByWorkspace returns all drafts for the given workspace,
// ordered by updated_at DESC. If kind is non-empty, results are filtered
// to that kind. Returns a non-nil empty slice when there are no rows.
func (s *Store) ListDraftsByWorkspace(ctx context.Context, workspaceID, kind string) ([]Draft, error) {
	query := `SELECT id, workspace_id, kind, title, body, input_format, output_format, turns_json, target_id, target_path, created_at, updated_at
	            FROM drafts
	           WHERE workspace_id = ?`
	args := []any{workspaceID}
	if kind != "" {
		query += ` AND kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY updated_at DESC, id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list drafts: %w", err)
	}
	defer rows.Close()

	drafts := []Draft{}
	for rows.Next() {
		var d Draft
		var targetID sql.NullInt64
		var turnsRaw string
		if err := rows.Scan(&d.ID, &d.WorkspaceID, &d.Kind, &d.Title, &d.Body, &d.InputFormat, &d.OutputFormat, &turnsRaw, &targetID, &d.TargetPath, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: list drafts scan: %w", err)
		}
		if targetID.Valid {
			d.TargetID = targetID.Int64
		}
		d.Turns = unmarshalTurns(turnsRaw)
		drafts = append(drafts, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list drafts rows: %w", err)
	}
	return drafts, nil
}

// DeleteDraft removes a draft by ID. Returns an error if no row was affected.
func (s *Store) DeleteDraft(_ context.Context, id int64) error {
	return s.writer.send(func(db *sql.DB) error {
		res, err := db.Exec(`DELETE FROM drafts WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("store: delete draft: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: delete draft rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("store: delete draft: no draft with id %d", id)
		}
		return nil
	})
}

// marshalTurns serializes a turn slice to JSON. nil and empty both
// become "[]" so the column is never NULL.
func marshalTurns(turns []DraftTurn) (string, error) {
	if len(turns) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(turns)
	if err != nil {
		return "", fmt.Errorf("store: marshal turns: %w", err)
	}
	return string(b), nil
}

// unmarshalTurns is the read-side counterpart. A corrupt or empty
// payload returns an empty slice rather than failing the read — the
// draft body is still recoverable, only the history is lost.
func unmarshalTurns(raw string) []DraftTurn {
	if raw == "" || raw == "[]" {
		return []DraftTurn{}
	}
	var turns []DraftTurn
	if err := json.Unmarshal([]byte(raw), &turns); err != nil {
		return []DraftTurn{}
	}
	if turns == nil {
		return []DraftTurn{}
	}
	return turns
}

// nullableInt converts a zero int64 to a NULL sql value so target_id is
// stored as NULL when no target is attached, instead of 0 (which would
// collide with a hypothetical row id of 0).
func nullableInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
