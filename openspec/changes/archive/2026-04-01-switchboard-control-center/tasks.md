## 1. Package Scaffold — internal/switchboard

- [x] 1.1 Create `internal/switchboard/` directory; copy `internal/sidebar/sidebar.go` → `internal/switchboard/switchboard.go`, updating the package name to `switchboard`
- [x] 1.2 Copy `internal/sidebar/sidebar_test.go` → `internal/switchboard/switchboard_test.go`; update package references
- [x] 1.3 Add `feedEntry` struct: `id string`, `title string`, `status feedStatus` (running/done/failed), `ts time.Time`, `lines []string`; add ring-buffer `feed []feedEntry` (cap 200) and `selectedFeed int` to the switchboard `Model`
- [x] 1.4 Add `launcherSection` struct (pipelines []string, selected int, focused bool) and `quickRunSection` struct (formStep int, providers []picker.ProviderDef, selectedProvider int, selectedModel int, prompt textinput.Model, focused bool) to `Model`
- [x] 1.5 Add `activeJob *jobHandle` to `Model` (nil = idle); `jobHandle` holds `id string` and a cancel func; add `width`, `height int` to `Model`
- [x] 1.6 Verify `go build ./internal/switchboard/...` passes with zero errors

## 2. Layout — Three-Region View

- [x] 2.1 Implement `viewLeftColumn(m Model, height, width int) string`: renders BBS banner at top, then Pipeline Launcher section (list or placeholder), then Quick Run form — stacked vertically with lipgloss
- [x] 2.2 Implement `viewActivityFeed(m Model, height, width int) string`: renders `feedEntry` rows newest-first; selected entry shows all output lines; others show one summary line with status badge (`▶` / `✓` / `✗`)
- [x] 2.3 Implement `viewBottomBar(m Model, width int) string`: one-line strip `enter launch · tab focus · r refresh · q quit` in dim Dracula colors
- [x] 2.4 Wire `View()`: left column (30% width) joined horizontally with activity feed (remaining width); bottom bar below the joined columns
- [x] 2.5 Handle `tea.WindowSizeMsg`: store `width`/`height` on model; recompute column widths
- [x] 2.6 Run `go build ./internal/switchboard/...`; fix any compile errors before continuing

## 3. Pipeline Launcher

- [x] 3.1 Implement `scanPipelines(dir string) []string`: list `*.pipeline.yaml` basenames (minus extension) from the given dir; return nil if dir missing
- [x] 3.2 On switchboard `Init`, call `scanPipelines(~/.config/orcai/pipelines/)` and store in `m.launcher.pipelines`
- [x] 3.3 Handle `↑`/`↓`/`j`/`k` to move `launcher.selected` when launcher section is focused
- [x] 3.4 Implement `ChanPublisher`: a `pipeline.Publisher` whose methods send `feedLineMsg{id, text}` to a `chan tea.Msg`; check the Publisher interface in `internal/pipeline/runner.go` or `internal/pipeline/publisher.go`
- [x] 3.5 Implement `launchPipelineCmd(ctx context.Context, p *pipeline.Pipeline, mgr *plugin.Manager, feedID string, ch chan tea.Msg) tea.Cmd`: runs `pipeline.Run(ctx, p, mgr, "", pub)` in a goroutine; drains the ch into the bubbletea program via `tea.Batch`; sends `jobDoneMsg{feedID}` or `jobFailedMsg{feedID, err}` when done
- [x] 3.6 Implement `buildMgrForPipeline() *plugin.Manager`: creates a fresh `plugin.Manager`, registers providers via `picker.BuildProviders()` + `picker.PipelineLaunchArgs()`, then loads sidecar wrappers from `~/.config/orcai/wrappers/` — identical to what `cmd/pipeline.go` does today
- [x] 3.7 On `Enter` in launcher (idle only): open the YAML file, call `pipeline.Load`, build manager, create feed entry with `status: feedRunning`, dispatch `launchPipelineCmd`; set `activeJob`
- [x] 3.8 Handle `feedLineMsg`: find matching feed entry by id and append the line; enforce ring-buffer cap of 200 entries
- [x] 3.9 Handle `jobDoneMsg`: set feed entry status to `feedDone`; clear `activeJob`
- [x] 3.10 Handle `jobFailedMsg`: append error text to feed entry lines; set status to `feedFailed`; clear `activeJob`
- [x] 3.11 Show `[running]` badge in launcher section header when `activeJob != nil`; disable Enter on launcher when busy
- [x] 3.12 Handle `r` key when launcher is focused: re-scan pipelines dir and update `launcher.pipelines`

