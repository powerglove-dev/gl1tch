## Why

User configuration and brain data live in `~/.config/glitch/` and `~/.local/share/glitch/glitch.db` with no backup mechanism — a lost machine or accidental deletion means permanent loss of pipelines, prompts, wrappers, themes, and accumulated brain notes. A single-command backup and restore makes glitch safe to use as a daily driver.

## What Changes

- New `glitch backup` command bundles config files and exported DB data into a portable `.tar.gz` archive
- New `glitch restore <file>` command unpacks an archive back into place, with merge and dry-run controls
- Brain notes and saved prompts are exported as JSONL inside the archive for human-readability and git-friendliness

## Capabilities

### New Capabilities

- `config-backup`: Export all user config (pipelines, prompts, wrappers, themes, cron, layout, keybindings, config, translations) plus brain notes and saved prompts into a `.tar.gz` archive
- `config-restore`: Import a backup archive, merging config files and re-importing DB records with deduplication

### Modified Capabilities

## Impact

- New files: `cmd/backup.go`, `cmd/restore.go`, `internal/backup/backup.go`, `internal/backup/restore.go`
- Touches: `internal/store/store.go` (needs `AllBrainNotes`, `AllSavedPrompts`, upsert methods surfaced), `cmd/root.go` (register new subcommands)
- No new external dependencies — uses stdlib `archive/tar`, `compress/gzip`, `encoding/json`
- No breaking changes
