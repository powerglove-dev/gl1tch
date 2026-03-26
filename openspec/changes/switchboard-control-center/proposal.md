## Why

The sysop panel and welcome dashboard are two separate entry points with overlapping concerns — the welcome dashboard is a keybinding cheatsheet that launches other tools, and the sysop panel is a live session monitor. Neither provides a unified control surface for what matters most: launching pipelines, running agent providers with prompts, and watching work in progress. The prompt/pipeline builder is an additional TUI screen that duplicates pipeline authoring concerns but belongs in an editor, not an operations panel. The result is a fragmented UX where the shell is the de-facto way to start any actual work.

## What Changes

- **Remove** the pipeline/prompt builder entry point from the welcome dashboard keybindings (`p` key) and hide the `orcai pipeline build` subcommand from everyday promotion.
- **Merge** the sysop panel and welcome dashboard into a single full-screen BubbleTea TUI called the **Switchboard** (`orcai switchboard` / `orcai sysop`).
- **Switchboard layout**: full-screen three-region layout — left column (pipeline launcher + agent runner), center/main area (live run log / activity feed), bottom bar (keybinding shortcuts, replaces the standalone welcome dashboard cheatsheet).
- **Pipeline launcher**: lists saved pipelines from `~/.config/orcai/pipelines/`; selecting one runs it in-process with live output streamed to the activity feed.
- **Agent runner**: provider picker (sidecar-aware) + model selector + inline prompt input; submitting sends the prompt to the chosen provider and streams output to the activity feed.
- **Activity feed**: replaces the current session list; shows timestamped entries for pipeline runs and agent invocations, with status badges (running / done / failed).
- **Keybinding bar**: bottom strip showing the most important shortcuts — replaces the welcome dashboard's full-screen cheatsheet.
- The `orcai welcome` subcommand and `orcai-welcome` binary remain but now launch the switchboard instead of the old ANSI art dashboard.

## Capabilities

### New Capabilities

- `switchboard-tui`: Full-screen BubbleTea switchboard merging sysop + welcome; three-region layout (launcher column | activity feed | keybinding bar).
- `pipeline-launcher`: Browse and run saved pipelines from within the switchboard with live streamed output.
- `agent-runner`: Provider + model picker with inline prompt input that fires agent invocations from the switchboard.

### Modified Capabilities

- `welcome-dashboard`: The welcome dashboard is replaced by the switchboard; `orcai welcome` and `orcai-welcome` launch the switchboard instead of the standalone ANSI art screen.

## Impact

- `internal/sidebar/` — becomes the switchboard model; layout expands to include launcher column and activity feed.
- `internal/welcome/` — gutted; `orcai welcome` delegates to the switchboard.
- `internal/promptbuilder/` — entry point hidden from welcome; the package stays for `orcai pipeline build` but is not promoted.
- `cmd/sysop.go`, `cmd/welcome.go` — both point to the new switchboard.
- `cmd/orcai-welcome/main.go`, `cmd/orcai-sysop/main.go` — both launch the switchboard.
- `internal/picker/` — reused for agent runner's provider/model selection.
- `internal/pipeline/` — reused to run pipelines from within the switchboard.
