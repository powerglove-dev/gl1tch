## 1. Bootstrap: Leader Key + ESC Removal

- [x] 1.1 In `internal/bootstrap/bootstrap.go`, change `leaderBinding` from `bind-key -n \`` to `bind-key -n C-\\;`
- [x] 1.2 Remove the `escBinding` variable and its inclusion in the returned config string
- [x] 1.3 Update the `` orcai-chord `` `\`` self-help binding to use `C-\\;` instead of the backtick repeat
- [x] 1.4 Add a `t` chord binding: `bind-key -T orcai-chord t { switch-client -T root ; run-shell "<self> _sidebar-toggle" }`
- [x] 1.5 Run `go build ./...` to confirm no compilation errors

## 2. Bootstrap: Status Bar + Bus Address

- [x] 2.1 Update `status-right` in `buildTmuxConf` to include chord hints: `"#[fg=#6272a4] ^; n new  ^; p build   %H:%M "`
- [x] 2.2 Increase `status-right-length` to 40 to accommodate the longer right content
- [x] 2.3 In `bootstrap.Run()`, after `bus.Listen()` succeeds, write the returned address to `~/.config/orcai/bus.addr`
- [x] 2.4 Add a test in `bootstrap_test.go` that calls `buildTmuxConf` and asserts: `C-\;` leader present, backtick leader absent, ESC binding absent, status right contains `^; n new`
- [x] 2.5 Run `go test ./internal/bootstrap/...`

## 3. Bootstrap: Sidebar Toggle Subcommand

- [x] 3.1 In `main.go`, add a `_sidebar-toggle` case to the `switch os.Args[1]` block that calls `sidebar.RunToggle()`
- [x] 3.2 In `internal/sidebar/sidebar.go`, add `RunToggle()` function that reads `~/.config/orcai/.sidebar-visible`, inverts it, and runs the appropriate tmux command (`kill-pane` or `split-window`)
- [x] 3.3 Add a marker file helper: `sidebarVisiblePath() string` returning `~/.config/orcai/.sidebar-visible`
- [x] 3.4 `RunToggle()` writes the new state to the marker file after toggling

## 4. Telemetry Bus Event

- [x] 4.1 Add `TelemetryPayload` struct to `internal/chatui/messages.go`: fields `SessionID`, `Provider`, `Status` (string: "streaming"/"done"), `InputTokens`, `OutputTokens`, `CostUSD float64`
- [x] 4.2 Add `CostEstimate(provider, model string, inputTokens, outputTokens int) float64` function in `internal/chatui/` — hardcoded rates for `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-haiku-4-5` ($/MTok: opus 15/75, sonnet 3/15, haiku 0.25/1.25)
- [x] 4.3 Add a `busClient` field to chatui's model (or provider bridge) — a gRPC client connected to the bus addr read from `~/.config/orcai/bus.addr`
- [x] 4.4 When streaming begins: publish `TelemetryPayload{Status: "streaming"}` to topic `orcai.telemetry` via bus Publish RPC
- [x] 4.5 On `StreamDone`: compute cost via `CostEstimate`, publish `TelemetryPayload{Status: "done", InputTokens: ..., OutputTokens: ..., CostUSD: ...}` to `orcai.telemetry`
- [x] 4.6 Add unit tests for `CostEstimate` covering each model and zero-token edge case
- [x] 4.7 Run `go test ./internal/chatui/...`

## 5. Sidebar: Agent Context Panel

- [x] 5.1 Replace the `Model` struct in `internal/sidebar/sidebar.go` — remove `windows`, `cursor`, `manager` fields; add `sessions map[string]SessionTelemetry`, `windowOrder []string` (for stable render order), `busConn *grpc.ClientConn`, `visible bool`
- [x] 5.2 Add `SessionTelemetry` struct: `WindowName`, `Provider`, `Status`, `InputTokens`, `OutputTokens`, `CostUSD float64`
- [x] 5.3 In `New()`: read `~/.config/orcai/bus.addr` with 3-second retry (250ms sleep); connect gRPC client; start goroutine subscribing to `orcai.telemetry` and emitting `tea.Msg` on each event
- [x] 5.4 Keep `listWindows()` for populating `windowOrder` — the sidebar still knows which windows exist from tmux; telemetry is overlaid on top
- [x] 5.5 Update `Update()`: handle new `TelemetryMsg` message type (merge into `sessions` map); remove `tickMsg` respawn logic (no longer manages pane spawning); keep `j/k` navigation and `enter` window-focus, `x` kill-window
- [x] 5.6 Remove `n` (new session) and `p` (prompt builder) key handlers from sidebar — those are chord-only now
- [x] 5.7 Rewrite `View()` to render agent context panel: ORCAI banner, per-session rows showing `SessionTelemetry`, updated footer (no new/build hints)
- [x] 5.8 Remove `ensureSidebars()` and `spawnSidebar()` — toggle is handled by `RunToggle()` in bootstrap
- [x] 5.9 Add `RunToggle()` and marker file helpers (from task 3.2–3.4)
- [x] 5.10 Run `go test ./internal/sidebar/...`

## 6. Final Verification

- [x] 6.1 Run `go test ./...` — all tests pass
- [x] 6.2 Run `go build ./...` — no compilation errors
- [x] 6.3 Commit: `feat(ux): replace sidebar with agent context panel, ctrl+; leader, remove ESC capture`
