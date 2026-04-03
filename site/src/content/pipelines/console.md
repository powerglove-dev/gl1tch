---
title: "Console"
description: "Chat with your AI assistant, launch pipelines, and watch your automations run — all from one workspace."
order: 2
---

Your gl1tch workspace is a single terminal screen that brings together everything: a conversation with your AI assistant, a list of your saved pipelines, and a live feed of everything that's running. Open it by launching gl1tch, then talk to it like a person or type a slash command to take action.


## Quick Start

Launch gl1tch and open your workspace:

```bash
glitch
```

Your workspace opens with three regions:

```text
┌─────────────────────────────────────────────────────────────┐
│ GL1TCH DECK — CONTROL PANEL                         hh:mm:ss │
├──────────────────────┬──────────────────────────────────────┤
│                      │                                       │
│  PIPELINE LAUNCHER   │                                       │
│  ─────────────────   │   ACTIVITY FEED                       │
│  • backup            │   (live output, status badges)        │
│  • standup           │                                       │
│  • pr-triage         │                                       │
│                      │                                       │
│  ASSISTANT           │                                       │
│  ─────────────────   │                                       │
│  Provider: ollama    │                                       │
│  Model: llama3.2     │                                       │
│                      │                                       │
│  > _                 │                                       │
│                      │                                       │
├──────────────────────┴──────────────────────────────────────┤
│ [TAB] focus  [j/k] navigate  [↵] launch  [ESC] back         │
└─────────────────────────────────────────────────────────────┘
```

Press `Tab` to move between regions. Use `j`/`k` to navigate lists. Press `↵` to launch a pipeline or send a message.


## Talking to Your Assistant

Type anything in the assistant input and press `ctrl+↵` to send. Your assistant knows about your recent runs, what pipelines you have, and the current working directory.

Ask questions:

```text
why did the backup fail?
what's the diff on PR 42?
suggest a refactor for this function
```

Give it direct commands:

```text
run standup
launch pr-triage
show pipelines
```

Your assistant only launches a pipeline when you explicitly tell it to. Questions and analysis requests are answered directly without triggering a run.

Press `c` from anywhere in the workspace to jump straight to the assistant input.


## Slash Commands

Slash commands take immediate action without involving the AI.

### Running Pipelines

```bash
/pipeline backup              # run the "backup" pipeline by name
/pipeline my-new-pipeline     # starts a creation flow if it doesn't exist yet
```

Output streams to the Activity Feed in real time.

### Managing Your Workspace

```bash
/terminal                     # open a new terminal pane (bottom, 25%)
/terminal 50%                 # 50% split
/terminal left 40%            # left side, 40% width
/terminal in ~/projects/myapp # pane opens in that directory
/terminal htop                # pane runs htop directly
```

### Sessions

Manage separate conversation threads without losing context:

```bash
/session new debug            # create a session named "debug"
/session debug                # switch to it (shorthand: /s debug)
```

Active session shows `●` in the footer. Background sessions with new activity show `◐`.

### Other Commands

```bash
/help                         # show command reference
/quit                         # exit gl1tch
```


## Launching Pipelines From the Sidebar

Your saved pipelines (`.pipeline.yaml` files in `~/.config/glitch/pipelines/`) appear in the left sidebar automatically.

1. Press `Tab` to focus the Pipeline Launcher
2. Use `j`/`k` to navigate to the pipeline you want
3. Press `↵` to launch it
4. Watch output stream in the Activity Feed

While a pipeline is running, the launcher shows `[running]` and the feed updates live. When it finishes, the badge changes to `[done]` or `[failed]`.


## The Activity Feed

The Activity Feed shows everything that runs — pipeline output, assistant responses, errors. Each entry is timestamped and tagged.

```text
[14:23:05] [running] pipeline:standup
  → Fetching commits...
  → Writing standup...

[14:23:18] [done] pipeline:standup
  → standup.md saved.
```

Navigate with `j`/`k`. Press `↵` on any entry to open the full result — you'll see the complete output, step-by-step timings, and a re-run button.

### Searching and Filtering

| Key | Action |
|-----|--------|
| `/` | Fuzzy search entries by name |
| `f` | Cycle status filter: `all` → `running` → `done` → `failed` |
| `j`/`k` | Navigate entries |
| `↵` | Open full result |


## Keyboard Reference

### Navigation

| Key | Action |
|-----|--------|
| `Tab` | Cycle focus: Launcher → Assistant → Feed → Launcher |
| `Shift+Tab` | Cycle focus backward |
| `h` / `←` | Focus left sidebar |
| `l` / `→` | Focus Activity Feed |
| `j` / `k` | Move up/down in current region |
| `↵` | Launch / open / send |
| `Esc` | Cancel or go back |

### Workspace Shortcuts

| Key | Action |
|-----|--------|
| `c` | Jump to assistant input |
| `T` | Open theme picker |
| `R` | Refresh provider and model list |
| `?` | Show help |

### Chord Shortcuts

Chord shortcuts start with `ctrl+space` followed by a key:

| Chord | Action |
|-------|--------|
| `ctrl+space a` | Focus assistant panel |
| `ctrl+space r` | Reload gl1tch (picks up a new binary) |
| `ctrl+space s` | Open the ops popup |
| `ctrl+space \|` | Split pane horizontally |
| `ctrl+space -` | Split pane vertically |
| `ctrl+space x` | Kill current pane |
| `ctrl+space ←/→/↑/↓` | Navigate panes by direction |


## Customizing

### Themes

Press `T` to open the theme picker. Available themes:

| Theme | Style |
|-------|-------|
| **Dracula** | Dark, high-contrast purples and cyans (default) |
| **Nord** | Cool arctic palette |
| **Gruvbox** | Warm earthy tones |

Your theme choice persists across sessions.

### Named Sessions

Create separate conversation threads for different contexts — debugging a deploy in one session, drafting docs in another:

```bash
/session new deploy-debug
/session new docs-draft
/s deploy-debug              # switch back
```

Each session keeps its own full history. Background sessions with new activity are highlighted so nothing gets missed.


## Examples

### "Why did that fail?"

After a pipeline fails, ask your assistant directly:

```text
why did backup fail?
```

Your assistant sees the recent run output, exit code, and step details. It answers based on what actually happened.

### "Run something and explain it"

```text
run standup explain
```

The pipeline runs immediately. Once it finishes, your assistant narrates what it did and what the output means.

### "Open a scratch terminal while a pipeline runs"

```text
/terminal 50% right
```

A new pane opens on the right. Your pipeline keeps running and streaming to the Activity Feed. Use `ctrl+space →` to switch back to the feed.


## See Also

- [Pipelines](/docs/pipelines/pipelines) — Write and structure `.pipeline.yaml` files
- [Examples](/docs/pipelines/examples) — Copy-paste pipelines for real workflows
- [Your First Pipeline](/docs/pipelines/quickstart) — Get running in five minutes
