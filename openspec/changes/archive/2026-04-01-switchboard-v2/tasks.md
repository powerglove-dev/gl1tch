## 1. Bootstrap — Keybindings & Status Bar

- [x] 1.1 In `internal/bootstrap/bootstrap.go`: remove the `^spc n` chord binding (`bind-key -T orcai-chord n`) entirely from the `chords` string
- [x] 1.2 Change the `^spc t` chord binding from `display-popup -E -w 100% -h 100% "<sysop>"` to `select-window -t orcai:0` so it focuses window 0 directly
- [x] 1.3 Update `status-right` to `"^spc t switchboard  ^spc c new-shell   %H:%M "` (remove `^spc n new`, `^spc p build`, `^spc x kill` references)
- [x] 1.4 Set `window-status-format ""` and `window-status-current-format ""` in the bootstrap tmux config to suppress the window list from the status bar
- [x] 1.5 Add a `before-kill-window` tmux hook that checks `#{window_index}` and `#{session_name}`; if the target is window 0 in session `orcai`, display a brief status message and cancel the kill
- [x] 1.6 Update the `^spc x` (kill-pane) and `^spc X` (kill-window) chord bindings to skip the action when the active window is window 0
- [x] 1.7 Run `go build ./internal/bootstrap/...` and verify no compile errors

## 2. Switchboard — Parallel Jobs

- [x] 2.1 In `internal/switchboard/switchboard.go`: replace the `activeJob *jobHandle` field on `Model` with `activeJobs map[string]*jobHandle`; initialize to `make(map[string]*jobHandle)` in `New()`
- [x] 2.2 Add a `maxParallelJobs int` constant (default 8) and enforce it: before launching a new job, check `len(m.activeJobs) >= maxParallelJobs`; if so, append a feed line warning and return without starting the job
- [x] 2.3 Update `launchPipelineCmd` and the quick-run launch path to store the new job in `m.activeJobs[feedID]` instead of `m.activeJob`
- [x] 2.4 Update `jobDoneMsg` and `jobFailedMsg` handlers to delete from `m.activeJobs[msg.id]` instead of clearing `m.activeJob`
- [x] 2.5 Remove any "busy" guard that checked `m.activeJob != nil` to block new launches (launcher Enter, quick-run Enter)
- [x] 2.6 Update the "running" badge in the launcher section header: show count `[N running]` based on `len(m.activeJobs)` instead of a simple nil check
- [x] 2.7 Update `drainChan` / tick logic: iterate over all `activeJobs` and drain each channel; use `tea.Batch` to collect all resulting commands
- [x] 2.8 Run `go build ./internal/switchboard/...` with no errors

## 3. Switchboard — Preview Popup

- [x] 3.1 Add `previewPopupOpen bool` and `previewPopupFeedID string` fields to `Model` (the existing `debugPopupOpen`/`debugPopupJobID` fields may be reused or kept separately for the debug window)
- [x] 3.2 In the activity feed key handler: when a feed entry is highlighted and the user presses Space, set `previewPopupOpen = true` and `previewPopupFeedID = entry.id`
- [x] 3.3 Implement `viewPreviewPopup(m Model) string`: read the last 30 lines from `entry.logFile`; render them inside a lipgloss border box at ~80% terminal width and ~60% height, centred over the Switchboard view
- [x] 3.4 In `View()`: if `previewPopupOpen`, overlay `viewPreviewPopup` on top of the normal view using lipgloss `Place`
- [x] 3.5 In the key handler: when `previewPopupOpen`, Esc closes the popup (`previewPopupOpen = false`); Enter closes the popup and calls `tmux select-window -t <entry.tmuxWindow>`
- [x] 3.6 Add a tick/poll message that refreshes the preview popup content while the job is running (reuse the existing blink tick or add a 500ms ticker)
- [x] 3.7 Ensure the preview popup is only activatable when the activity feed is focused and the selected entry has a non-empty `tmuxWindow`
- [x] 3.8 Run `go build ./internal/switchboard/...` with no errors

