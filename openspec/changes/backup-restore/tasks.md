## 1. Store Layer

- [x] 1.1 Add `AllSavedPrompts() ([]SavedPrompt, error)` to `internal/store/store.go` if not present
- [x] 1.2 Add `UpsertBrainNote(note BrainNote) error` (insert-or-ignore by id) to store
- [x] 1.3 Add `UpsertSavedPrompt(prompt SavedPrompt) error` (insert-or-ignore by id) to store

## 2. Backup Package

- [x] 2.1 Create `internal/backup/backup.go` with `Run(opts BackupOptions) (*Manifest, error)`
- [x] 2.2 Implement config file collection: walk `~/.config/glitch/` for the target paths, write into archive under `config/` prefix
- [x] 2.3 Implement DB export: call `AllBrainNotes()` and `AllSavedPrompts()`, write `db/brain_notes.jsonl` and `db/saved_prompts.jsonl` to archive
- [x] 2.4 Implement raw DB copy: stream `~/.local/share/glitch/glitch.db` into archive as `db/glitch.db`
- [x] 2.5 Build `.tar.gz` archive using stdlib `archive/tar` + `compress/gzip`
- [x] 2.6 Return `Manifest` struct with file count, note count, prompt count, archive path
- [x] 2.7 Handle output-file-exists case: return error if destination already exists

## 3. Restore Package

- [x] 3.1 Create `internal/backup/restore.go` with `Restore(archivePath string, opts RestoreOptions) (*RestoreSummary, error)`
- [x] 3.2 Implement archive extraction: unpack `config/**` entries to `~/.config/glitch/` with skip-existing / overwrite logic
- [x] 3.3 Implement JSONL import: parse `db/brain_notes.jsonl`, call `UpsertBrainNote()` per record, track imported vs skipped counts
- [x] 3.4 Implement JSONL import: parse `db/saved_prompts.jsonl`, call `UpsertSavedPrompt()` per record
- [x] 3.5 Implement `--dry-run`: collect planned actions without writing, return summary
- [x] 3.6 Return `RestoreSummary` with per-category counts (written, skipped, overwritten, imported, db-skipped)

## 4. CLI Commands

- [x] 4.1 Create `cmd/backup.go` — `glitch backup [--output <path>]` cobra command, call `backup.Run()`, print manifest
- [x] 4.2 Create `cmd/restore.go` — `glitch restore <file> [--dry-run] [--overwrite]` cobra command, call `backup.Restore()`, print summary
- [x] 4.3 Register both commands in `cmd/root.go`

## 5. Tests

- [x] 5.1 Unit test `backup.Run()`: verify archive contains expected entries for a temp config dir
- [x] 5.2 Unit test `backup.Restore()`: verify files are written/skipped correctly with and without `--overwrite`
- [x] 5.3 Unit test dry-run: verify no files are written and summary is correct
- [x] 5.4 Unit test DB dedup: verify records with duplicate IDs are skipped on restore
