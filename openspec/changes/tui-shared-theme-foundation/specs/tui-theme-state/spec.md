## ADDED Requirements

### Requirement: ThemeState encapsulates theme init and live subscription
The `tuikit` package SHALL provide a `ThemeState` struct that any BubbleTea model can embed or hold as a field to get theme initialization and live cross-process theme updates without implementing subscription boilerplate.

#### Scenario: Model initializes ThemeState and gets theme cmd
- **WHEN** a BubbleTea model includes `ThemeState` and calls `themeState.Init()` in its `Init()` method
- **THEN** a subscription cmd is returned that connects to busd and waits for `theme.changed` events

#### Scenario: ThemeState.Handle processes ThemeChangedMsg
- **WHEN** a BubbleTea model calls `ts.Handle(msg)` in its `Update()` method and `msg` is a `ThemeChangedMsg`
- **THEN** `Handle` returns an updated `ThemeState` with the new bundle, a new subscription cmd, and `ok=true`
- **AND** the model re-renders using the new bundle

#### Scenario: ThemeState.Bundle returns active bundle
- **WHEN** `themeState.Bundle()` is called during view rendering
- **THEN** it returns the currently active `*themes.Bundle`, never nil when at least one bundled theme exists

### Requirement: ThemeState retries busd subscription on failure
If busd is not reachable when a subscription is attempted, `ThemeState` SHALL automatically retry after a backoff delay rather than silently losing the subscription.

#### Scenario: busd unavailable at startup, retry succeeds
- **WHEN** a sub-TUI starts and busd is not yet running
- **THEN** `ThemeState` retries the subscription every few seconds until busd becomes available
- **AND** once busd is reachable, subsequent theme changes are delivered normally

#### Scenario: Retry does not block TUI rendering
- **WHEN** busd is unavailable and ThemeState is retrying
- **THEN** the TUI continues to render and respond to input normally using its last-known theme

### Requirement: jumpwindow uses ThemeState for live theme updates
The `jumpwindow` package SHALL use `ThemeState` for theme management, replacing its frozen at-startup palette with live busd subscription.

#### Scenario: jumpwindow reflects theme switch from switchboard
- **WHEN** the user switches themes in switchboard while jumpwindow is open
- **THEN** jumpwindow re-renders with the new theme colors within 1 second

#### Scenario: jumpwindow starts with correct active theme
- **WHEN** jumpwindow opens
- **THEN** it displays the currently active theme (the same theme visible in switchboard)

### Requirement: crontui uses ThemeState uniformly
The `crontui` package SHALL use `ThemeState` for all theme management, removing the duplicate in-process-channel + manual-busd-subscription pattern.

#### Scenario: crontui reflects theme switch from switchboard
- **WHEN** the user switches themes in switchboard while crontui is open
- **THEN** crontui re-renders with the new theme within 1 second

### Requirement: crontui loads user-installed themes
The cron sub-TUI SHALL load themes from the user themes directory so that user-installed themes are available and can be applied when received via busd.

#### Scenario: User theme applied to crontui
- **WHEN** the user has a custom theme installed and selects it in switchboard
- **THEN** crontui renders using that custom theme bundle after receiving the busd event
