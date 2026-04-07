package store

import "database/sql"

// createSchema is the DDL for the runs table.
//
// workspace_id ties each run to the workspace it executed in (set by
// the chain runner via the pipeline runner's WithWorkspaceID option)
// so the PipelineIndexer can scope its query to a single workspace's
// runs and avoid cross-contamination in the glitch-pipelines index.
const createSchema = `CREATE TABLE IF NOT EXISTS runs (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  kind         TEXT NOT NULL,
  name         TEXT NOT NULL,
  started_at   INTEGER NOT NULL,
  finished_at  INTEGER,
  exit_status  INTEGER,
  stdout       TEXT,
  stderr       TEXT,
  metadata     TEXT,
  workspace_id TEXT NOT NULL DEFAULT ''
);`

// addStepsColumn is the migration that adds the steps column to an existing
// runs table that was created before this column existed.
const addStepsColumn = `ALTER TABLE runs ADD COLUMN steps TEXT DEFAULT '[]'`

// addRunsWorkspaceIDColumn migrates legacy runs tables (created before
// the workspace-scoped collectors split) to include a workspace_id
// keyword column. Empty string means "global / unattributed" so
// pre-existing rows continue to work without backfill.
const addRunsWorkspaceIDColumn = `ALTER TABLE runs ADD COLUMN workspace_id TEXT NOT NULL DEFAULT ''`

// createBrainNotesSchema is the DDL for the brain_notes table.
const createBrainNotesSchema = `CREATE TABLE IF NOT EXISTS brain_notes (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id      INTEGER NOT NULL,
  step_id     TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  tags        TEXT DEFAULT '',
  body        TEXT NOT NULL
);`

// createPromptsSchema is the DDL for the prompts table.
const createPromptsSchema = `CREATE TABLE IF NOT EXISTS prompts (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title       TEXT NOT NULL,
  body        TEXT NOT NULL,
  model_slug  TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);`

// addPromptLastResponseColumn is the migration that adds last_response to the
// prompts table for databases created before this column existed.
const addPromptLastResponseColumn = `ALTER TABLE prompts ADD COLUMN last_response TEXT DEFAULT ''`

// addPromptCWDColumn is the migration that adds cwd to the prompts table for
// databases created before this column existed.
const addPromptCWDColumn = `ALTER TABLE prompts ADD COLUMN cwd TEXT DEFAULT ''`

// addPromptInputFormatColumn / addPromptOutputFormatColumn are the
// migrations that add the optional input/output format hints to a
// prompt. Empty string means "free-form text" — the default for any
// prompt that hasn't opted into a structured shape yet. The builder UI
// uses these to lint downstream plane wiring without forcing the user
// to declare a schema upfront.
const addPromptInputFormatColumn = `ALTER TABLE prompts ADD COLUMN input_format TEXT NOT NULL DEFAULT ''`
const addPromptOutputFormatColumn = `ALTER TABLE prompts ADD COLUMN output_format TEXT NOT NULL DEFAULT ''`

// createDraftsSchema is the DDL for the drafts table. A draft is a
// work-in-progress prompt / workflow / skill / agent that the user is
// iterating on with the gl1tch prompt agent. Each refinement turn is
// appended to turns_json so the conversation history survives restarts
// and the user can resume tomorrow.
//
// kind ∈ {"prompt", "workflow", "skill", "agent"}.
//
// target_id is set when the draft is editing an existing prompt row;
// target_path is set when the draft is editing a file-backed entity
// (workflow yaml, skill md). Both unset means the draft is brand new
// and hasn't been promoted yet.
const createDraftsSchema = `CREATE TABLE IF NOT EXISTS drafts (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id  TEXT    NOT NULL,
  kind          TEXT    NOT NULL,
  title         TEXT    NOT NULL DEFAULT '',
  body          TEXT    NOT NULL DEFAULT '',
  turns_json    TEXT    NOT NULL DEFAULT '[]',
  target_id     INTEGER,
  target_path   TEXT    NOT NULL DEFAULT '',
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_drafts_ws_kind ON drafts(workspace_id, kind, updated_at DESC);`

