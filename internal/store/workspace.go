package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
)

// Workspace represents a chat workspace with its own set of monitored directories.
//
// Directories / RepoNames contain the *enabled* directory paths in
// canonical order: the primary directory is always element 0, followed
// by the additional reference directories. PrimaryDirectory holds the
// same value as Directories[0] (or "" when the workspace has no
// directories yet) so call sites that just want the cwd don't have to
// check len(Directories).
//
// Code that wants the full set (including disabled rows for the UI
// toggle list) calls ListWorkspaceDirectories — that returns one
// WorkspaceDirectory per row with both the Enabled and Primary flags.
type Workspace struct {
	ID                    string   `json:"id"`
	Title                 string   `json:"title"`
	Directories           []string `json:"directories"`            // primary first, then additionals (enabled only)
	RepoNames             []string `json:"repo_names"`             // parallel to Directories
	PrimaryDirectory      string   `json:"primary_directory"`      // == Directories[0] when set
	AdditionalDirectories []string `json:"additional_directories"` // == Directories[1:]
	CreatedAt             int64    `json:"created_at"`
	UpdatedAt             int64    `json:"updated_at"`
}

// WorkspaceDirectory is one row from workspace_directories with its
// enable + primary flags. Used by the desktop UI to render per-
// directory toggles + a "primary" badge / right-click action alongside
// the directory list.
type WorkspaceDirectory struct {
	Path     string `json:"path"`
	RepoName string `json:"repo_name"`
	Enabled  bool   `json:"enabled"`
	Primary  bool   `json:"primary"`
}

// WorkspaceMessage is a persisted chat message within a workspace.
type WorkspaceMessage struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Role        string `json:"role"`
	BlocksJSON  string `json:"blocks_json"`
	Timestamp   int64  `json:"timestamp"`
}

// CreateWorkspace creates a new workspace and returns it.
func (s *Store) CreateWorkspace(ctx context.Context, title string, ts int64) (Workspace, error) {
	id := uuid.New().String()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspaces (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		id, title, ts, ts)
	if err != nil {
		return Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	return Workspace{ID: id, Title: title, CreatedAt: ts, UpdatedAt: ts}, nil
}

// ListWorkspaces returns all workspaces ordered by most recently updated.
func (s *Store) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, created_at, updated_at FROM workspaces ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []Workspace
	for rows.Next() {
		var ws Workspace
		if err := rows.Scan(&ws.ID, &ws.Title, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
			return nil, err
		}
		// Load directories for this workspace
		ws.Directories, ws.RepoNames, _ = s.getWorkspaceDirectories(ctx, ws.ID)
		fillDerivedDirectoryFields(&ws)
		workspaces = append(workspaces, ws)
	}
	return workspaces, rows.Err()
}

// GetWorkspace returns a single workspace by ID.
func (s *Store) GetWorkspace(ctx context.Context, id string) (Workspace, error) {
	var ws Workspace
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, created_at, updated_at FROM workspaces WHERE id = ?`, id,
	).Scan(&ws.ID, &ws.Title, &ws.CreatedAt, &ws.UpdatedAt)
	if err != nil {
		return Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	ws.Directories, ws.RepoNames, _ = s.getWorkspaceDirectories(ctx, ws.ID)
	fillDerivedDirectoryFields(&ws)
	return ws, nil
}

// DeleteWorkspace removes a workspace and all its directories and messages.
func (s *Store) DeleteWorkspace(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM workspace_messages WHERE workspace_id = ?`, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM workspace_directories WHERE workspace_id = ?`, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, id)
	return err
}

// UpdateWorkspaceTitle updates the title and updated_at timestamp.
func (s *Store) UpdateWorkspaceTitle(ctx context.Context, id, title string, ts int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE workspaces SET title = ?, updated_at = ? WHERE id = ?`, title, ts, id)
	return err
}

