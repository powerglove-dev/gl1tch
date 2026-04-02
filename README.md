# GL1TCH — your AI, your terminal, your rules

```
  _____ _     __ _______ _____ _    _
 / ____| |   /_ |__   __/ ____| |  | |
| |  __| |    | |  | | | |    | |__| |
| | |_ | |    | |  | | | |    |  __  |
| |__| | |____| |  | | | |____| |  | |
 \_____|______|_|  |_|  \_____|_|  |_|
```

GL1TCH is a tmux-native AI workspace. It runs pipelines, coordinates AI agents, and keeps everything visible in one terminal session. Your GL1TCH is your own.

## Getting Started

### Launch

```
glitch
```

This creates (or reattaches to) your GL1TCH session. Everything runs inside tmux — you can detach and reconnect anytime.

### The Switchboard

The **Switchboard** (window 0) is your control panel. GL1TCH takes the full screen — talk to it to run pipelines, launch agents, check job status, or just get help.

### Navigation

| Key | Action |
|---|---|
| `tab` | Cycle focus between panels |
| `j` / `k` | Move selection up/down |
| `enter` | Launch / open selected item |
| `esc` | Back / close overlay |
| `T` | Open theme picker |

### Chord Shortcuts

Press `^spc` (ctrl+space) then a key:

| Chord | Action |
|---|---|
| `^spc h` | This help screen |
| `^spc t` | Switch to Switchboard |
| `^spc m` | Theme picker |
| `^spc c` | New window |
| `^spc d` | Detach session |
| `^spc r` | Reload GL1TCH (picks up new binary) |
| `^spc q` | Quit (tears down all tmux sessions) |
| `^spc [` / `]` | Previous / next window |
| `^spc x` / `X` | Kill pane / window |
| `^spc a` | Jump to GL1TCH assistant |

### Pipelines

Pipelines live in `~/.config/glitch/pipelines/`. Each is a `.pipeline.yaml` file.

To run a pipeline, use an explicit run-verb in the chat:

```
run backup
launch deploy every morning at 8am
execute sync-repos on https://github.com/org/repo
```

GL1TCH only dispatches pipelines when you explicitly ask. Questions, observations, and generic task requests (`review my PR`, `check the logs`) are handled by the AI directly — they never trigger an automated pipeline.

### Terminal Panes

The `/terminal` command manages inline tmux panes without leaving the Switchboard:

| Command | Action |
|---|---|
| `/terminal` | Open a new terminal pane (horizontal split) |
| `/terminal <cmd>` | Open a pane running `<cmd>` |
| `/terminal list` | List open terminal panes |
| `/terminal kill` | Kill the most recently opened terminal pane |
| `/terminal focus <n>` | Focus terminal pane `n` |
| `/terminal equalize` | Equalize all pane sizes |

### Themes

Press `T` in the Switchboard or `^spc m` to open the theme picker. Themes live in `~/.config/glitch/themes/`.

Built-in themes: **Dracula**, **Nord**, **Catppuccin Mocha**, **Tokyo Night**, **Rose Piné**, **Solarized Dark**, **Kanagawa**.

### Reconnecting

```
glitch
```

If a session is already running, this reattaches. Your jobs keep running while detached.

## Releasing

Use the `/release` skill in Claude Code to cut a new release:

```
/release
```

The skill guides you through the full flow: branch → PR → merge to protected main → changelog curation → semver tag → GitHub Actions release build.

Requires: `gh` CLI authenticated, `goreleaser` installed, `task` installed.