// addDraftInputFormatColumn / addDraftOutputFormatColumn mirror the
// prompt format columns on the drafts table so the editor popup can
// hold the user's in-progress format choices alongside the body
// before they're promoted to the real prompts row. Empty = free-form
// text, same convention as the prompts table.
const addDraftInputFormatColumn = `ALTER TABLE drafts ADD COLUMN input_format TEXT NOT NULL DEFAULT ''`
const addDraftOutputFormatColumn = `ALTER TABLE drafts ADD COLUMN output_format TEXT NOT NULL DEFAULT ''`

// addClarificationStepIDColumn is the migration that adds step_id to the
// clarifications table for databases created before this column existed.
const addClarificationStepIDColumn = `ALTER TABLE clarifications ADD COLUMN step_id TEXT NOT NULL DEFAULT ''`

// createClarificationsSchema is the DDL for the clarifications table.
const createClarificationsSchema = `CREATE TABLE IF NOT EXISTS clarifications (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id      TEXT    NOT NULL,
    question    TEXT    NOT NULL DEFAULT '',
    output      TEXT    NOT NULL DEFAULT '',
    asked_at    INTEGER NOT NULL,
    answered_at INTEGER,
    answer      TEXT
)`

// dropBrainVectorsSchema removes the legacy SQLite vector store. As of
// the brainrag → Elasticsearch migration, embeddings live in the
// glitch-vectors index instead. The table is dropped (rather than left
// dangling) so old gl1tch.db files don't carry stale BLOB data forever.
const dropBrainVectorsSchema = `DROP TABLE IF EXISTS brain_vectors`

// createStepCheckpointsSchema is the DDL for the step_checkpoints table.
const createStepCheckpointsSchema = `CREATE TABLE IF NOT EXISTS step_checkpoints (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id      INTEGER NOT NULL,
  step_id     TEXT    NOT NULL,
  step_index  INTEGER NOT NULL DEFAULT 0,
  status      TEXT    NOT NULL DEFAULT 'pending',
  prompt      TEXT    NOT NULL DEFAULT '',
  output      TEXT    NOT NULL DEFAULT '',
  model       TEXT    NOT NULL DEFAULT '',
  vars_json   TEXT    NOT NULL DEFAULT '{}',
  started_at  INTEGER,
  finished_at INTEGER,
  duration_ms INTEGER,
  UNIQUE(run_id, step_id)
)`

// createScoreEventsSchema is the DDL for the score_events table.
const createScoreEventsSchema = `CREATE TABLE IF NOT EXISTS score_events (
  id                    INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id                INTEGER,
  xp                    INTEGER NOT NULL DEFAULT 0,
  input_tokens          INTEGER NOT NULL DEFAULT 0,
  output_tokens         INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
  cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
  cost_usd              REAL NOT NULL DEFAULT 0,
  provider              TEXT NOT NULL DEFAULT '',
  model                 TEXT NOT NULL DEFAULT '',
  created_at            INTEGER NOT NULL
)`

// createUserScoreSchema is the DDL for the user_score table.
const createUserScoreSchema = `CREATE TABLE IF NOT EXISTS user_score (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  total_xp      INTEGER NOT NULL DEFAULT 0,
  level         INTEGER NOT NULL DEFAULT 1,
  streak_days   INTEGER NOT NULL DEFAULT 0,
  last_run_date TEXT NOT NULL DEFAULT '',
  total_runs    INTEGER NOT NULL DEFAULT 0
)`

// createAchievementsSchema is the DDL for the achievements table.
const createAchievementsSchema = `CREATE TABLE IF NOT EXISTS achievements (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  achievement_id TEXT NOT NULL UNIQUE,
  unlocked_at    INTEGER NOT NULL
)`

