## Context

ORCAI is a BubbleTea TUI application backed by SQLite (`internal/store/`) for persistence. Existing full-screen modals (inbox detail, crontui) follow a consistent pattern: a standalone BubbleTea program launched via a CLI subcommand (`orcai _inbox`, `orcai _cron`) and integrated into the jump window as a synthetic sysop entry. The pipeline builder lives in `internal/promptbuilder/` (confusingly named — it builds pipeline steps, not AI prompts). The new prompt manager is a distinct feature for authoring and testing AI prompts.

Providers (Claude, Ollama, OpenCode, etc.) are managed by `internal/providers/` and exposed via the plugin system. The agent runner modal in the switchboard already lets users select a model and run agents; the prompt manager needs to plug into the same provider/model selection flow.

## Goals / Non-Goals

**Goals:**
- Persistent, searchable prompt library backed by SQLite
- Full-screen BubbleTea modal matching ABBS aesthetic (Dracula palette, box-drawing borders)
- Inline test loop: write prompt → run against model → view response → edit → repeat
- Jump window integration (same as cron)
- Prompt pre-selection in the agent runner modal (switchboard)
- Optional `prompt_id` reference on pipeline steps
- Theme inheritance via `tuikit.ThemeState`

**Non-Goals:**
- Prompt versioning or diff history (prompts are last-write-wins)
- Prompt sharing or export to external systems
- Prompt chaining or composition
- Changing the existing `internal/promptbuilder/` pipeline builder

## Decisions

### New package `internal/promptmgr/` (not `promptbuilder`)

The existing `internal/promptbuilder/` is the pipeline step builder and must not be renamed. The new prompt management TUI lives in `internal/promptmgr/` to avoid confusion.

**Alternatives considered:** Reuse `promptbuilder/` namespace — rejected because the existing package's purpose (pipeline construction) is unrelated and already has tests.

### SQLite store with migration pattern

Prompts are persisted in the existing SQLite database via a new `prompts` table, using the established `applySchema` + additive migration pattern in `internal/store/schema.go`. No new database file is needed.

Fields: `id`, `title`, `body`, `model_slug`, `created_at`, `updated_at`.

**Alternatives considered:** Flat YAML files in `~/.config/orcai/prompts/` — rejected because fuzzy search, ordering, and CRUD are trivially handled by SQLite and it matches how cron entries and brain notes are stored.

### Three-panel layout

The prompt manager modal uses a three-panel layout:
1. **Left panel** — prompt list (browsable, fuzzy-searchable) with title and model slug
2. **Right-top panel** — prompt editor (textarea for body, model selector, title input)
3. **Right-bottom panel** — test runner output (streamed response, scrollable)

This mirrors the inbox detail two-column layout (`internal/inbox/`) which the user cited as the reference.

**Alternatives considered:** Single-panel wizard — rejected because it hides the list context while editing; two-panel without inline test — rejected because the iterative test loop is a core requirement.

### Inline test via existing provider/model invocation

The test runner calls the provider by reusing the same model invocation path as chatui/the agent runner. It does not spawn a new tmux window or agent session — it runs inline and streams output into the right-bottom panel via a `tea.Cmd`.

**Alternatives considered:** Launch a full agent session for testing — rejected because it adds session overhead and navigation complexity for what is a tight prompt-tuning loop.

### Jump window synthetic entry

Add a `promptsWindow bool` field to the `window` struct in `internal/jumpwindow/jumpwindow.go`, following the exact pattern of `cronSession`. Selecting it runs `orcai _promptmgr` in a new or existing tmux window named `orcai-prompts`.

### Prompt pre-selection in agent runner

The switchboard agent runner modal gains a "Prompt" field rendered above the existing fields. It shows a searchable dropdown of prompt titles. When selected, the prompt body is injected as the initial message/context for the agent run. The `prompt_id` is passed as a flag to the agent runner CLI.

### Pipeline `prompt_id` field

`pipeline.Step` gains an optional `PromptID string \`yaml:"prompt_id,omitempty"\`` field. During pipeline execution, if `prompt_id` is set, the store is queried for the prompt body and it is prepended to the step's input. This is a non-breaking additive change.

## Risks / Trade-offs

- **`internal/promptbuilder/` naming confusion** → Mitigated by using `promptmgr` for the new package and adding a package comment that distinguishes the two.
- **Inline streaming in BubbleTea** → Streaming LLM responses into a viewport requires careful use of `tea.Cmd` and channel-based message passing. Existing chatui code provides a working reference. Risk: output truncation or stall on slow models. Mitigation: show a spinner, allow cancellation via `q`/`esc`.
- **SQLite migration on upgrade** → New `prompts` table uses `CREATE TABLE IF NOT EXISTS` — idempotent and safe on existing databases.
- **Agent runner modal coupling** → Injecting the prompt picker into the switchboard modal requires reading from the store at modal open time. Risk: slow store reads blocking the TUI. Mitigation: load prompts asynchronously via `tea.Cmd` on modal init.

## Migration Plan

1. Deploy new `prompts` table migration — runs automatically on `store.Open()`, no manual step.
2. New CLI subcommand `orcai _promptmgr` registered in the same place as `orcai _cron`.
3. Jump window entry added to `internal/jumpwindow/jumpwindow.go`.
4. Agent runner modal change is additive — existing runs without a prompt continue to work identically.
5. Pipeline `prompt_id` field is `omitempty` — existing pipelines are unaffected.

No rollback complexity: the `prompts` table and CLI subcommand are purely additive.

## Open Questions

- Should the test runner support streaming output or wait for full completion before displaying? (Preference: streaming, matching chatui behaviour.)
- Should prompt titles be unique-enforced at the store level or just display a warning in the TUI?
- Should `prompt_id` in pipeline steps resolve at load time (eager) or at step execution time (lazy)?
