## Context

After the `shared-tui-components-theme-signals` change:

- `tuikit.ThemeSubscribeCmd` exists and works when busd is running.
- `crontui` uses it but has a silent failure mode: if busd is unreachable at startup, the cmd returns nil and the subscription is permanently lost.
- `jumpwindow` has no subscription at all. It calls `themes.NewRegistry(userThemesDir)` in a palette-loader function that runs once at startup — the resulting `jumpPalette` is frozen for the lifetime of the process.
- `cmd_cron.go` calls `themes.NewRegistry("")` (no user themes dir), so user-installed themes aren't visible to crontui even if the busd event delivers them.
- Every new sub-TUI will need to hand-roll the same init + subscribe + update boilerplate.

## Goals / Non-Goals

**Goals:**
- `ThemeState` struct in `tuikit` that any model can embed (or hold as a field) to get theme init + live subscription in one place.
- Retry busd subscription on failure so a momentary unavailability doesn't permanently kill the subscription.
- Wire jumpwindow to busd via `ThemeState`.
- Simplify crontui's duplicate in-process/busd paths to use `ThemeState`.
- Fix crontui's missing user themes dir.

**Non-Goals:**
- Consolidating all UI rendering (modals, headers) into tuikit — that's `panelrender`'s job.
- Changing how switchboard handles themes internally (it's in-process, works fine).
- Plugin API changes — plugins get this for free once they use `ThemeState`.

## Decisions

### D1: ThemeState as an embeddable value (not an interface)

**Decision**: `ThemeState` is a plain struct with a `Bundle() *themes.Bundle` accessor and two methods: `Init() tea.Cmd` (starts subscription) and `Handle(tea.Msg) (ThemeState, tea.Cmd, bool)` (returns updated state, next cmd, and whether the msg was consumed).

```go
type ThemeState struct {
    bundle    *themes.Bundle
    retryWait time.Duration
}

func NewThemeState(bundle *themes.Bundle) ThemeState { ... }
func (ts ThemeState) Bundle() *themes.Bundle { ... }
func (ts ThemeState) Init() tea.Cmd { ... }
func (ts ThemeState) Handle(msg tea.Msg) (ThemeState, tea.Cmd, bool) { ... }
```

**Rationale**: Embedding a struct is simpler than interface dispatch. The `Handle` pattern lets each model call `ts.Handle(msg)` before its own switch — if `ok` is true, the msg was a `ThemeChangedMsg` and the caller just returns the updated model. Models that need custom logic on theme change can check `ok` and add their own code after.

**Alternative**: A `ThemeManager` with callbacks — more flexible but more complex; unnecessary for now.

### D2: Retry with backoff on busd unavailability

**Decision**: When `ThemeSubscribeCmd` fails to dial busd, instead of returning nil it returns a `themeRetryMsg` after a short delay (2s, doubling up to 30s cap). `ThemeState.Handle` processes this by re-issuing the subscription cmd.

**Rationale**: The orcai session may start crontui before the busd daemon has fully started. Silent failure causes a broken subscription that's invisible to the user. A retry loop is cheap (just a `time.Sleep`) and self-healing.

**Alternative**: Check busd availability at startup and wait — couples the TUI startup to busd liveness; bad UX if busd never starts.

### D3: jumpwindow gets a `ThemeState` field, replaces frozen palette

**Decision**: Add `themeState tuikit.ThemeState` to the jumpwindow model. In `Init()`, start `themeState.Init()`. In `Update()`, call `ts.Handle(msg)` first. In `View()`, derive all colors from `themeState.Bundle()` at render time (not from a pre-baked `jumpPalette` struct).

**Rationale**: This is consistent with crontui. The frozen `jumpPalette` can be removed; color derivation at render time is fast (it's just field reads on a struct).

**Alternative**: Keep `jumpPalette` but refresh it on `ThemeChangedMsg` — two parallel color representations; confusing and error-prone.

### D4: crontui's in-process channel path kept but secondary

**Decision**: The `themeCh` / `listenTheme` path stays for the embedded case (when crontui is embedded in-process and `GlobalRegistry` is set). `ThemeState` handles the busd path. Both paths update `m.bundle` via the same `ThemeChangedMsg` / `ThemeState` mechanism.

**Rationale**: When crontui is embedded (rare), the in-process path is faster and doesn't require busd. Removing it is safe but not necessary.

### D5: Fix crontui's missing user themes

**Decision**: `cmd_cron.go` passes `filepath.Join(home, ".config", "orcai", "themes")` to `NewRegistry()` instead of `""`.

**Rationale**: This is a simple bug — user themes aren't visible to the cron TUI. Fixing it is one line.

## Risks / Trade-offs

- **Retry goroutine leak**: The retry `time.Sleep` runs inside a `tea.Cmd` goroutine — BubbleTea manages these goroutines, so there's no leak risk.
- **jumpwindow render-time color derivation is slightly more work per frame**: Negligible — it's field reads, not file I/O.
- **ThemeState.Handle consumes ThemeChangedMsg**: If a model needs to react to theme changes beyond what ThemeState does, it checks `ok` and adds logic after — this is explicit and clear.

## Migration Plan

1. Add `ThemeState`, `themeRetryMsg`, and retry logic to `internal/tuikit`.
2. Wire `ThemeState` into `crontui` — replace the manual `tuikit.ThemeSubscribeCmd()` calls.
3. Wire `ThemeState` into `jumpwindow` — remove frozen palette, derive colors at render time.
4. Fix `cmd_cron.go` user themes dir.
5. Run tests and smoke test live theme switching.

## Open Questions

- Should `ThemeState.Handle` also call `GlobalRegistry.RefreshActive()` as a fallback when `gr.Get(msg.Name)` fails? Yes — keeps the file-based path as a last resort if the registry doesn't have the bundle by name.