// createWorkflowRunsSchema is the DDL for the workflow_runs table.
const createWorkflowRunsSchema = `CREATE TABLE IF NOT EXISTS workflow_runs (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'running',
  input        TEXT,
  output       TEXT,
  error        TEXT,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  completed_at DATETIME
)`

// createICEEncountersSchema is the DDL for the ice_encounters table.
const createICEEncountersSchema = `CREATE TABLE IF NOT EXISTS ice_encounters (
  id         TEXT PRIMARY KEY,
  ice_class  TEXT NOT NULL,
  run_id     TEXT NOT NULL DEFAULT '',
  deadline   INTEGER NOT NULL,
  resolved   INTEGER NOT NULL DEFAULT 0,
  outcome    TEXT
)`

// createWorkspacesSchema is the DDL for workspace tables.
const createWorkspacesSchema = `
CREATE TABLE IF NOT EXISTS workspaces (
  id          TEXT PRIMARY KEY,
  title       TEXT NOT NULL DEFAULT 'New Chat',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS workspace_directories (
  workspace_id TEXT NOT NULL,
  path         TEXT NOT NULL,
  repo_name    TEXT NOT NULL,
  PRIMARY KEY (workspace_id, path)
);
CREATE TABLE IF NOT EXISTS workspace_messages (
  id            TEXT PRIMARY KEY,
  workspace_id  TEXT NOT NULL,
  role          TEXT NOT NULL,
  blocks_json   TEXT NOT NULL,
  timestamp     INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ws_messages_ws ON workspace_messages(workspace_id, timestamp);
`

// createPersonalBestsSchema is the DDL for the game_personal_bests table.
const createPersonalBestsSchema = `CREATE TABLE IF NOT EXISTS game_personal_bests (
  metric      TEXT PRIMARY KEY,
  value       REAL NOT NULL DEFAULT 0,
  run_id      TEXT NOT NULL DEFAULT '',
  recorded_at INTEGER NOT NULL
)`

// chat_workflows table was removed in the YAML unification. Existing
// databases still carry the table; the one-shot
// MigrateChatWorkflowsToYAML in pkg/glitchd reads it via raw SQL,
// writes each row out as a .workflow.yaml file, and deletes the row.
// Fresh databases never see the table.

// createWorkflowCheckpointsSchema is the DDL for the workflow_checkpoints table.
const createWorkflowCheckpointsSchema = `CREATE TABLE IF NOT EXISTS workflow_checkpoints (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id       INTEGER NOT NULL REFERENCES workflow_runs(id),
  step_id      TEXT NOT NULL,
  status       TEXT NOT NULL,
  context_json TEXT NOT NULL,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
)`

// applyBrainVectorsEmbedIDMigration is a no-op since the brain_vectors
// table no longer exists. Kept as a stub so the migration sequence in
// applySchema doesn't change order — existing dbs still walk the same
// list, they just hit a drop instead of a column add.
func applyBrainVectorsEmbedIDMigration(db *sql.DB) error {
	return nil
}

// applySchema runs the schema migration against db.
func applySchema(db *sql.DB) error {
	if _, err := db.Exec(createSchema); err != nil {
		return err
	}
	if err := applyStepsColumnMigration(db); err != nil {
		return err
	}
	if err := applyRunsWorkspaceIDMigration(db); err != nil {
		return err
	}
	if err := applyBrainNotesTableMigration(db); err != nil {
		return err
	}
	if err := applyPromptsTableMigration(db); err != nil {
		return err
	}
	if err := applyPromptLastResponseMigration(db); err != nil {
		return err
	}
	if err := applyPromptCWDMigration(db); err != nil {
		return err
	}
	if err := applyPromptFormatColumnsMigration(db); err != nil {
		return err
	}
	if err := applyClarificationsTableMigration(db); err != nil {
		return err
	}
	if err := applyClarificationStepIDMigration(db); err != nil {
		return err
	}
	if err := applyStepCheckpointsTableMigration(db); err != nil {
		return err
	}
	if err := applyBrainVectorsTableMigration(db); err != nil {
		return err
	}
	if err := applyBrainVectorsEmbedIDMigration(db); err != nil {
		return err
	}
	if err := applyScoreEventsTableMigration(db); err != nil {
		return err
	}
	if err := applyUserScoreTableMigration(db); err != nil {
		return err
	}
	if err := applyAchievementsTableMigration(db); err != nil {
		return err
	}
	if err := applyWorkflowRunsTableMigration(db); err != nil {
		return err
	}
	if err := applyWorkflowCheckpointsTableMigration(db); err != nil {
		return err
	}
	if err := applyICEEncountersTableMigration(db); err != nil {
		return err
	}
	if err := applyPersonalBestsTableMigration(db); err != nil {
		return err
	}
	if err := applyWorkspacesTableMigration(db); err != nil {
		return err
	}
	if err := applyDraftsTableMigration(db); err != nil {
		return err
	}
	return applyDraftFormatColumnsMigration(db)
}

