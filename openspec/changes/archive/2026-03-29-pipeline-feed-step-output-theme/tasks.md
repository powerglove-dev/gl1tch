## 1. Fix `not_empty` condition in EvalCondition

- [x] 1.1 Add `case expr == "not_empty": return strings.TrimSpace(output) != ""` to the switch in `internal/pipeline/condition.go`
- [x] 1.2 Add table-driven test cases for `not_empty` in `internal/pipeline/condition_test.go` (non-empty value passes, empty string fails, whitespace-only fails)
- [ ] 1.3 Run `opencode-code-review` pipeline and confirm `assert-reviewed` passes when Ollama returns output

## 2. Theme-aware step colors — add `Warn` to `ANSIPalette`

- [x] 2.1 Add `Warn string` field to `ANSIPalette` struct in `internal/styles/styles.go`
- [x] 2.2 In `BundleANSI`, populate `Warn` using the bundle's `FG` color (nearest available to a "running" indicator)
- [x] 2.3 In the fallback `ansiPalette()` in `switchboard.go`, set `Warn: "\x1b[33m"` (preserve existing yellow fallback)
- [x] 2.4 Replace both occurrences of `col = aYlw` (case `"running"`) in `viewActivityFeed` and `feedRawLines` with `col = pal.Warn`

## 3. Populate `step.lines` from `StepDone` busd event

- [x] 3.1 Extend the `topics.StepDone` decode in `handlePipelineBusEvent` (`pipeline_bus.go`) to also parse `output.value` (string field)
- [x] 3.2 Split the value on newlines, trim whitespace, take the last 5 non-empty lines
- [x] 3.3 Add a helper `appendStepLines(entryID, stepID string, lines []string) Model` and call it after updating step status for `StepDone`
- [x] 3.4 Write a unit test in `pipeline_bus_test.go` (or `switchboard_test.go`) that sends a synthetic `StepDone` event with multi-line output and asserts `step.lines` is populated correctly

## 4. Inbox attention flag for failed runs

- [x] 4.1 In `item.Description()` (`internal/inbox/model.go`), prepend a `⚠` marker rendered with the error palette color when `run.ExitStatus != nil && *run.ExitStatus != 0`
- [x] 4.2 Confirm that the marker does not appear for successful or in-flight runs (manual smoke test or add to existing inbox tests)

## 5. Text wrapping for Activity Feed and Inbox detail

- [x] 5.1 In `viewActivityFeed` (`switchboard.go`), wrap output lines to fit within the panel width before appending to `allLines` (use `muesli/reflow/wrap` or manual word-wrap at `width-6`)
- [x] 5.2 Apply the same wrap to step output lines rendered beneath step badges
- [x] 5.3 In the inbox detail/modal view, wrap the description/output text to the modal width so it does not scroll off screen horizontally
