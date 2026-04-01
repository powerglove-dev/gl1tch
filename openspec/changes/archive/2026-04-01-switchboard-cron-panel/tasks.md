## 1. Internal Cron Package — NextRun Helper

- [x] 1.1 Add `NextRun(entry Entry) (time.Time, error)` helper to `internal/cron/scheduler.go` that parses the entry's schedule expression with robfig/cron and returns the next fire time after `time.Now()`
- [x] 1.2 Add `FormatRelative(t time.Time) string` helper in `internal/cron/` that returns a human-readable relative duration string (e.g. "in 4m", "in 2h 30m") for use in both the switchboard panel and the TUI

## 2. Switchboard Cron Panel

- [x] 2.1 Add `CronPanel` state struct to `internal/switchboard/switchboard.go` (or a new `cron_panel.go` file) tracking selected index and scroll offset
- [x] 2.2 Implement `buildCronSection(w int) []string` that calls `cron.LoadConfig()` + `NextRun` for each entry, sorts by next-run ascending, and renders a compact ANSI box using `PanelHeader(bundle, "cron", w)`
- [x] 2.3 Wire `c` keybinding in `handleKey` (global shortcuts block) to focus the Cron panel, consistent with the `p`/`a`/`s`/`i` pattern
- [x] 2.4 Implement `m` keybinding inside the Cron panel focus block to call `ensureCronDaemon()` and switch the active tmux window to `orcai-cron` (using `tmux switch-client -t orcai-cron`)
- [x] 2.5 Add `c cron` entry to `viewBottomBar` hint strip alongside existing panel shortcuts
- [x] 2.6 Include the Cron panel in the `View()` layout between Inbox and the bottom of the right column

## 3. orcai-cron TUI Binary

- [x] 3.1 Create `cmd/orcai-cron/` directory with `main.go` that initialises a BubbleTea program and starts `orcai cron tui`
- [x] 3.2 Add `cronTuiCmd` to `cmd/cmd_cron.go` as a hidden subcommand (`orcai cron tui`) that runs the BubbleTea TUI
- [x] 3.3 Define `TuiModel` struct with fields: `scheduler *cron.Scheduler`, `entries []cron.Entry`, `logBuf []string` (ring buffer, cap 500), `filterInput textinput.Model`, `selectedIdx int`, `scrollOffset int`, `logScrollOffset int`, `activePane int` (0=jobs, 1=logs), `editOverlay *EditOverlay`, `deleteConfirm *DeleteConfirm`, `width/height int`, `styles TuiStyles`
- [x] 3.4 Implement `TuiStyles` that loads the active ABBS bundle via the switchboard bundle loader and maps palette colors to lipgloss styles for border, selection highlight, status indicators, and log-level colors; fall back to Dracula defaults when no bundle is active
- [x] 3.5 Implement `Init()` — start the scheduler, emit an initial `tickMsg` for refreshing next-run times every 30s

## 4. TUI — Job List Pane

- [x] 4.1 Implement `viewJobList(width, height int) string` that renders the top pane with columns: name, kind, schedule, next-run (relative), last-run status
- [x] 4.2 Implement fuzzy filtering: on each keystroke to the filter input, run `fuzzy.FindFrom` over entry names and update the displayed list
- [x] 4.3 Add `j`/`k` (and arrow key) navigation for the job list with scroll clamping
- [x] 4.4 Render empty state placeholder when `entries` is empty or filter matches nothing
- [x] 4.5 Highlight the selected row with the active bundle's selection color

## 5. TUI — Log Pane

- [x] 5.1 Implement `LogSink` struct that satisfies `io.Writer`, writes each line to a channel, and is passed as the output target to `charmbracelet/log` logger
- [x] 5.2 Configure the `charmbracelet/log` logger with a `job` prefix field; each `runEntry` call sets the field to `entry.Name` before writing
- [x] 5.3 Implement `viewLogPane(width, height int) string` that renders the bottom 40% with the last N lines from the ring buffer, auto-scrolling to the tail
- [x] 5.4 Handle `logLineMsg` in `Update()`: append to ring buffer, drop oldest when > 500, trigger re-render
- [x] 5.5 Support `j`/`k` scrolling in the log pane when it has focus (via `tab` to switch active pane)

## 6. TUI — Edit Overlay

- [x] 6.1 Define `EditOverlay` struct with five `textinput.Model` fields (name, schedule, kind, target, timeout) and a `focusIdx int`
- [x] 6.2 Implement `viewEditOverlay(width, height int) string` that renders the overlay centred over the job list
- [x] 6.3 Handle `tab`/`shift+tab` to move focus between fields, `enter` on last field to confirm
- [x] 6.4 On confirm: validate schedule with robfig/cron parser; show inline error on failure; on success call `cron.WriteEntry` (replace by name) and reload the scheduler
- [x] 6.5 Handle `esc` to close the overlay without saving

## 7. TUI — Delete Confirmation

- [x] 7.1 Define `DeleteConfirm` struct holding the entry to delete and a `confirmed bool`
- [x] 7.2 Implement `viewDeleteConfirm(width int) string` that renders a one-line prompt (e.g. `delete "my-job"? [y/N]`)
- [x] 7.3 Handle `y`/`Y` to remove the entry from `cron.yaml` (via a `cron.DeleteEntry` helper) and reload the scheduler; handle `n`/`esc` to cancel

## 8. TUI — Run-Now Action

- [x] 8.1 Handle `enter` (and `r`) in the job list pane to fire the selected entry immediately via a `tea.Cmd` that runs `scheduler.RunNow(entry)` asynchronously
- [x] 8.2 Add `RunNow(entry Entry)` method to `cron.Scheduler` that executes `runEntry` in a goroutine outside the cron schedule
- [x] 8.3 Emit run output as `logLineMsg` events so the log pane shows the run-now output prefixed with the job name

## 9. Update `orcai cron start`

- [x] 9.1 Update `cronStartCmd` in `cmd/cmd_cron.go` to pass `cron tui` as the tmux session command instead of `cron run`
- [x] 9.2 Keep `cron run` functional as a fallback headless command (no change to its implementation)
- [x] 9.3 Verify that `ensureCronDaemon()` in `switchboard.go` invokes the correct command after this change

## 10. Internal Cron Package — Delete Helper

- [x] 10.1 Add `DeleteEntry(name string) error` to `internal/cron/writer.go` that reads `cron.yaml`, removes the entry with the matching name, and writes the file back atomically

## 11. Tests

- [x] 11.1 Unit test `NextRun` and `FormatRelative` in `internal/cron/`
- [x] 11.2 Unit test `DeleteEntry` in `internal/cron/writer_test.go`
- [x] 11.3 Unit test `buildCronSection` renders expected rows and empty-state message
- [x] 11.4 BubbleTea model test: `c` keypress moves focus to Cron panel
- [x] 11.5 BubbleTea model test: `m` keypress with mocked tmux command triggers switch-client
