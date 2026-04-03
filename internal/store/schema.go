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

// createBrainVectorsSchema is the DDL for the brain_vectors table.
const createBrainVectorsSchema = `CREATE TABLE IF NOT EXISTS brain_vectors (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  cwd         TEXT NOT NULL,
  note_id     TEXT NOT NULL,
  text        TEXT NOT NULL,
  vector      BLOB NOT NULL,
  hash        TEXT NOT NULL,
  embed_id    TEXT NOT NULL DEFAULT '',
  indexed_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(cwd, note_id)
)`

// addBrainVectorsEmbedIDColumn is the migration that adds embed_id to the
// brain_vectors table for databases created before this column existed.
const addBrainVectorsEmbedIDColumn = `ALTER TABLE brain_vectors ADD COLUMN embed_id TEXT NOT NULL DEFAULT ''`

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

// createPersonalBestsSchema is the DDL for the game_personal_bests table.
const createPersonalBestsSchema = `CREATE TABLE IF NOT EXISTS game_personal_bests (
  metric      TEXT PRIMARY KEY,
  value       REAL NOT NULL DEFAULT 0,
  run_id      TEXT NOT NULL DEFAULT '',
  recorded_at INTEGER NOT NULL
)`

// createWorkflowCheckpointsSchema is the DDL for the workflow_checkpoints table.
const createWorkflowCheckpointsSchema = `CREATE TABLE IF NOT EXISTS workflow_checkpoints (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id       INTEGER NOT NULL REFERENCES workflow_runs(id),
  step_id      TEXT NOT NULL,
  status       TEXT NOT NULL,
  context_json TEXT NOT NULL,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
)`

// applyBrainVectorsEmbedIDMigration adds the embed_id column to brain_vectors
// if it does not already exist, then back-fills '' rows with the legacy Ollama ID.
func applyBrainVectorsEmbedIDMigration(db *sql.DB) error {
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('brain_vectors') WHERE name='embed_id'`)
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(addBrainVectorsEmbedIDColumn); err != nil {
			return err
		}
		// Back-fill existing rows so they continue to work with the Ollama embedder.
		if _, err := db.Exec(`UPDATE brain_vectors SET embed_id = 'ollama:nomic-embed-text' WHERE embed_id = ''`); err != nil {
			return err
		}
	}
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
	return applyPersonalBestsTableMigration(db)
}

// applyStepCheckpointsTableMigration creates the step_checkpoints table if it
// does not already exist. CREATE TABLE IF NOT EXISTS is idempotent.
func applyStepCheckpointsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createStepCheckpointsSchema)
	return err
}

// applyBrainVectorsTableMigration creates the brain_vectors table if it does
// not already exist. CREATE TABLE IF NOT EXISTS is idempotent.
func applyBrainVectorsTableMigration(db *sql.DB) error {
	_, err := db.Exec(createBrainVectorsSchema)
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
