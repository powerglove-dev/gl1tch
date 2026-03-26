## Why

The Activity Feed is not keyboard-navigable — users cannot tab focus it or scroll through long output without opening a tmux window. Pipeline executions also produce no per-step visibility inside the feed, so operators can't tell which step is running or what its status is. A latent double-registration bug causes spurious `plugin "ollama" already registered` warnings on every pipeline run because `buildProviders` never copies the `SidecarPath` back onto static-provider entries, causing them to be registered twice.

## What Changes

- The Activity Feed gains tab focus and in-panel navigation (line up/down, page up/down) using a lightweight menu model inside the feed panel.
- Each feed entry is expanded to show its pipeline steps with live status indicators (`running`, `done`, `failed`, `pending`) updated in real time as the pipeline executes.
- `buildProviders()` is fixed to propagate `SidecarPath` from the discovery layer back onto static-provider entries (ollama, etc.) so `pipelineRunCmd`'s sidecar-skip guard works correctly and eliminates the duplicate-registration warning.

## Capabilities

### New Capabilities

- `activity-feed-navigation`: Tab focus support for the Activity Feed panel, with keyboard navigation (j/k arrow keys, PgUp/PgDn, g/G top/bottom) and a visible selection cursor, matching the existing focus model used by the Pipeline Launcher and Agent Runner panels.
- `pipeline-step-visibility`: Each feed entry expands inline to show the individual steps declared in the pipeline YAML alongside live status badges; step state is derived by parsing structured step-lifecycle log lines emitted by the pipeline runner.

### Modified Capabilities

- `pipeline-step-lifecycle`: The step executor must emit structured lifecycle log lines (`[step:<id>] status:<state>`) so the switchboard log-watcher can parse and surface per-step state in the feed.

## Impact

- `internal/switchboard/switchboard.go` — feed focus/navigation logic, feed entry rendering, step-status display
- `internal/picker/picker.go` — `buildProviders()`: copy discovered `SidecarPath` onto static ProviderDef entries
- `internal/pipeline/runner.go` (or equivalent) — emit structured step-lifecycle log lines
- `internal/switchboard/switchboard_test.go` — navigation and step-status test coverage