## 4. Ollama Plugin — Model Pull Guard

- [x] 4.1 In `../orca-plugins/plugins/ollama/main.go`: implement `isModelPresent(model string) bool` that runs `ollama list`, parses the output (one model name per line after the header), and returns true if the model is found
- [x] 4.2 Before calling `ollama pull` in the 404 error handler, first call `isModelPresent(model)`; if true, skip the pull and retry inference directly
- [x] 4.3 Also call `isModelPresent` proactively at startup (before the first inference attempt) and skip the pull entirely if the model is present
- [x] 4.4 Run `go test ./...` in `../orca-plugins/plugins/ollama/` — all existing tests pass

## 5. Opencode Plugin — Model Pull Guard

- [x] 5.1 In `../orca-plugins/plugins/opencode/main.go`: update `pullOllamaModel` to call `ollama list` first; parse output to check if the bare model name (after stripping `ollama/`) is present; return nil immediately if found
- [x] 5.2 Run `go test ./...` in `../orca-plugins/plugins/opencode/` — all existing tests pass

## 6. Integration Tests

- [x] 6.1 Create `internal/pipeline/integration_test.go` with `//go:build integration` build tag
- [x] 6.2 Write `TestPipelineFullRun_Llama` and `TestPipelineFullRun_Qwen`: each loads a test fixture YAML pipeline (`testdata/llama_pipeline.yaml` and `testdata/qwen_pipeline.yaml`), calls `pipeline.Run`, and asserts nil error + non-empty publisher output; call `t.Skip` if the model is not in `ollama list`
- [x] 6.3 Create `testdata/llama_pipeline.yaml`: single-step pipeline with `executor: ollama`, `model: llama3.2`, `prompt: "Say hello in one sentence."`
- [x] 6.4 Create `testdata/qwen_pipeline.yaml`: single-step pipeline with `executor: ollama`, `model: qwen2.5`, `prompt: "Say hello in one sentence."`
- [x] 6.5 Write `TestAgentSingleStep_Llama` and `TestAgentSingleStep_Qwen`: construct an in-memory single-step `pipeline.Pipeline` (no YAML file), run it, assert nil error + non-empty output; skip if model absent
- [x] 6.6 Verify `go test ./internal/pipeline/...` (without `-tags integration`) does not compile or run the integration tests
- [x] 6.7 Verify `go test -tags integration ./internal/pipeline/...` runs the integration tests (or skips cleanly if models are absent)

## 7. Tests & Validation

- [x] 7.1 Write `TestParallelJobs` in `switchboard_test.go`: simulate launching two jobs back-to-back; assert both are present in `m.activeJobs` and feed has two `FeedRunning` entries
- [x] 7.2 Write `TestParallelJobCap`: simulate launching `maxParallelJobs + 1` jobs; assert the last launch is blocked and a warning feed line is added
- [x] 7.3 Write `TestPreviewPopupOpen`: send a Space key with a feed entry highlighted; assert `m.previewPopupOpen == true` and `m.previewPopupFeedID == entry.id`
- [x] 7.4 Write `TestPreviewPopupEscCloses`: with popup open, send Esc; assert popup is closed
- [x] 7.5 Run `go test ./internal/switchboard/...` — all tests pass
- [x] 7.6 Run `go test ./internal/bootstrap/...` (if tests exist) — all pass; otherwise do a manual smoke test of `orcai start` to verify status bar and keybindings are correct
- [ ] 7.7 Manual: launch two pipelines from the Switchboard simultaneously; confirm both appear as `running` with separate tmux windows
- [ ] 7.8 Manual: open preview popup on a running job with Space; confirm live log tail appears; press Enter to navigate into the background window
- [ ] 7.9 Manual: press `^spc t` from a background window; confirm focus returns to window 0 (Switchboard), not a popup
- [ ] 7.10 Manual: attempt `^spc X` on window 0; confirm it is blocked and a status message is shown
