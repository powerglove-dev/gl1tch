## 1. Store — prompts table and CRUD

- [x] 1.1 Add `prompts` table DDL constant to `internal/store/schema.go` and wire it into `applySchema`
- [x] 1.2 Define `Prompt` struct in `internal/store/` with fields: `ID`, `Title`, `Body`, `ModelSlug`, `CreatedAt`, `UpdatedAt`
- [x] 1.3 Implement `InsertPrompt(ctx, Prompt) (int64, error)` in `internal/store/query.go`
- [x] 1.4 Implement `UpdatePrompt(ctx, Prompt) error`
- [x] 1.5 Implement `DeletePrompt(ctx, id int64) error`
- [x] 1.6 Implement `ListPrompts(ctx) ([]Prompt, error)` ordered by `updated_at DESC`
- [x] 1.7 Implement `SearchPrompts(ctx, query string) ([]Prompt, error)` using case-insensitive LIKE on title and body
- [x] 1.8 Implement `GetPrompt(ctx, id int64) (Prompt, error)`
- [x] 1.9 Write table-driven tests for all store methods in `internal/store/store_test.go`

## 2. Prompt manager TUI — package skeleton

- [x] 2.1 Create `internal/promptmgr/` package with `model.go`, `view.go`, `update.go`, `keys.go`
- [x] 2.2 Define `Model` struct with store reference, prompt list, filtered list, selected index, search input, editor fields (title textarea, body textarea, model selector), test runner viewport and streaming state
- [x] 2.3 Implement `New(store *store.Store, pluginMgr *plugin.Manager) *Model` constructor seeding `ThemeState`
- [x] 2.4 Define keybindings in `keys.go`: `n` new, `e`/`enter` edit, `d` delete, `ctrl+s` save, `ctrl+r` run, `tab`/`shift+tab` focus cycle, `j`/`k`/`↑`/`↓` navigate, `q`/`esc` quit/close

## 3. Prompt manager TUI — list panel

- [x] 3.1 Implement left-panel `viewList()` rendering prompt rows with title and model slug, cursor highlight using accent background
- [x] 3.2 Implement fuzzy search input (using `sahilm/fuzzy`) with real-time list filtering
- [x] 3.3 Implement scroll/clamp logic for the list when prompts exceed panel height
- [x] 3.4 Handle `n`, `j`/`k`, `↑`/`↓`, `d` (with confirmation overlay) key messages in `Update`

## 4. Prompt manager TUI — editor panel

- [x] 4.1 Implement right-top `viewEditor()` with title input, body textarea, model selector (cycling or dropdown), and CWD field with fuzzy dir picker (same pattern as agent runner CWD picker)
- [x] 4.2 Load available model slugs from the plugin registry on init via `tea.Cmd`
- [x] 4.3 Implement `ctrl+s` save: call `InsertPrompt` or `UpdatePrompt` depending on whether the prompt has an ID; refresh list
- [x] 4.4 Implement `e`/`enter` to populate editor fields from the selected list prompt

## 5. Prompt manager TUI — test runner panel

- [x] 5.0 Add `last_response TEXT DEFAULT ''` column migration to `internal/store/schema.go`; add `LastResponse string` field to `Prompt` struct; implement `SavePromptResponse(ctx, id int64, response string) error` in `internal/store/prompt.go`; scan `last_response` in `GetPrompt` and list methods; add tests
- [x] 5.0b Add `cwd TEXT DEFAULT ''` column migration (same pattern as last_response); add `CWD string` field to `Prompt` struct; update `InsertPrompt`/`UpdatePrompt` to store/retrieve cwd; update all scan sites; add test for cwd round-trip
- [x] 5.1 Implement right-bottom `viewTestRunner()` with a scrollable viewport and spinner while streaming; pre-populate with `LastResponse` when a saved prompt is opened
- [x] 5.2 Implement `ctrl+r` to invoke the selected model with the current editor body via a `tea.Cmd` that streams tokens as `tea.Msg`
- [x] 5.3 Handle streaming token messages: append to viewport content, trigger re-render
- [x] 5.4 Implement `ctrl+c`/`esc` cancellation of in-progress run (context cancellation); do NOT overwrite saved response on cancel
- [x] 5.5 Implement `↑`/`↓` scroll in test runner viewport when the runner panel is focused
- [x] 5.6 On run completion, call `SavePromptResponse` to persist the full response if the prompt has a saved ID; skip persist if unsaved

## 6. CLI subcommand and jump window integration

- [x] 6.1 Register `orcai _promptmgr` CLI subcommand (same pattern as `orcai _cron`) that opens the prompt manager as a standalone BubbleTea program
- [x] 6.2 Add `promptsWindow bool` field to `window` struct in `internal/jumpwindow/jumpwindow.go`
- [x] 6.3 Append synthetic "prompts" entry to sysop list in `newModel()`
- [x] 6.4 Handle `promptsWindow` case in the jump window `Update` to run `orcai _promptmgr` in a new/existing tmux window named `orcai-prompts`

## 7. Agent runner — prompt pre-selection

- [x] 7.1 Add prompt picker field to the switchboard agent runner modal (load prompt list via `tea.Cmd` on modal init)
- [x] 7.2 Render the prompt picker as a searchable dropdown or cycling selector above existing fields
- [x] 7.3 Pass selected `prompt_id` (or resolved body) to the agent runner CLI invocation
- [x] 7.4 Ensure empty prompt selection leaves agent run behaviour unchanged

## 8. Pipeline — prompt_id field

- [x] 8.1 Add `PromptID string \`yaml:"prompt_id,omitempty"\`` to `pipeline.Step` struct
- [x] 8.2 In the pipeline executor, resolve `prompt_id` from the store before step execution and prepend the body to the step input
- [x] 8.3 Return a descriptive error if `prompt_id` is set but the prompt is not found in the store
- [x] 8.4 Verify existing pipeline YAML without `prompt_id` parses and runs without change (regression test)

## 9. Tests and polish

- [x] 9.1 Write BubbleTea model tests for `internal/promptmgr/` covering list navigation, search filter, save, delete confirmation
- [x] 9.2 Write test for test runner streaming and cancellation (use mock model invoker)
- [x] 9.3 Verify theme inheritance — render with a non-default theme and assert accent colours in output
- [x] 9.4 Run `go build ./...` and `go test ./...` to confirm no regressions
