## Why

The Switchboard's pipeline and agent workflows currently lack key productivity shortcuts and visual affordances — new/edit/delete pipeline actions are missing, the agent runner form is cramped and inline, scroll position in the activity feed is not communicated, and navigation between panels requires too many keystrokes.

## What Changes

- **[n] New pipeline** — pressing `n` while pipelines panel is focused opens `$EDITOR` in a new tmux window shell so the user can author a pipeline YAML file; focus returns to the switchboard after the editor exits
- **[e] Edit pipeline** — pressing `e` on a selected pipeline opens that pipeline's YAML file in `$EDITOR` in a new tmux window (also reachable from the jump window); returns focus on exit
- **[d] Delete pipeline** — pressing `d` on a selected pipeline shows a confirmation modal (`[y]es / [n]o`) before removing the file from disk; cancels on any other key
- **[p] Pipelines focus shortcut** — pressing `p` from anywhere in the switchboard focuses the pipelines panel
- **Feed scroll indicators** — the activity feed box header shows `↑` / `↓` glyphs when content is scrollable above/below the visible window
- **Agent Runner popup modal** — the inline provider/model/prompt form steps are replaced by a centred overlay modal with ample real estate for writing prompts and selecting provider + model; activated by `enter` on the agent runner panel
- **Remove empty space** — the pipelines panel body no longer renders blank lines below the last pipeline entry

## Capabilities

### New Capabilities

- `pipeline-crud-keys`: Keyboard-driven pipeline CRUD — `n` (new), `e` (edit), `d` (delete with confirmation) — operating on the pipelines panel selection; editor integration via `$EDITOR` in a new tmux window
- `agent-runner-modal`: Full-screen overlay modal for agent runner that replaces the inline multi-step form; provides a textarea for prompt entry and selection lists for provider and model
- `feed-scroll-indicators`: Visual `↑`/`↓` scroll indicators rendered in the activity feed box border/header when off-screen content exists above or below

### Modified Capabilities

<!-- none -->

## Impact

- `internal/switchboard/switchboard.go` — key handling, model fields, view rendering
- `internal/switchboard/signal_board.go` — signal board integration (read-only, no changes expected)
- Pipeline YAML files on disk (`~/.config/orcai/pipelines/`) — `d` deletes, `n`/`e` open in editor
- tmux session — `n` and `e` open new windows via `tmux new-window -d -n <name> "$EDITOR <path>"`
- No new dependencies; uses existing `os/exec`, `os`, BubbleTea overlay pattern
