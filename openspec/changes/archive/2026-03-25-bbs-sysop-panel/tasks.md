## 1. Activity Log

- [x] 1.1 Add `logEntry` struct to `internal/sidebar/sidebar.go`: fields `At time.Time`, `Node int` (1-based), `WindowName string`, `Event string` ("streaming"/"done"), `CostUSD float64`
- [x] 1.2 Add `log []logEntry` field to `Model` struct
- [x] 1.3 In `NewWithWindows()`, initialise `log` as empty slice
- [x] 1.4 In `Update()`, when a `TelemetryMsg` arrives: prepend a `logEntry` to `m.log`; cap slice at 12 entries (`m.log = m.log[:min(len(m.log), 12)]`)
- [x] 1.5 Node number lookup: derive node number from window index position in `m.windows` slice (1-based ordinal)

## 2. RunToggle: Per-Window Spawn

- [x] 2.1 In `RunToggle()`, read `TMUX_WINDOW_INDEX` env var; default to `"0"` if unset
- [x] 2.2 Change the marker file path to `~/.config/orcai/.panel-<windowIndex>` — update `sidebarVisiblePath()` to accept a `windowIndex string` argument
- [x] 2.3 Update `isSidebarVisible()` and `setSidebarVisible()` to pass the window index through
- [x] 2.4 Change the `tmux split-window` command: remove `-f` and `-t orcai:0`; change `-l 25%` to `-l 30%`; keep `-d -h -b`
- [x] 2.5 Update `sidebar_test.go`: add `TestRunToggleCmd` that verifies the spawn command does not contain `-t orcai:0` and uses `-l 30%`

## 3. View: Outer Border and Header

- [x] 3.1 Rewrite the top of `View()`: render `╔` + `═`×(w-2) + `╗` as the first line
- [x] 3.2 Render `║` + ` ▒▒▒ ORCAI SYSOP MONITOR ▒▒▒` (centred, padded to w-2) + `║` as the header line
- [x] 3.3 Render `║` + ` NODES: N ACTIVE  HH:MM` (padded to w-2) + `║` as the sub-header line (N = len(m.windows), time from `time.Now().Format("15:04")`)
- [x] 3.4 Render `╠` + `═`×(w-2) + `╣` as the divider after the header block
- [x] 3.5 Remove the old `dotsRow` and `bannerRow` / `═╢ ORCAI ╟═` rendering

## 4. View: Node Sections

- [x] 4.1 For each window (ordered by window index), compute its node number (1-based)
- [x] 4.2 Build `nodeHeader` line: `║ NODE XX [STATUS]` + padding + `║` — status badge coloured per state (BUSY=green, IDLE=dimT, WAIT=yellow); selected node uses `aSelBg` background on this line
- [x] 4.3 Build `providerLine`: `║   <windowName>  <provider>` or `║   <windowName>` + padding + `║`
- [x] 4.4 Build `metricsLine`: `║   <Nk↑ M↓ $C.CCC>` or `║   no data` + padding + `║`
- [x] 4.5 Append `╠` + `═`×(w-2) + `╣` divider between node sections (not after the last one)
- [x] 4.6 When no windows exist, render `║   no nodes active` + padding + `║`

## 5. View: Activity Log Section

- [x] 5.1 Render `╠` + `─`×(w-2) + `╣` thin divider before the log section
- [x] 5.2 Render `║ ── ACTIVITY LOG ──` (padded) + `║` as the log header
- [x] 5.3 For each `logEntry` in `m.log` (newest-first, up to 12): render `║  HH:MM NODE XX <event>[ $C.CCC]` + padding + `║`
- [x] 5.4 When `m.log` is empty, render `║  no activity` (dim) + padding + `║`

## 6. View: Footer and Close

- [x] 6.1 Render `║ enter focus · x kill · ↑↓ nav` (dim blue, padded) + `║` as the footer line
- [x] 6.2 Render `╚` + `═`×(w-2) + `╝` as the last line (closing border)
- [x] 6.3 Remove the old standalone `divider` + plain-text footer

## 7. Tests and Verification

- [x] 7.1 Update `TestViewContainsHeader` in `sidebar_test.go`: assert `╔` and `SYSOP MONITOR` present; assert old `═╢ ORCAI ╟═` absent
- [x] 7.2 Update `TestViewContainsActiveAccent` to look for `NODE 01` line with selection background escape
- [x] 7.3 Add `TestViewActivityLog`: send a `TelemetryMsg` update to the model, call `View()`, assert log section contains `ACTIVITY LOG` and the event entry
- [x] 7.4 Add `TestViewNodeStatus`: verify `[BUSY]` appears after streaming event, `[IDLE]` after done event, `[WAIT]` for no-data window
- [x] 7.5 Run `go test ./internal/sidebar/...`
- [x] 7.6 Run `go build ./...` — no compilation errors
- [x] 7.7 Commit: `feat(sidebar): BBS sysop panel with per-window toggle and activity log`