// TouchWorkspace updates the updated_at timestamp.
func (s *Store) TouchWorkspace(ctx context.Context, id string, ts int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workspaces SET updated_at = ? WHERE id = ?`, ts, id)
	return err
}

// AddWorkspaceDirectory associates a directory with a workspace. The
// first directory added to a workspace is automatically flagged as the
// primary; subsequent rows land as additional reference directories.
// Callers can re-pick the primary later via SetWorkspacePrimaryDirectory.
func (s *Store) AddWorkspaceDirectory(ctx context.Context, workspaceID, path string) error {
	repoName := filepath.Base(path)
	// Determine whether this workspace already has a primary so we
	// know whether to stamp the new row as primary on insert.
	var existing int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workspace_directories WHERE workspace_id = ? AND primary_dir = 1`,
		workspaceID,
	).Scan(&existing); err != nil {
		return fmt.Errorf("count primary dirs: %w", err)
	}
	primary := 0
	if existing == 0 {
		primary = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO workspace_directories (workspace_id, path, repo_name, primary_dir) VALUES (?, ?, ?, ?)`,
		workspaceID, path, repoName, primary)
	return err
}

// RemoveWorkspaceDirectory removes a directory association from a
// workspace. If the removed row was the primary directory, the
// remaining-lowest-rowid row inherits the primary flag so the workspace
// always has either zero directories or exactly one primary.
func (s *Store) RemoveWorkspaceDirectory(ctx context.Context, workspaceID, path string) error {
	// Snapshot the row's primary flag before deletion so we know
	// whether we need to elect a new primary afterwards.
	var wasPrimary int
	_ = s.db.QueryRowContext(ctx,
		`SELECT primary_dir FROM workspace_directories WHERE workspace_id = ? AND path = ?`,
		workspaceID, path,
	).Scan(&wasPrimary)

	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM workspace_directories WHERE workspace_id = ? AND path = ?`,
		workspaceID, path); err != nil {
		return err
	}
	if wasPrimary == 0 {
		return nil
	}
	// Elect the lowest-rowid surviving directory as the new primary.
	// rowid order matches insert order which matches the user's
	// historical "first added" intuition.
	_, err := s.db.ExecContext(ctx, `
		UPDATE workspace_directories
		SET primary_dir = 1
		WHERE rowid = (
			SELECT MIN(rowid) FROM workspace_directories WHERE workspace_id = ?
		)
	`, workspaceID)
	return err
}

// SetWorkspacePrimaryDirectory promotes one of a workspace's
// directories to primary, demoting whatever was primary before. The
// path must already exist in workspace_directories — call
// AddWorkspaceDirectory first if necessary.
//
// Runs as a single transaction so the workspace is never momentarily
// without a primary directory between the demote and the promote.
func (s *Store) SetWorkspacePrimaryDirectory(ctx context.Context, workspaceID, path string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Ensure the target row exists before we touch the existing
	// primary, so a typo can't leave the workspace with no primary.
	var present int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workspace_directories WHERE workspace_id = ? AND path = ?`,
		workspaceID, path,
	).Scan(&present); err != nil {
		return err
	}
	if present == 0 {
		return fmt.Errorf("workspace %s has no directory at %s", workspaceID, path)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE workspace_directories SET primary_dir = 0 WHERE workspace_id = ?`,
		workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE workspace_directories SET primary_dir = 1 WHERE workspace_id = ? AND path = ?`,
		workspaceID, path); err != nil {
		return err
	}
	return tx.Commit()
}

// getWorkspaceDirectories returns only the *enabled* directories for a
// workspace, with the primary directory always first. Collectors that
// iterate Workspace.Directories see the primary repo on the first
// iteration so first-wins shortcuts (workspaceCWD, file scanning
// shortcuts, etc.) keep working without each call site sorting.
func (s *Store) getWorkspaceDirectories(ctx context.Context, workspaceID string) ([]string, []string, error) {
	// ORDER BY: primary rows first, then by path so the order is
	// deterministic across calls. SQLite sorts boolean ints as 0/1
	// so DESC puts primary_dir = 1 in front.
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, repo_name FROM workspace_directories
		 WHERE workspace_id = ? AND enabled = 1
		 ORDER BY primary_dir DESC, path ASC`, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var dirs, repos []string
	for rows.Next() {
		var path, repo string
		if err := rows.Scan(&path, &repo); err != nil {
			return nil, nil, err
		}
		dirs = append(dirs, path)
		repos = append(repos, repo)
	}
	return dirs, repos, rows.Err()
}

// fillDerivedDirectoryFields populates Workspace.PrimaryDirectory and
// Workspace.AdditionalDirectories from the canonically-ordered
// Directories slice. Call after every getWorkspaceDirectories so the
// caller-facing struct always has both views in sync.
func fillDerivedDirectoryFields(ws *Workspace) {
	if len(ws.Directories) == 0 {
		ws.PrimaryDirectory = ""
		ws.AdditionalDirectories = nil
		return
	}
	ws.PrimaryDirectory = ws.Directories[0]
	if len(ws.Directories) > 1 {
		ws.AdditionalDirectories = append([]string(nil), ws.Directories[1:]...)
	} else {
		ws.AdditionalDirectories = nil
	}
}

