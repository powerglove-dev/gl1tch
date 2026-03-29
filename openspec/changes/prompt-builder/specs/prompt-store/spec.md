## ADDED Requirements

### Requirement: Prompts table schema
The SQLite database SHALL include a `prompts` table with the following columns: `id` (INTEGER PRIMARY KEY AUTOINCREMENT), `title` (TEXT NOT NULL), `body` (TEXT NOT NULL), `model_slug` (TEXT NOT NULL DEFAULT ''), `created_at` (INTEGER NOT NULL), `updated_at` (INTEGER NOT NULL). The table SHALL be created via an additive migration in `applySchema` using `CREATE TABLE IF NOT EXISTS`.

#### Scenario: Table created on store open
- **WHEN** `store.Open()` or `store.OpenAt()` is called
- **THEN** the `prompts` table exists in the database (idempotent — safe on existing databases)

#### Scenario: Existing databases are unaffected
- **WHEN** `store.Open()` is called on a database that does not yet have the `prompts` table
- **THEN** the table is created without error and existing data is preserved

### Requirement: Insert prompt
The store SHALL expose `InsertPrompt(ctx, Prompt) (int64, error)` that inserts a new prompt record and returns its generated ID. `created_at` and `updated_at` SHALL be set to the current Unix timestamp.

#### Scenario: Insert returns new ID
- **WHEN** `InsertPrompt` is called with a valid title and body
- **THEN** the returned ID is positive and the row exists in the `prompts` table

#### Scenario: Insert sets timestamps
- **WHEN** `InsertPrompt` is called
- **THEN** `created_at` and `updated_at` are both set to the current Unix timestamp

### Requirement: Update prompt
The store SHALL expose `UpdatePrompt(ctx, Prompt) error` that updates `title`, `body`, `model_slug`, and `updated_at` for an existing prompt by ID. It SHALL return an error if the prompt does not exist.

#### Scenario: Update modifies record
- **WHEN** `UpdatePrompt` is called with a modified body
- **THEN** the `body` and `updated_at` columns are updated in the database

#### Scenario: Update of missing ID returns error
- **WHEN** `UpdatePrompt` is called with an ID that does not exist
- **THEN** an error is returned

### Requirement: Delete prompt
The store SHALL expose `DeletePrompt(ctx, id int64) error` that removes the prompt with the given ID. It SHALL return an error if the prompt does not exist.

#### Scenario: Delete removes record
- **WHEN** `DeletePrompt` is called with a valid ID
- **THEN** the row is removed from the `prompts` table

#### Scenario: Delete of missing ID returns error
- **WHEN** `DeletePrompt` is called with an ID that does not exist
- **THEN** an error is returned

### Requirement: List prompts
The store SHALL expose `ListPrompts(ctx) ([]Prompt, error)` that returns all prompts ordered by `updated_at` DESC.

#### Scenario: Returns all prompts newest-first
- **WHEN** `ListPrompts` is called with prompts in the store
- **THEN** the returned slice is ordered by `updated_at` DESC

#### Scenario: Returns empty slice when no prompts
- **WHEN** `ListPrompts` is called on an empty table
- **THEN** an empty (non-nil) slice and no error are returned

### Requirement: Search prompts
The store SHALL expose `SearchPrompts(ctx, query string) ([]Prompt, error)` that returns prompts where `title` or `body` contains the query string (case-insensitive LIKE). Results SHALL be ordered by `updated_at` DESC.

#### Scenario: Search matches title
- **WHEN** `SearchPrompts` is called with a query matching a prompt's title
- **THEN** that prompt is included in the results

#### Scenario: Search matches body
- **WHEN** `SearchPrompts` is called with a query matching content in a prompt's body
- **THEN** that prompt is included in the results

#### Scenario: Search returns empty on no match
- **WHEN** `SearchPrompts` is called with a query that matches nothing
- **THEN** an empty slice and no error are returned

### Requirement: Get prompt by ID
The store SHALL expose `GetPrompt(ctx, id int64) (Prompt, error)` that returns the prompt with the given ID or an error if not found.

#### Scenario: Get returns prompt
- **WHEN** `GetPrompt` is called with an existing ID
- **THEN** the full `Prompt` struct is returned with all fields populated

#### Scenario: Get of missing ID returns error
- **WHEN** `GetPrompt` is called with an ID that does not exist
- **THEN** an error is returned (e.g. `sql.ErrNoRows`)
