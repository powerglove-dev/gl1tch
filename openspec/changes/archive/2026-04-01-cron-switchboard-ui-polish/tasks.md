## 1. Fix Switchboard Live Theme Preview

- [x] 1.1 Add `previewBundle *themes.Bundle` field to the switchboard `Model` struct; update `activeBundle()` to return `m.previewBundle` when non-nil (overrides registry lookup during picker navigation)
- [x] 1.2 In `internal/switchboard/switchboard.go:Update`, in the `themeState.Handle` branch, set `m.previewBundle = ts.Bundle()` when the theme picker is open, and `m.previewBundle = nil` (plus `m.registry.RefreshActive()`) when it is closed — this gives live preview without disk writes
- [x] 1.3 In `handleThemePicker`, when `close == true`, clear `m.previewBundle = nil` so the registry-backed active bundle takes over after picker exits

## 2. Add busd Topic Constant for Cron Entry Updates

- [x] 2.1 In `internal/busd/topics/topics.go`, add `CronEntryUpdated = "cron.entry.updated"` constant alongside the existing `CronJobStarted` / `CronJobCompleted` constants

## 3. Publish Rename Event from Cron TUI

- [x] 3.1 In `internal/crontui/update.go:confirmEdit`, after `cron.WriteEntry` succeeds and the name changed (`ov.original.Name != updated.Name`), publish a `cron.entry.updated` busd event with payload `{"old_name": <original>, "new_name": <updated>}` — use `_ =` to swallow errors so crontui works when no bus is connected

## 4. Handle Rename Event in Switchboard

- [x] 4.1 In `internal/switchboard/pipeline_bus.go`, handle `cron.entry.updated` events in `handlePipelineBusEvent` — unmarshal old/new name and update matching feed entries
- [x] 4.2 When a rename is received, scan `m.feed` entries whose title matches `"cron: " + old_name` and update to `"cron: " + new_name` (signal board reads from feed, so this covers both)
- [x] 4.3 Subscribe to `cron.entry.*` in the Switchboard's busd subscription in `pipeline_bus.go`

## 5. Extract Top Header Bar to panelrender (no duplication)

- [x] 5.1 In `internal/panelrender/panelrender.go`, add `TopBar(bundle *themes.Bundle, title string, width int) string`
- [x] 5.2 Add `TopBar` wrapper in `internal/switchboard/ansi_render.go`; refactor `viewTopBar` to delegate to it
- [x] 5.3 In `internal/crontui/view.go:View()`, call `panelrender.TopBar(m.bundle, "░▒▓ ORCAI — ABBS Cron ▓▒░", m.width)`, subtract 2 from height before `splitHeight`, and join as `lipgloss.JoinVertical(lipgloss.Left, topBar, "", top, bot)`

## 6. Add Padding Row Below Header in Switchboard

- [x] 6.1 In `internal/switchboard/switchboard.go` view assembly, append `"\n"` to `topBar` variable so all join sites below include one blank padding row between header and panels

## 7. Add "p pipeline" Shortcut to Cron TUI

- [x] 7.1 In `internal/crontui/update.go:handleJobPaneKey`, add `"p"` case: call `resolvePipelinePath(entry.Target)` which tries `~/.config/orcai/pipelines/<target>.yaml` variants
- [x] 7.2 Open resolved path with `tea.ExecProcess(exec.Command(editor, path), nil)` where editor falls back to `"vi"`
- [x] 7.3 In `internal/crontui/view.go:viewJobList`, add `{Key: "p", Desc: "pipeline"}` to the `jobHints` slice

## 8. Fix Inbox Warning Icon for Failed Runs

- [x] 8.1 Investigated: `buildInboxSection` in `switchboard.go` renders the inbox panel via its own ANSI path — it had the red dot for failures but no ⚠ attention indicator
- [x] 8.2 The store correctly receives `ExitStatus = 1` for failures; the missing piece was the ⚠ marker in `buildInboxSection`
- [x] 8.3 Added `warn` variable (`" ⚠"` in red) appended after the run name when `*ExitStatus != 0`, with `warnVis = 2` accounted in the padding calculation

## 9. Verification

- [x] 9.1 Build the binary (`go build ./...`) — passes with no errors
- [ ] 9.2 Manually test: open switchboard theme picker, navigate themes — panels should update in real time
- [ ] 9.3 Manually test: open cron TUI, confirm ORCAI header bar appears and padding row is present
- [ ] 9.4 Manually test: rename a cron entry in cron TUI, switch to switchboard — cron panel and signal board should show the new name immediately
- [ ] 9.5 Manually test: press `p` on a pipeline-kind cron entry — editor should open the pipeline file
- [ ] 9.6 Manually test: trigger a pipeline failure — inbox should show red dot and ⚠ needs attention

## 10. Pipeline YAML Name Sync on Edit

- [x] 10.1 In `confirmEdit`, after `cron.WriteEntry` and when `updated.Kind == "pipeline"` and name changed, call `updatePipelineYAMLName(updated.Target, updated.Name)`
- [x] 10.2 Add `updatePipelineYAMLName(target, newName string) error` helper in `update.go` — reads YAML with `yaml.Node` to preserve formatting, updates the `name` key, writes back
- [x] 10.3 Build passes (`go build ./...`)
