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
// Directories / RepoNames only contain the *enabled* directory paths so
// existing collector code that consumes Workspace.Directories without
// caring about the toggle state automatically respects the user's
// per-directory enable/disable choices. Code that needs the full set
// (including disabled rows for the UI toggle list) calls
// ListWorkspaceDirectories instead.
type Workspace struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Directories []string `json:"directories"` // absolute paths (enabled only)
	RepoNames   []string `json:"repo_names"`  // filepath.Base of each dir (enabled only)
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
}

// WorkspaceDirectory is one row from workspace_directories with its
// enable flag. Used by the desktop UI to render per-directory toggles
// alongside the directory list — the legacy Workspace.Directories
// field hides disabled rows so collectors don't have to know about the
// flag, but the UI does.
type WorkspaceDirectory struct {
	Path     string `json:"path"`
	RepoName string `json:"repo_name"`
	Enabled  bool   `json:"enabled"`
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

// AddWorkspaceDirectory associates a directory with a workspace.
func (s *Store) AddWorkspaceDirectory(ctx context.Context, workspaceID, path string) error {
	repoName := filepath.Base(path)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO workspace_directories (workspace_id, path, repo_name) VALUES (?, ?, ?)`,
		workspaceID, path, repoName)
	return err
}

// RemoveWorkspaceDirectory removes a directory association from a workspace.
func (s *Store) RemoveWorkspaceDirectory(ctx context.Context, workspaceID, path string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM workspace_directories WHERE workspace_id = ? AND path = ?`,
		workspaceID, path)
	return err
}

// getWorkspaceDirectories returns only the *enabled* directories for a
// workspace. This is what Workspace.Directories surfaces — collector
// code doesn't need to know that disabled rows even exist.
func (s *Store) getWorkspaceDirectories(ctx context.Context, workspaceID string) ([]string, []string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, repo_name FROM workspace_directories WHERE workspace_id = ? AND enabled = 1`, workspaceID)
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

// ListWorkspaceDirectories returns every directory associated with a
// workspace, including disabled ones, with their enable flags. The
// desktop UI uses this to render the per-directory toggle list in the
// workspace settings popover.
func (s *Store) ListWorkspaceDirectories(ctx context.Context, workspaceID string) ([]WorkspaceDirectory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, repo_name, enabled FROM workspace_directories WHERE workspace_id = ? ORDER BY path`,
		workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WorkspaceDirectory
	for rows.Next() {
		var d WorkspaceDirectory
		var enabled int
		if err := rows.Scan(&d.Path, &d.RepoName, &enabled); err != nil {
			return nil, err
		}
		d.Enabled = enabled != 0
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
