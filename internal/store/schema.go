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
	return applyPromptCWDMigration(db)
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