## 4. Quick Run (single-step pipeline)

- [x] 4.1 On switchboard `Init`, call `picker.BuildProviders()` and store in `m.quickRun.providers`
- [x] 4.2 Implement `viewQuickRun(m Model, width int) string`: step 0 renders provider list with selection highlight; step 1 renders model list (or skips to step 2 if no models); step 2 renders the textinput prompt
- [x] 4.3 Handle `Tab`/`→` in quick run: advance `formStep` (0→1→2); handle `←`/`Esc` to go back; at step 1, skip to step 2 if selected provider has no models
- [x] 4.4 Handle `↑`/`↓`/`j`/`k` at steps 0 and 1 to move provider/model selection
- [x] 4.5 On `Enter` at step 2 (prompt): build an in-memory `pipeline.Pipeline` with one step — `executor: <providerID>`, `model: <modelID>` (empty string if skipped), `prompt: promptInput.Value()`; use `buildMgrForPipeline()` as manager; add feed entry titled `<providerID>/<modelID>`; dispatch `launchPipelineCmd` (same function as saved pipelines)
- [x] 4.6 After dispatching: clear `promptInput`, reset `formStep` to 0
- [x] 4.7 Handle `r` key when quick run is focused: re-call `picker.BuildProviders()` and reset selections to 0
- [x] 4.8 Top-level `Tab` (when no sub-form is active) toggles focus between launcher and quick run sections

## 5. Entry Point Wiring

- [x] 5.1 Update all import paths in `cmd/` that reference `internal/sidebar` to `internal/switchboard`; the sidebar package stays in place but stops being imported
- [x] 5.2 Update `cmd/sysop.go` to import and call `switchboard.Run()`
- [x] 5.3 Rewrite `internal/welcome/welcome.go`: remove all ANSI art dashboard logic; replace with a thin package that exports `Run() error` calling `switchboard.Run()`
- [x] 5.4 Update `cmd/welcome.go` to call `welcome.Run()` (which delegates to switchboard)
- [x] 5.5 Update `cmd/orcai-sysop/main.go` to call `switchboard.Run()` directly
- [x] 5.6 Update `cmd/orcai-welcome/main.go` to call `switchboard.Run()` directly
- [x] 5.7 Remove the `p` keybinding (pipeline/prompt builder launch) from any remaining entry point
- [x] 5.8 Run `go build ./...` and fix all compile errors

## 6. Tests & Validation

- [x] 6.1 Write `TestScanPipelines` in `switchboard_test.go`: empty dir returns nil; populated dir returns basenames; missing dir returns nil
- [x] 6.2 Write `TestChanPublisher`: verify a publish call sends a `feedLineMsg` on the channel
- [x] 6.3 Write `TestLauncherNav`: `↓` increments selection; clamps at len-1; `↑` from 0 stays at 0
- [x] 6.4 Write `TestQuickRunSteps`: Tab at step 0 → step 1; Tab at step 1 → step 2; provider with no models skips from 0 to 2
- [x] 6.5 Run `go test ./internal/switchboard/...` — all tests pass
- [ ] 6.6 Run `orcai sysop` manually: switchboard opens with all three regions visible
- [ ] 6.7 Run `orcai welcome` manually: same switchboard opens (not the old ANSI art dashboard)
- [ ] 6.8 Launch a saved pipeline from the launcher: output streams to activity feed; finishes with `✓ done`
- [ ] 6.9 Use Quick Run to fire an agent: single-step pipeline runs and output appears in feed
