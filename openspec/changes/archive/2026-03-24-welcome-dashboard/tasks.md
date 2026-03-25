## 1. Data Types and Bus Connection

- [x] 1.1 In `internal/welcome/welcome.go`, add local `sessionTelemetry` struct: `WindowName`, `Provider`, `Status`, `InputTokens`, `OutputTokens`, `CostUSD float64`
- [x] 1.2 Add local `telemetryMsg` type carrying the parsed bus payload
- [x] 1.3 Add `connectBus() (pb.EventBusClient, *grpc.ClientConn)` helper — reads `~/.config/orcai/bus.addr` with 3-second retry, returns nil client on failure
- [x] 1.4 Add `subscribeCmd(conn *grpc.ClientConn) tea.Cmd` — subscribes to `orcai.telemetry`, returns one `telemetryMsg` per event received
- [x] 1.5 Add required imports: `proto/orcai/v1`, `google.golang.org/grpc`, `encoding/json`, `os`, `path/filepath`, `time`

## 2. Model Redesign

- [x] 2.1 Replace the `model` struct fields: remove `userArt`, `markdown`; add `sessions map[string]sessionTelemetry`, `windows []string` (ordered window names from tmux), `busConn *grpc.ClientConn`, `width int`, `height int`, `self string`
- [x] 2.2 Update `newModel()`: call `connectBus()`, call `listWindows()` (reuse sidebar's approach via `exec.Command("tmux", "list-windows", ...)`), initialise `sessions` map
- [x] 2.3 Add `tickMsg` type and `tickCmd()` returning a 5-second ticker
- [x] 2.4 Update `Init()` to return `tea.Batch(tickCmd(), subscribeCmd(m.busConn))` when bus is connected, or just `tickCmd()` otherwise

## 3. Update Handler

- [x] 3.1 In `Update()`, handle `tea.WindowSizeMsg` — update `m.width`, `m.height`
- [x] 3.2 Handle `tickMsg` — refresh window list via `listWindows()`, return `tickCmd()`
- [x] 3.3 Handle `telemetryMsg` — upsert into `m.sessions`, re-issue `subscribeCmd(m.busClient)`
- [x] 3.4 Handle `tea.KeyMsg`: `"q"` and `"ctrl+c"` → close bus conn and `tea.Quit`; `"enter"` → open picker popup (existing behaviour); all other keys → no-op (do NOT quit)

## 4. Dashboard View

- [x] 4.1 Keep `buildWelcomeArt(width int) string` unchanged as the header renderer
- [x] 4.2 Add `buildSessionCard(name string, st *sessionTelemetry, cardWidth int) string` — renders a `╔═╗ ║ ╚═╝` card with provider, status icon (● green / ○ dim), token counts, and cost; uses `"no data"` placeholder when `st == nil`
- [x] 4.3 Add `buildTotalsRow(sessions map[string]sessionTelemetry, width int) string` — sums input tokens, output tokens, cost; renders a single bordered row labelled `TOTAL`
- [x] 4.4 Rewrite `View()`:
  - Render banner via `buildWelcomeArt(w)`
  - Render a divider line
  - If no windows: render `"  no active sessions"` in dim blue
  - Else: render cards — two columns when `w >= 100`, otherwise stacked
  - Render totals row
  - Render footer: `"\x1b[38;5;61m  ^spc n new · ^spc p build · enter new session · q quit\x1b[0m"`
- [x] 4.5 Remove all glamour / `helpMarkdown` references and the `userArt` / `ansiart` usage

## 5. Lifecycle

- [x] 5.1 In `Run()`, remove the `execShell()` call that fires unconditionally after `p.Run()` — it should only fire when the user explicitly quits (handled via `tea.Quit` in Update)
- [x] 5.2 After `p.Run()` returns, call `execShell()` (the BubbleTea program only exits on q/ctrl+c, so this is safe)
- [x] 5.3 Remove unused imports (`glamour`, `ansiart`) after refactor

## 6. Verification

- [x] 6.1 Run `go build ./...` — no compilation errors
- [x] 6.2 Run `go test ./...` — all tests pass
- [x] 6.3 Commit: `feat(welcome): convert splash screen to live agent dashboard`
