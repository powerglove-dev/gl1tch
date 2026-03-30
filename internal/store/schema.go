package store

import "database/sql"

// createSchema is the DDL for the runs table.
const createSchema = `CREATE TABLE IF NOT EXISTS runs (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL,
  name        TEXT NOT NULL,
  started_at  INTEGER NOT NULL,
  finished_at INTEGER,
  exit_status INTEGER,
  stdout      TEXT,
  stderr      TEXT,
  metadata    TEXT
);`

// addStepsColumn is the migration that adds the steps column to an existing
// runs table that was created before this column existed.
const addStepsColumn = `ALTER TABLE runs ADD COLUMN steps TEXT DEFAULT '[]'`

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

// applySchema runs the schema migration against db.
func applySchema(db *sql.DB) error {
	if _, err := db.Exec(createSchema); err != nil {
		return err
	}
	if err := applyStepsColumnMigration(db); err != nil {
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
	if err := applyClarificationsTableMigration(db); err != nil {
		return err
	}
	if err := applyClarificationStepIDMigration(db); err != nil {
		return err
	}
	return applyStepCheckpointsTableMigration(db)
}

// applyStepCheckpointsTableMigration creates the step_checkpoints table if it
// does not already exist. CREATE TABLE IF NOT EXISTS is idempotent.
func applyStepCheckpointsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createStepCheckpointsSchema)
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
