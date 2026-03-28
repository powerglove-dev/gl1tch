## Why

Every standalone sub-TUI (crontui, jumpwindow) and every new plugin or subcommand that renders a TUI currently has to independently solve three problems: load a theme at startup, subscribe to theme changes, and apply palette colors. None of them do it consistently. jumpwindow bakes a `jumpPalette` at startup and never updates it. crontui polls a file. The busd subscription introduced for crontui still has an edge case: if the daemon isn't reachable when the process starts, the subscription is silently lost forever. The result is that switching themes in switchboard leaves most of the session stale.

## What Changes

- **`tuikit.ThemeState`** — a small embeddable struct that any BubbleTea model can include to get correct theme initialization + live busd subscription with automatic retry. One call to `Init()` starts the subscription; one pattern match in `Update()` applies changes.
- **`tuikit.ThemeSubscribeCmd` retry** — if busd isn't available at subscription time, the cmd reschedules itself after a short backoff instead of silently returning nil and losing the subscription permanently.
- **jumpwindow wired to busd** — `jumpwindow` embeds `ThemeState`, subscribes on init, and re-renders on `ThemeChangedMsg` instead of baking a frozen palette at startup.
- **crontui palette path unified** — crontui's duplicate theme-update code (in-process channel + busd) is collapsed to use `ThemeState` uniformly.
- **New sub-TUIs/plugins get theme for free** — any future command that uses `tuikit.ThemeState` automatically inherits live theme updates without boilerplate.

## Capabilities

### New Capabilities

- `tui-theme-state`: A shared `ThemeState` struct in `internal/tuikit` that encapsulates theme init, busd subscription, retry on failure, and the `ThemeChangedMsg` update pattern — ready to embed in any BubbleTea model.

### Modified Capabilities

- `tui-theme-bus-subscriber`: The existing `ThemeSubscribeCmd` is updated to retry on busd unavailability rather than silently failing, and the cmd signature is simplified.

## Impact

- `internal/tuikit` — new `ThemeState` type and retry logic in `ThemeSubscribeCmd`
- `internal/jumpwindow` — embed `ThemeState`, remove frozen palette, subscribe to busd
- `internal/crontui` — replace dual-path (in-process channel + busd) with `ThemeState`
- `cmd/cmd_cron.go` — ensure registry is loaded with user dir (currently passes `""`, missing user themes)
- Any future sub-TUI plugin: use `ThemeState` from the start