// applyDraftsTableMigration creates the drafts table and its index if
// they do not already exist. Idempotent.
func applyDraftsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createDraftsSchema)
	return err
}

// applyStepCheckpointsTableMigration creates the step_checkpoints table if it
// does not already exist. CREATE TABLE IF NOT EXISTS is idempotent.
func applyStepCheckpointsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createStepCheckpointsSchema)
	return err
}

// applyBrainVectorsTableMigration drops the legacy brain_vectors
// SQLite table. As of the brainrag → ES migration, embeddings live in
// the glitch-vectors index. The drop is idempotent (DROP TABLE IF
// EXISTS) so it's safe to run on every startup.
func applyBrainVectorsTableMigration(db *sql.DB) error {
	_, err := db.Exec(dropBrainVectorsSchema)
	return err
}

// applyPromptsTableMigration creates the prompts table if it does not already
// exist. CREATE TABLE IF NOT EXISTS is idempotent, so this is safe to run on
// every startup.
func applyPromptsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createPromptsSchema)
	return err
}

// applyBrainNotesTableMigration creates the brain_notes table if it does not
// already exist. CREATE TABLE IF NOT EXISTS is idempotent, so this is safe to
// run on every startup.
func applyBrainNotesTableMigration(db *sql.DB) error {
	_, err := db.Exec(createBrainNotesSchema)
	return err
}

// applyPromptLastResponseMigration adds the last_response column to the prompts
// table if it does not already exist. modernc.org/sqlite does not support
// ALTER TABLE ... ADD COLUMN IF NOT EXISTS, so we probe pragma_table_info first.
func applyPromptLastResponseMigration(db *sql.DB) error {
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('prompts') WHERE name='last_response'`)
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(addPromptLastResponseColumn); err != nil {
			return err
		}
	}
	return nil
}

// applyPromptCWDMigration adds the cwd column to the prompts table if it does
// not already exist. modernc.org/sqlite does not support ALTER TABLE ... ADD
// COLUMN IF NOT EXISTS, so we probe pragma_table_info first.
func applyPromptCWDMigration(db *sql.DB) error {
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('prompts') WHERE name='cwd'`)
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(addPromptCWDColumn); err != nil {
			return err
		}
	}
	return nil
}

// applyPromptFormatColumnsMigration adds the input_format and
// output_format columns to the prompts table for legacy databases.
// Same probe-then-add pattern as the other column migrations because
// modernc.org/sqlite lacks ALTER TABLE ... ADD COLUMN IF NOT EXISTS.
func applyPromptFormatColumnsMigration(db *sql.DB) error {
	if err := addColumnIfMissing(db, "prompts", "input_format", addPromptInputFormatColumn); err != nil {
		return err
	}
	return addColumnIfMissing(db, "prompts", "output_format", addPromptOutputFormatColumn)
}

// applyDraftFormatColumnsMigration is the drafts-table counterpart to
// applyPromptFormatColumnsMigration. Runs after applyDraftsTableMigration
// so the table is guaranteed to exist before we probe it.
func applyDraftFormatColumnsMigration(db *sql.DB) error {
	if err := addColumnIfMissing(db, "drafts", "input_format", addDraftInputFormatColumn); err != nil {
		return err
	}
	return addColumnIfMissing(db, "drafts", "output_format", addDraftOutputFormatColumn)
}

