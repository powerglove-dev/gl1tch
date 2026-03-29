## Why

ORCAI has no way to author, store, or iteratively test AI prompts — users must manage prompt text externally and re-paste it into agent runners or pipeline YAML every session. A first-class prompt builder gives users a persistent, searchable prompt library with an inline test loop that lets them refine prompts against a live model before wiring them into agents or pipelines.

## What Changes

- Add a **Prompt Manager TUI** — a full-screen modal (like the inbox detail) accessible from the jump window and keybindings, with browse/search/edit/delete of saved prompts
- Add a **Prompt Editor panel** — text area for writing or editing a prompt, with model/agent selector
- Add a **Prompt Test Runner panel** — inline run against the selected model with scrollable response output, allowing prompt iteration without leaving the modal
- Add a **prompt store** — SQLite-backed persistence for named prompts (title, body, model, created_at, updated_at)
- Integrate **prompt pre-selection** into the agent runner modal — users can pick a saved prompt from a dropdown before launching
- Integrate **prompt selection** into the pipeline builder — a `prompt_id` field on steps that execute agent/model calls
- Add a **jump window entry** — "prompts" synthetic entry in the jump window navigates to the prompt manager, same pattern as "cron"

## Capabilities

### New Capabilities

- `prompt-manager`: Full-screen TUI modal for browsing, searching, creating, editing, and deleting saved prompts; accessible from jump window and keyboard shortcut
- `prompt-store`: SQLite schema and CRUD operations for persisting named prompts (title, body, model slug, timestamps)
- `prompt-test-runner`: Inline prompt execution panel within the prompt manager — run a prompt against a selected model, view streamed response, edit prompt, and repeat

### Modified Capabilities

<!-- none — the agent runner modal has no existing spec; prompt pre-selection is covered under prompt-manager -->

## Impact

- New package `internal/promptmgr/` — BubbleTea model, view, update, keys for the prompt manager TUI
- New store methods in `internal/store/` — `InsertPrompt`, `UpdatePrompt`, `DeletePrompt`, `ListPrompts`, `SearchPrompts`, `GetPrompt`; new `prompts` table via schema migration
- `internal/jumpwindow/jumpwindow.go` — add "prompts" synthetic sysop entry
- `internal/switchboard/` or agent runner modal — add prompt picker dropdown
- `internal/pipeline/` step schema — optional `prompt_id` string field (non-breaking, omitempty)
- Theme system inherited automatically via existing `tuikit.ThemeState` pattern
