## 1. Activity Feed Scrolling

- [x] 1.1 Add `feedScrollOffset int` field to `Model` struct in `switchboard.go`
- [x] 1.2 In `viewActivityFeed()`, flatten all feed entries+lines into a single `[]string` slice, then slice `[feedScrollOffset : feedScrollOffset+visibleH]` before rendering
- [x] 1.3 Clamp `feedScrollOffset` to `[0, max(0, totalLines-visibleH)]` whenever it is modified or the window resizes
- [x] 1.4 Wire `↓` and `↑` keys to increment/decrement `feedScrollOffset` when feed has focus
- [x] 1.5 Reset `feedScrollOffset` to 0 whenever a new feed entry is appended (follow mode)
- [x] 1.6 Add tests in `switchboard_test.go` verifying feed lines are clamped to height and scroll offset shifts the visible window

## 2. Agent Runner Panel Fixed Height

- [x] 2.1 Add `agentScrollOffset int` to `agentSection` struct to track internal provider/model scroll position
- [x] 2.2 Define a constant `agentInnerHeight = 8` (inner rows budget for the agent box body)
- [x] 2.3 Refactor `buildAgentSection()` to render exactly `agentInnerHeight` body rows regardless of form step; pad with empty `boxRow` lines when content is shorter
- [x] 2.4 In the provider list (step 0) and model list (step 1) rendering, apply `agentScrollOffset` to slice the visible window; ensure selected item is always scrolled into view
- [x] 2.5 Reset `agentScrollOffset` when switching form steps
- [x] 2.6 Add test asserting `buildAgentSection()` always returns the same number of lines for step 0, 1, and 2

## 3. Signal Board

- [x] 3.1 Create `internal/switchboard/signal_board.go` with `SignalBoard` struct: `selectedIdx int`, `activeFilter string`, `blinkOn bool`
- [x] 3.2 Add `signalBoard SignalBoard` to `Model` struct; add `focusSignalBoard` to focus constants
- [x] 3.3 Implement `buildSignalBoard(height, width int) []string` — derive rows from `m.feed` at render time; respect `activeFilter`; render LED `●` with blink state from `m.signalBoard.blinkOn`
- [x] 3.4 Wire `tickMsg` to toggle `m.signalBoard.blinkOn` when any job has `FeedRunning` status
- [x] 3.5 In `View()` right column: render signal board above activity feed; allocate a fixed height (e.g. `min(len(m.feed)+2, 8)`) to the signal board panel
- [x] 3.6 Add `focusSignalBoard` to the `tab` focus rotation: launcher → agent → signalBoard → launcher
- [x] 3.7 Wire `↑`/`↓` in signal board focus to move `selectedIdx`; wire `f` to cycle `activeFilter` (all→running→done→failed→all)
- [x] 3.8 Add tests for `buildSignalBoard` filter behavior and blink state toggle

## 4. Tmux Hidden Windows

- [x] 4.1 Add `tmuxWindow string` field to `jobHandle` struct
- [x] 4.2 Add helper `createJobWindow(feedID string) (windowName string, err error)` that runs `tmux new-window -d -n orcai-<feedID>` and then `tmux set-window-option ... hide-from-statusbar on` (ignore error on second call)
- [x] 4.3 Call `createJobWindow(feedID)` in `handleEnter()` before starting the agent goroutine; store result in `m.activeJob.tmuxWindow`
- [x] 4.4 In the `FeedLineMsg` handler, if `m.activeJob.tmuxWindow != ""`, shell-escape the line and call `tmux send-keys -t <window> "<line>" Enter` via `exec.Command`
- [x] 4.5 Add a `currentTmuxSession() string` helper (reads `TMUX` env var or runs `tmux display-message -p '#S'`)
- [x] 4.6 Write a test for `createJobWindow` that skips if tmux is not available

## 5. Debug Popup

- [x] 5.1 Create `internal/switchboard/debug_popup.go` with `buildDebugPopup(height, width int, jobID string, feed []feedEntry) string` that renders an 80%-wide bordered overlay
- [x] 5.2 Add `debugPopupOpen bool` and `debugPopupJobID string` fields to `Model`
- [x] 5.3 In `buildDebugPopup`, call `tmux capture-pane -t orcai-<jobID> -p` via `exec.Command`; display output inside the popup; show "window closed or not available" if command fails
- [x] 5.4 In `View()`, when `debugPopupOpen` is true, overlay the popup centered horizontally over the normal layout
- [x] 5.5 In `Update()`, when `debugPopupOpen` is true: `esc` closes the popup; `enter` runs `tmux select-window -t orcai-<jobID>` and closes the popup
- [x] 5.6 Wire `enter` on the signal board to set `debugPopupOpen = true` and `debugPopupJobID` to the selected row's job ID
- [x] 5.7 Refresh popup content on each tick while open (re-run `tmux capture-pane` in a tea.Cmd)
- [x] 5.8 Add tests for popup overlay rendering and dismiss behavior

## 6. Integration and Cleanup

- [x] 6.1 Run `go build ./...` and fix any compile errors
- [x] 6.2 Run `go test ./internal/switchboard/...` and ensure all tests pass
- [x] 6.3 Run the full test suite `go test ./...` and fix any regressions
- [x] 6.4 Rebuild and install: `go build -o ~/.local/bin/orcai .`
- [x] 6.5 Manual smoke test: launch orcai, run an agent job, verify signal board shows the job, scroll the feed, open the debug popup, verify tmux window exists
