## 1. Fix tmux chord bindings in bootstrap.go

- [x] 1.1 Fix `^spc m` binding: when in `orcai-cron`, switch to orcai session and send `T` to `orcai:0` (same as non-cron branch) instead of sending `T` to the cron pane
- [x] 1.2 Fix `^spc q` binding: when in `orcai-cron`, switch to orcai session and send `C-q` to `orcai:0` (same as non-cron branch) instead of sending `q` to the cron pane

## 2. Remove quit state from crontui model

- [x] 2.1 Remove `quitConfirm bool` field from `internal/crontui/model.go`

## 3. Remove quit key handling from crontui update

- [x] 3.1 Remove the `"ctrl+c", "q"` case (lines 63–68) from `handleKey` in `internal/crontui/update.go`
- [x] 3.2 Remove the `if m.quitConfirm` overlay guard (lines 58–60) from `handleKey`
- [x] 3.3 Remove the `"esc"` → `quitConfirm = true` case (lines 92–94) from `handleJobPaneKey`
- [x] 3.4 Remove the `handleQuitConfirmKey` function entirely from `internal/crontui/update.go`

## 4. Remove quit UI from crontui view

- [x] 4.1 Remove the `if m.quitConfirm` block (around line 99) from `View()` in `internal/crontui/view.go`
- [x] 4.2 Remove the `viewQuitConfirm()` function from `internal/crontui/view.go`

## 5. Verify and build

- [x] 5.1 Confirm the project compiles with no errors (`go build ./...`)
- [x] 5.2 Manually test `^spc m` from the cron session opens the switchboard theme picker
- [x] 5.3 Confirm `q` and `esc` no longer trigger any quit behavior in the cron TUI