// addColumnIfMissing probes pragma_table_info for column on table and
// runs alterSQL only when the column is absent. Centralizes the
// probe-then-add idiom so the format migrations don't repeat the same
// scaffolding twice.
func addColumnIfMissing(db *sql.DB, table, column, alterSQL string) error {
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column)
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(alterSQL); err != nil {
			return err
		}
	}
	return nil
}

// applyClarificationsTableMigration creates the clarifications table if it does
// not already exist. CREATE TABLE IF NOT EXISTS is idempotent, so this is safe
// to run on every startup.
func applyClarificationsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createClarificationsSchema)
	return err
}

// applyClarificationStepIDMigration adds the step_id column to the clarifications
// table if it does not already exist.
func applyClarificationStepIDMigration(db *sql.DB) error {
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('clarifications') WHERE name='step_id'`)
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(addClarificationStepIDColumn); err != nil {
			return err
		}
	}
	return nil
}

// applyScoreEventsTableMigration creates the score_events table if it does not
// already exist. CREATE TABLE IF NOT EXISTS is idempotent.
func applyScoreEventsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createScoreEventsSchema)
	return err
}

// applyUserScoreTableMigration creates the user_score table if it does not
// already exist. CREATE TABLE IF NOT EXISTS is idempotent.
func applyUserScoreTableMigration(db *sql.DB) error {
	_, err := db.Exec(createUserScoreSchema)
	return err
}

// applyAchievementsTableMigration creates the achievements table if it does not
// already exist. CREATE TABLE IF NOT EXISTS is idempotent.
func applyAchievementsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createAchievementsSchema)
	return err
}

// applyWorkflowRunsTableMigration creates the workflow_runs table if it does not
// already exist. CREATE TABLE IF NOT EXISTS is idempotent.
func applyWorkflowRunsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createWorkflowRunsSchema)
	return err
}

// applyWorkflowCheckpointsTableMigration creates the workflow_checkpoints table
// if it does not already exist. CREATE TABLE IF NOT EXISTS is idempotent.
func applyWorkflowCheckpointsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createWorkflowCheckpointsSchema)
	return err
}

// applyICEEncountersTableMigration creates the ice_encounters table if it does
// not already exist. CREATE TABLE IF NOT EXISTS is idempotent.
func applyICEEncountersTableMigration(db *sql.DB) error {
	_, err := db.Exec(createICEEncountersSchema)
	return err
}

// applyPersonalBestsTableMigration creates the game_personal_bests table if it
// does not already exist. CREATE TABLE IF NOT EXISTS is idempotent.
func applyPersonalBestsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createPersonalBestsSchema)
	return err
}

// applyWorkspacesTableMigration creates workspace tables if they don't exist.
func applyWorkspacesTableMigration(db *sql.DB) error {
	_, err := db.Exec(createWorkspacesSchema)
	return err
}

// applyStepsColumnMigration adds the steps column if it does not already exist.
// modernc.org/sqlite does not support ALTER TABLE ... ADD COLUMN IF NOT EXISTS,
// so we probe pragma_table_info first.
func applyStepsColumnMigration(db *sql.DB) error {
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('runs') WHERE name='steps'`)
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(addStepsColumn); err != nil {
			return err
		}
	}
	return nil
}

// applyRunsWorkspaceIDMigration adds the workspace_id column to the
// runs table for legacy databases. Same probe-then-add pattern as
// the other column migrations because modernc/sqlite lacks
// ALTER TABLE ... ADD COLUMN IF NOT EXISTS.
func applyRunsWorkspaceIDMigration(db *sql.DB) error {
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('runs') WHERE name='workspace_id'`)
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(addRunsWorkspaceIDColumn); err != nil {
			return err
		}
	}
	return nil
}
