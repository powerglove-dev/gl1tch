## ADDED Requirements

### Requirement: Backup command produces a portable archive
The system SHALL provide a `glitch backup` subcommand that collects all user config files and exported database records into a single `.tar.gz` archive.

#### Scenario: Default output path
- **WHEN** the user runs `glitch backup` with no flags
- **THEN** a file named `glitch-backup-<YYYY-MM-DD>.tar.gz` is created in the current working directory

#### Scenario: Custom output path
- **WHEN** the user runs `glitch backup --output /path/to/my-backup.tar.gz`
- **THEN** the archive is written to the specified path

#### Scenario: Output file already exists
- **WHEN** the output file path already exists
- **THEN** the command prints a warning and exits without overwriting

### Requirement: Archive includes all user config files
The system SHALL include the following paths from `~/.config/glitch/` in the archive, preserving relative directory structure:
- `pipelines/` (all `*.pipeline.yaml` files)
- `prompts/` (all `*.md` files)
- `wrappers/` (all `*.yaml` files)
- `themes/` (full directory tree)
- `cron.yaml`
- `layout.yaml`
- `keybindings.yaml`
- `config.yaml`
- `translations.yaml`

#### Scenario: Config directory exists with files
- **WHEN** the user runs `glitch backup` and `~/.config/glitch/` contains config files
- **THEN** those files appear in the archive under a `config/` prefix

#### Scenario: Optional subdirectory is missing
- **WHEN** a config subdirectory (e.g., `themes/`) does not exist
- **THEN** the command skips it without error and continues

### Requirement: Archive includes exported database records
The system SHALL export brain notes and saved prompts from `glitch.db` as JSONL files and include them in the archive alongside the raw database file.

#### Scenario: Brain notes export
- **WHEN** the user runs `glitch backup` and brain notes exist in the database
- **THEN** the archive contains `db/brain_notes.jsonl` with one JSON object per line per note

#### Scenario: Saved prompts export
- **WHEN** the user runs `glitch backup` and saved prompts exist in the database
- **THEN** the archive contains `db/saved_prompts.jsonl` with one JSON object per line per prompt

#### Scenario: Raw database included
- **WHEN** the user runs `glitch backup`
- **THEN** the archive contains `db/glitch.db` as a binary copy

#### Scenario: Database is empty
- **WHEN** the database contains no brain notes or saved prompts
- **THEN** empty JSONL files are written and no error is returned

### Requirement: Backup prints a manifest on completion
The system SHALL print a summary of what was included in the archive after a successful backup.

#### Scenario: Successful backup manifest
- **WHEN** `glitch backup` completes successfully
- **THEN** the output lists the archive path, total file count, and the number of brain notes and saved prompts exported
