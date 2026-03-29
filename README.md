# ORCAI — the Agentic Bulletin Board System

ORCAI is a tmux-native AI workspace. It runs pipelines, coordinates AI agents, and keeps everything visible in one terminal session.

## Getting Started

### Launch

```
orcai
```

This creates (or reattaches to) your ORCAI session. Everything runs inside tmux — you can detach and reconnect anytime.

### The Switchboard

The **Switchboard** (window 0) is your control panel. It has four panels:

| Panel | What it shows |
|---|---|
| **Pipelines** | YAML pipelines you can launch |
| **Agent Runner** | AI agents you can start |
| **Signal Board** | Live status of running jobs |
| **Activity Feed** | Full output of every job |

### Navigation

| Key | Action |
|---|---|
| `tab` | Cycle focus between panels |
| `j` / `k` | Move selection up/down |
| `enter` | Launch / open selected item |
| `f` | Cycle Signal Board filter (all/running/done/failed) |
| `/` | Fuzzy search in Signal Board |
| `s` | Focus Signal Board |
| `p` | Focus Pipelines |
| `a` | Focus Agent Runner |
| `T` | Open theme picker |

### Chord Shortcuts

Press `^spc` (ctrl+space) then a key:

| Chord | Action |
|---|---|
| `^spc h` | This help screen |
| `^spc t` | Switch to Switchboard |
| `^spc m` | Theme picker |
| `^spc j` | Jump to any window |
| `^spc c` | New window |
| `^spc d` | Detach session |
| `^spc r` | Reload ORCAI (picks up new binary) |
| `^spc q` | Quit |
| `^spc [` / `]` | Previous / next window |
| `^spc x` / `X` | Kill pane / window |

### Pipelines

Pipelines live in `~/.config/orcai/pipelines/`. Each is a `.pipeline.yaml` file. Select one in the Pipelines panel and press `enter` to run it.

### Themes

Press `T` in the Switchboard or `^spc m` to open the theme picker. Themes live in `~/.config/orcai/themes/`.

Built-in themes: **Dracula**, **Nord**, **Catppuccin Mocha**, **Tokyo Night**, **Rose Piné**, **Solarized Dark**, **Kanagawa**.

### Reconnecting

```
orcai
```

If a session is already running, this reattaches. Your jobs keep running while detached.

## Releasing

Use the `/release` skill in Claude Code to cut a new release:

```
/release
```

The skill guides you through the full flow: branch → PR → merge to protected main → changelog curation → semver tag → GitHub Actions release build.

Requires: `gh` CLI authenticated, `goreleaser` installed, `task` installed.
