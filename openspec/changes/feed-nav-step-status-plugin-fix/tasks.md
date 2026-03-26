## 1. Bug Fix: Plugin Double-Registration

- [x] 1.1 In `buildProviders()` (`internal/picker/picker.go`), build a map from plugin name → `SidecarPath` using the `extras` slice collected from `discovery.Discover`
- [x] 1.2 After adding each static-provider entry to `out`, set `p.SidecarPath` from the extras map so `pipelineRunCmd`'s sidecar-skip guard fires correctly
- [x] 1.3 Add a test in `picker_test.go` that verifies ollama's `ProviderDef.SidecarPath` is non-empty when a wrappers YAML is present

## 2. Pipeline Runner: Structured Step-Status Logging

- [x] 2.1 Define a `StepStatusLine` constant (`"[step:%s] status:%s"`) in `internal/pipeline/runner.go` to stabilise the format
- [x] 2.2 In `runDAG`, emit `[step:<id>] status:running` to stdout immediately before calling `executor.Execute`
- [x] 2.3 In `runDAG`, emit `[step:<id>] status:done` on successful `Execute` return
- [x] 2.4 In `runDAG`, emit `[step:<id>] status:failed` after exhausting retries or on unrecoverable error
- [x] 2.5 Confirm `input` / `output` step types do not emit status lines
- [x] 2.6 Add unit test in `pipeline/runner_test.go` capturing stdout and asserting the expected `[step:*] status:*` lines appear in the right order

## 3. Switchboard: StepStatusMsg and Feed Entry Step List

- [x] 3.1 Add `StepInfo` struct (`id string`, `status string`) and a `steps []StepInfo` field to `feedEntry` in `switchboard.go`
- [x] 3.2 Add `StepStatusMsg` type (`FeedID`, `StepID`, `Status` string fields) to `switchboard.go`
- [x] 3.3 Write `parseStepStatus(line string) (stepID, status string, ok bool)` helper that matches the `[step:<id>] status:<state>` pattern
- [x] 3.4 In the log-watcher's line-dispatch loop, call `parseStepStatus`; send `StepStatusMsg` when matched, `FeedLineMsg` otherwise
- [x] 3.5 Handle `StepStatusMsg` in `Update()`: find the matching feed entry and update the step's status field
- [x] 3.6 When a pipeline run starts, load the pipeline YAML, extract non-input/output step IDs, and store them as `steps` (status `pending`) on the new `feedEntry`
- [x] 3.7 Add test coverage for `parseStepStatus` (valid, empty ID, empty state, non-matching)
- [x] 3.8 Add test for `StepStatusMsg` updating the correct feed entry's step status

## 4. Switchboard: Activity Feed Navigation

- [x] 4.1 Add `feedCursor int` field to `Model` in `switchboard.go`
- [x] 4.2 Include the Activity Feed in the Tab-cycle: after Agent Runner, Tab sets `feedFocused = true`; Tab from feed sets `launcher.focused = true`
- [x] 4.3 Handle `j` / down-arrow when `feedFocused`: increment `feedCursor`, clamped to visible line count
- [x] 4.4 Handle `k` / up-arrow when `feedFocused`: decrement `feedCursor`, clamped to 0
- [x] 4.5 Handle PgDn when `feedFocused`: advance `feedCursor` by feed panel height, clamped
- [x] 4.6 Handle PgUp when `feedFocused`: retreat `feedCursor` by feed panel height, clamped
- [x] 4.7 Handle `g` when `feedFocused`: set `feedCursor = 0`
- [x] 4.8 Handle `G` when `feedFocused`: set `feedCursor` to last visible line index
- [x] 4.9 In feed render, draw a `>` prefix (or highlight) on the line at `feedCursor` when `feedFocused == true`
- [x] 4.10 Add `FeedCursor() int` accessor (mirrors `FeedScrollOffset`) for test access

## 5. Switchboard: Step Status Rendering and Status Bar Hints

- [x] 5.1 In the feed entry render function, display step badges below the entry title in the format `  [running] fetch  [done] build  [pending] deploy`
- [x] 5.2 Use distinct colours for each badge state (running = yellow, done = green, failed = red, pending = dim)
- [x] 5.3 Update the status bar hint to show `↑↓ nav · PgUp/PgDn page · g/G top/bottom · enter open · tab focus` when `feedFocused == true`
- [x] 5.4 Verify default hint line is restored when focus leaves the feed

## 6. Tests and Cleanup

- [x] 6.1 Add switchboard test: Tab from Agent Runner focuses feed (`feedFocused == true`)
- [x] 6.2 Add switchboard test: Tab from feed wraps back to Pipeline Launcher
- [x] 6.3 Add switchboard test: `j` and `k` change `feedCursor` only when focused
- [x] 6.4 Add switchboard test: `g` sets cursor to 0, `G` sets cursor to last line
- [x] 6.5 Run full test suite (`go test ./...`) and confirm all pass
- [x] 6.6 Manually verify "ollama already registered" warning is gone from pipeline run output
