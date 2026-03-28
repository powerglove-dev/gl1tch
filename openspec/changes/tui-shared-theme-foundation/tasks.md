## 1. tuikit: ThemeState + retry

- [x] 1.1 Add `themeRetryMsg` type to `internal/tuikit/tuikit.go` carrying a `retryWait time.Duration`
- [x] 1.2 Update `ThemeSubscribeCmd` to return a `themeRetryMsg` (via `tea.Tick`) instead of nil when busd is unreachable — initial wait 2s, doubling up to 30s cap
- [x] 1.3 Add `ThemeState` struct to `internal/tuikit/tuikit.go` with fields: `bundle *themes.Bundle`, `retryWait time.Duration`
- [x] 1.4 Add `NewThemeState(bundle *themes.Bundle) ThemeState` constructor
- [x] 1.5 Add `ThemeState.Bundle() *themes.Bundle` accessor
- [x] 1.6 Add `ThemeState.Init() tea.Cmd` — returns `ThemeSubscribeCmd()` to start the subscription
- [x] 1.7 Add `ThemeState.Handle(msg tea.Msg) (ThemeState, tea.Cmd, bool)` — matches `ThemeChangedMsg` and `themeRetryMsg`, updates bundle, re-issues cmd; returns `ok=true` when consumed
- [x] 1.8 Update `ThemeState.Handle` to fall back to `GlobalRegistry().RefreshActive()` when `gr.Get(name)` misses
- [x] 1.9 Update unit tests in `tuikit_test.go` to cover retry behavior (busd unavailable → retry fires → eventually delivers event)

## 2. crontui: Replace manual subscription with ThemeState

- [x] 2.1 Add `themeState tuikit.ThemeState` field to `crontui.Model`
- [x] 2.2 In `crontui.New()`, initialize `themeState` with `tuikit.NewThemeState(bundle)` instead of manually calling `gr.SafeSubscribe()` + setting `themeCh`
- [x] 2.3 In `crontui.Init()`, replace `tuikit.ThemeSubscribeCmd()` with `m.themeState.Init()`; remove `listenTheme(m.themeCh)` cmd
- [x] 2.4 In `crontui.Update()`, add `ts, cmd, ok := m.themeState.Handle(msg)` before the switch; if `ok`, set `m.themeState = ts`, update `m.bundle = ts.Bundle()`, return with cmd
- [x] 2.5 Remove the manual `themeChangedMsg` type and `listenTheme` function from crontui (now handled by ThemeState)
- [x] 2.6 Remove `themeCh chan string` field from `crontui.Model` and all references

## 3. crontui: Fix missing user themes

- [x] 3.1 In `cmd/cmd_cron.go`, replace `themes.NewRegistry("")` with `themes.NewRegistry(filepath.Join(home, ".config", "orcai", "themes"))` (derive `home` from `os.UserHomeDir()`)

## 4. jumpwindow: Replace frozen palette with ThemeState

- [x] 4.1 Add `themeState tuikit.ThemeState` field to the `jumpwindow` model struct
- [x] 4.2 In jumpwindow initialization, replace `loadPalette()` call with `tuikit.NewThemeState(reg.Active())`; set `themeState` on the model
- [x] 4.3 Update jumpwindow's `Init()` to return `tea.Batch(textinput.Blink, m.themeState.Init())`
- [x] 4.4 In jumpwindow's `Update()`, add `ts, cmd, ok := m.themeState.Handle(msg)` at the top; if `ok`, set `m.themeState = ts`, return with cmd
- [x] 4.5 In jumpwindow's `View()`, derive all colors directly from `m.themeState.Bundle()` at render time — remove `jumpPalette` struct and `loadPalette()` function
- [x] 4.6 Remove the `jumpPalette` struct and `loadPalette()` function entirely

## 5. Validation

- [x] 5.1 Run `go build ./...` — no compilation errors
- [x] 5.2 Run `go test ./internal/tuikit/... ./internal/crontui/... ./internal/jumpwindow/...`
- [x] 5.3 Run full test suite: `go test ./...`
- [ ] 5.4 Manual smoke test: switch themes in switchboard — crontui updates within 1 second
- [ ] 5.5 Manual smoke test: switch themes in switchboard — jumpwindow updates on next open
- [ ] 5.6 Manual smoke test: launch `orcai cron` before switchboard; confirm retry connects and theme works once switchboard is up