// ListWorkspaceDirectories returns every directory associated with a
// workspace, including disabled ones, with their enable + primary
// flags. The desktop UI uses this to render the per-directory toggle
// list and the "primary" badge in the workspace settings popover.
//
// Ordering: primary directory first, then by path so the row order is
// stable and the user's eye lands on the primary repo immediately.
func (s *Store) ListWorkspaceDirectories(ctx context.Context, workspaceID string) ([]WorkspaceDirectory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, repo_name, enabled, primary_dir
		   FROM workspace_directories
		  WHERE workspace_id = ?
		  ORDER BY primary_dir DESC, path ASC`,
		workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WorkspaceDirectory
	for rows.Next() {
		var d WorkspaceDirectory
		var enabled, primary int
		if err := rows.Scan(&d.Path, &d.RepoName, &enabled, &primary); err != nil {
			return nil, err
		}
		d.Enabled = enabled != 0
		d.Primary = primary != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// HasAnalyzedEvent reports whether the deep-analysis loop has already
// produced an analysis for the given event_key. Used as the fast
// pre-check before spending an opencode call on a duplicate.
//
// event_key is whatever the analyzer chose as a stable identifier
// (e.g. "github.pr:owner/repo:#42:updated_at" or "git.commit:<sha>")
// — the table is source-agnostic so any collector type can dedupe
// through the same path.
func (s *Store) HasAnalyzedEvent(ctx context.Context, eventKey string) (bool, error) {
	var ts int64
	err := s.db.QueryRowContext(ctx,
		`SELECT analyzed_at FROM analysis_dedupe WHERE event_key = ?`, eventKey,
	).Scan(&ts)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}

// MarkEventAnalyzed records that the deep-analysis loop has produced
// (or attempted to produce) an analysis for event_key. Subsequent
// HasAnalyzedEvent calls return true so the same event isn't
// re-analyzed on the next collector tick.
//
// source is stored alongside the key so future maintenance / cleanup
// can scope by collector — e.g. clearing all github analyses without
// touching git ones.
func (s *Store) MarkEventAnalyzed(ctx context.Context, eventKey, source string, analyzedAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO analysis_dedupe (event_key, source, analyzed_at) VALUES (?, ?, ?)`,
		eventKey, source, analyzedAt)
	return err
}

// ClearAnalysisDedupe wipes every row in the analysis_dedupe table.
// After this runs, the next collector tick will re-analyze every
// event the user has kept in Elasticsearch even if it was analyzed
// before — the intended use case is `glitch observe reset`, which
// clears events out of ES too so the re-analysis lands on a fresh
// slate.
//
// This function intentionally does NOT touch brain_notes or any
// other table: the "brain learning" the user has accumulated across
// pipeline runs is stored there and must be preserved.
//
// Returns the number of rows deleted so the caller can report it.
func (s *Store) ClearAnalysisDedupe(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM analysis_dedupe`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetWorkspaceDirectoryEnabled flips the enable flag on one directory.
// The collector pod must be restarted by the caller for the change to
// take effect — toggling here only updates the persisted state.
func (s *Store) SetWorkspaceDirectoryEnabled(ctx context.Context, workspaceID, path string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE workspace_directories SET enabled = ? WHERE workspace_id = ? AND path = ?`,
		v, workspaceID, path)
	return err
}

// SaveWorkspaceMessage persists a chat message.
func (s *Store) SaveWorkspaceMessage(ctx context.Context, msg WorkspaceMessage) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO workspace_messages (id, workspace_id, role, blocks_json, timestamp) VALUES (?, ?, ?, ?, ?)`,
		msg.ID, msg.WorkspaceID, msg.Role, msg.BlocksJSON, msg.Timestamp)
	return err
}

// GetWorkspaceMessages returns all messages for a workspace, oldest first.
func (s *Store) GetWorkspaceMessages(ctx context.Context, workspaceID string) ([]WorkspaceMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, role, blocks_json, timestamp FROM workspace_messages WHERE workspace_id = ? ORDER BY timestamp ASC`,
		workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []WorkspaceMessage
	for rows.Next() {
		var m WorkspaceMessage
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Role, &m.BlocksJSON, &m.Timestamp); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// ListAllWorkspaceMessages returns every message across every
// workspace, oldest first. Used by the chat-history backfill
// command to walk the SQLite table and re-index everything into
// glitch-chat-history without needing to enumerate workspaces
// separately.
//
// Returns the slice in chronological order so the backfill can
// stream into ES in the same sequence the user originally typed
// the messages — useful when the ES timestamp ties or for
// debugging "what came first".
func (s *Store) ListAllWorkspaceMessages(ctx context.Context) ([]WorkspaceMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, role, blocks_json, timestamp
		   FROM workspace_messages
		  ORDER BY timestamp ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []WorkspaceMessage
	for rows.Next() {
		var m WorkspaceMessage
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Role, &m.BlocksJSON, &m.Timestamp); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// DeleteWorkspaceMessage removes one row by id. Used by the
// prune-phantoms command after it identifies a stale injection
// that the user wants gone retroactively. Idempotent: deleting a
// non-existent id is not an error.
func (s *Store) DeleteWorkspaceMessage(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM workspace_messages WHERE id = ?`, id)
	return err
}
