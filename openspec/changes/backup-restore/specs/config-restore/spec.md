## ADDED Requirements

### Requirement: Restore command unpacks a backup archive
The system SHALL provide a `glitch restore <file>` subcommand that extracts a backup archive produced by `glitch backup` and restores user config and database records.

#### Scenario: Successful restore
- **WHEN** the user runs `glitch restore glitch-backup-2026-04-01.tar.gz`
- **THEN** config files are written to `~/.config/glitch/` and DB records are imported

#### Scenario: Archive file not found
- **WHEN** the user specifies a path that does not exist
- **THEN** the command exits with a clear error message and makes no changes

#### Scenario: Invalid archive format
- **WHEN** the user provides a file that is not a valid `.tar.gz`
- **THEN** the command exits with a descriptive error and makes no changes

### Requirement: Restore skips existing config files by default
The system SHALL skip extracting a config file if a file already exists at the destination path, unless `--overwrite` is passed.

#### Scenario: File exists, no overwrite flag
- **WHEN** a config file from the archive already exists at its destination
- **AND** `--overwrite` is not passed
- **THEN** the file is skipped and reported in the output as "skipped (exists)"

#### Scenario: File exists with overwrite flag
- **WHEN** a config file from the archive already exists at its destination
- **AND** `--overwrite` is passed
- **THEN** the existing file is replaced with the archived version

#### Scenario: File does not exist
- **WHEN** a config file from the archive does not yet exist at its destination
- **THEN** the file is written regardless of `--overwrite`

### Requirement: Restore deduplicates database records by ID
The system SHALL import brain notes and saved prompts from JSONL files in the archive, skipping any record whose `id` already exists in the local database.

#### Scenario: Record not present locally
- **WHEN** a brain note or saved prompt in the archive has an ID not found in the local database
- **THEN** it is inserted into the database

#### Scenario: Record already present locally
- **WHEN** a brain note or saved prompt in the archive has an ID that already exists in the local database
- **THEN** it is skipped (not overwritten)

#### Scenario: JSONL file is empty
- **WHEN** the archive contains an empty `brain_notes.jsonl` or `saved_prompts.jsonl`
- **THEN** no records are imported and no error is returned

### Requirement: Dry-run mode previews changes without writing
The system SHALL support a `--dry-run` flag that shows what would be created, overwritten, or skipped without making any changes.

#### Scenario: Dry-run with existing files
- **WHEN** the user runs `glitch restore <file> --dry-run`
- **THEN** the command prints each file and record action (would write / would skip) and exits without modifying anything

#### Scenario: Dry-run reports DB import count
- **WHEN** `--dry-run` is passed
- **THEN** the output includes how many brain notes and saved prompts would be imported vs skipped

### Requirement: Restore prints a summary on completion
The system SHALL print a summary of actions taken after a successful restore.

#### Scenario: Restore summary output
- **WHEN** `glitch restore` completes successfully
- **THEN** the output lists the number of config files written, skipped, and overwritten, plus DB records imported and skipped
