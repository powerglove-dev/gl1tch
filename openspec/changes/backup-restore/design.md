## Context

User data is spread across two locations:
- `~/.config/glitch/` — YAML/Markdown config files (pipelines, prompts, wrappers, themes, cron, layout, keybindings, translations, config)
- `~/.local/share/glitch/glitch.db` — SQLite database (brain notes, saved prompts, run history)

There is currently no mechanism to back up or restore this data. The existing `glitchConfigDir()` helper in `cmd/` resolves the config directory; the store layer exposes `AllBrainNotes()` but lacks a corresponding export for saved prompts. No new third-party dependencies are desired.

## Goals / Non-Goals

**Goals:**
- Single-command backup to a portable `.tar.gz` file
- Single-command restore with merge semantics (skip existing by default, `--overwrite` to replace)
- Human-readable DB export (JSONL) inside the archive so data is inspectable without SQLite
- Deduplication on restore to make re-importing safe and idempotent
- Dry-run mode for restore so users can preview before committing

**Non-Goals:**
- Scheduled/automatic backups (out of scope for this change)
- Encrypting the archive
- Backing up run history (`runs`, `steps` tables) — low value, high volume
- Backing up `brain_vectors` — can be re-indexed from note bodies on restore
- Cloud sync or remote storage

## Decisions

### Archive format: `.tar.gz` over `.zip`

Go's stdlib `archive/tar` + `compress/gzip` handles tar.gz natively without external deps. Tar preserves file modes and relative paths cleanly. The resulting file is a standard Unix artifact users can inspect with `tar -tzf`.

**Alternatives considered:** `.zip` (stdlib `archive/zip`) — rejected because tar.gz is idiomatic on Unix and the stdlib tar API is simpler for streaming writes.

### DB export as JSONL alongside the binary blob

The archive includes both the raw `glitch.db` file and JSONL exports (`brain_notes.jsonl`, `saved_prompts.jsonl`). The JSONL makes data human-readable and git-diffable. On restore, JSONL is the primary import source; the raw DB is included as a fallback for advanced users.

**Alternatives considered:** Export JSONL only — rejected because losing the raw DB makes point-in-time recovery harder for advanced users.

### Restore merge strategy: skip by default, `--overwrite` to replace

For config files: if the destination file already exists, skip unless `--overwrite` is passed. This prevents clobbering an actively-used config with a stale backup.

For DB records: deduplicate by `id`. Records with matching IDs are skipped (not overwritten) even with `--overwrite`, since brain notes are append-only by design.

### New `internal/backup` package

Backup and restore logic lives in `internal/backup/` (not in `cmd/`) to keep the command layer thin and make the logic testable. `cmd/backup.go` and `cmd/restore.go` are thin wrappers that parse flags and call into the package.

## Risks / Trade-offs

- **Partial backup if DB is locked** → Mitigation: copy the DB file using SQLite's backup API or a read transaction to get a consistent snapshot; fall back to file copy if that fails.
- **Config dir not writable on restore** → Mitigation: check writability before extracting, return actionable error.
- **Archive path collisions** → Mitigation: default output name includes RFC3339 date (`glitch-backup-2026-04-01.tar.gz`); warn if file exists.
- **JSONL dedup misses semantic duplicates** (same content, different IDs) → Accepted trade-off; ID-based dedup is simple and safe.

## Migration Plan

No migration required. Commands are additive. Users run `glitch backup` to create their first backup at any time.
