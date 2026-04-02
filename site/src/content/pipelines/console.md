---
title: "Console — The Switchboard"
description: "Navigate the GL1TCH Switchboard TUI, launch pipelines, run agents, and manage your workspace."
order: 2
---

GL1TCH runs inside a tmux session. The **Switchboard** (window 0) is your primary control panel — a full-screen BubbleTea TUI where you launch pipelines, run agents, check activity, and navigate your workspace. Everything streams live to the Activity Feed with status badges and timestamps.


## The Switchboard Layout

The Switchboard divides into three regions:

```
┌─────────────────────────────────────────────────────────────┐
│ GL1TCH DECK — CONTROL PANEL                         hh:mm:ss │
├──────────────────────┬────────────────────────────────────────┤
│                      │                                        │
│  PIPELINE LAUNCHER   │                                        │
│  ─────────────────   │      ACTIVITY FEED                     │
│  • backup.pipeline   │      (live output, results,            │
│  • deploy.pipeline   │       status badges)                   │
│  • sync.pipeline     │                                        │
│                      │                                        │
│  AGENT RUNNER        │                                        │
│  ─────────────────   │                                        │
│  Provider: ollama    │                                        │
│  Model: neural-chat  │                                        │
│                      │                                        │
│  Prompt: ___________ │                                        │
│  [Enter to send]     │                                        │
│                      │                                        │
├──────────────────────┴────────────────────────────────────────┤
│ [TAB] focus  [j/k] navigate  [↵] launch  [ESC] back  [T] theme│
└───────────────────────────────────────────────────────────────┘
```

**Left Sidebar**: Pipeline Launcher (saved `.pipeline.yaml` files) and Agent Runner (provider/model picker + inline prompt input).

**Center/Right**: Activity Feed — timestamped output from pipeline runs and agent invocations, with status badges (`[running]`, `[done]`, `[failed]`).

**Bottom Bar**: Compact keybinding reference showing the most important shortcuts.


## Sidebar and Focus Navigation

The Switchboard divides into three focusable regions: **Pipeline Launcher** (top-left), **Agent Runner** (bottom-left), and **Activity Feed** (center/right).

### Region Navigation

| Key | Action |
|---|---|
| `tab` | Cycle focus forward: Launcher → Agent Runner → Activity Feed → Launcher |
| `shift+tab` | Cycle focus backward |
| `h` or arrow-left | Shift focus to the left sidebar (from Activity Feed) |
| `l` or arrow-right | Shift focus to the Activity Feed (from sidebar) |

When you press `tab` in the Launcher, focus moves to the Agent Runner. Press `tab` again to move to the Activity Feed. From the Feed, `tab` wraps back to the Launcher.

### Selection Within Regions

| Key | Action |
|---|---|
| `j` | Move selection down (in lists, left sidebar, or Activity Feed) |
| `k` | Move selection up |
| `↵` (Enter) | Launch or open the selected item |
| `esc` | Cancel or deselect |

### Visual Controls

| Key | Action |
|---|---|
| `T` or `^spc t` | Open theme picker modal |
| `?` or `h` | Show help (keybinding reference) |
| `R` | Reload providers/models (refresh from disk) |


## Pipeline Launcher

The **Pipeline Launcher** scans `~/.config/glitch/pipelines/` for saved `.pipeline.yaml` files and displays them as a scrollable list in the left sidebar.

### Launch a Pipeline

You can launch a pipeline from the sidebar list or by typing in the chat. Chat-based dispatch requires an explicit run-verb — GL1TCH only routes to a pipeline when you clearly ask it to:

```
run backup
launch deploy
execute sync-repos on https://github.com/org/repo
```

Questions and generic task requests (`check the logs`, `review my PR`) are answered by the AI directly without triggering a pipeline.

To launch from the sidebar:

1. Use `j`/`k` to navigate to the pipeline you want
2. Press `↵` to launch it
3. Output streams live to the Activity Feed
4. A status badge (`[running]` → `[done]` or `[failed]`) tracks progress
5. Each run is timestamped and added to the Inbox

### Active Job

While a pipeline is running, the launcher is disabled and shows a `[running]` badge. You can still navigate other parts of the Switchboard and watch the output stream in real time. When the job completes, the launcher re-enables and the badge updates.

> [!TIP]
> Multi-job queuing is planned for a future release. For now, wait for the current job to finish before launching another.


## Agent Runner

The **Agent Runner** is an inline agent invocation tool in the left sidebar. Pick a provider and model, type a prompt, and send it directly to your local Ollama or cloud provider.

### Anatomy

```
Provider: [ollama                    ↓]
Model:    [neural-chat              ↓]

Prompt:   ____________________________
          [Your prompt here]
          [↵ to send]
```

**Provider**: Dropdown showing installed sidecars and configured cloud providers. Managed via `apm.yml`.

**Model**: Dropdown populated from the selected provider's available models (refreshed at startup and on `R` key).

**Prompt Input**: Textarea for typing your message. Press `↵` to submit; output streams to the Activity Feed.

### Running an Agent

1. Navigate to the Agent Runner section (use `tab`)
2. Use `↓` to open the Provider dropdown and select a sidecar or cloud provider
3. Use `↓` again to open the Model dropdown and pick a model
4. Click in the Prompt field and type your message
5. Press `↵` to send
6. Output appears in the Activity Feed with a `[running]` badge
7. When the agent responds, the badge updates to `[done]` or `[failed]`

Press `Esc` at any time while GL1TCH is responding to cancel the in-flight stream. Any partial response is saved to the history and input focus is restored immediately.

### Providers and Models

GL1TCH auto-detects installed Ollama sidescar via `~/.local/share/ollama/models` and registered cloud providers from `apm.yml`. Available models are pulled at startup.

To refresh the model list after installing a new sidecar, press `R` in the Switchboard. This re-calls the provider discovery without restarting the TUI.


## Activity Feed

The **Activity Feed** (center/right pane) displays real-time output from pipeline runs and agent invocations. Each entry is timestamped and tagged with its source.

### Entry Structure

```
[14:23:05] [running] pipeline:backup.yaml
  → Backing up database...
  → Compressing files...

[14:23:18] [done] agent:ask
  → Response from neural-chat:
  Here's what I think about that...
```

**Timestamp**: Shows when the activity started (not when output was written).

**Status Badge**: 
- `[running]` — job is active
- `[done]` — job completed successfully
- `[failed]` — job exited with non-zero status or error

**Source**: Either `pipeline:<name>` or `agent:<provider>:<model>`.

**Output**: Streamed line-by-line. Long pipelines show all stdout/stderr captured during execution.

### Selecting and Inspecting Results

The Activity Feed is scrollable. Navigate with `j`/`k` to move between entries. Press `↵` to open the full result in the **Inbox Detail Modal**.

### Live Streaming

Output is streamed in real time as it arrives from the pipeline or agent. The feed is throttled (50ms debounce) to avoid excessive BubbleTea re-renders on high-frequency output. Very long or complex pipelines may batch lines together.

### Search and Filter

The Signal Board (Activity Feed) supports efficient searching and filtering:

| Key | Action |
|---|---|
| `f` | Cycle through status filters: `running` → `all` → `done` → `failed` → `archived` |
| `/` | Enter fuzzy search mode; type to match entry titles; `Esc` to clear search |
| `j`/`k` | Navigate entries (normal mode) or navigate + mark (mark mode) |
| `m` | Toggle mark mode — hold position while marking/unmarking multiple entries |

**Mark Mode** (`m`): Toggles between normal navigation and mark-while-navigate. Useful for batch operations on related runs. Press `m` again to exit mark mode.

**Fuzzy Search** (`/`): Match entry titles by substring. The search resets when you select an entry or press `esc`.

### JSON Output Expansion

Pipeline output containing valid JSON objects or arrays is automatically detected and rendered compactly:

```
▸ { … } (8 keys)
```

Press `↵` on a JSON line to expand inline:

```
▾ { … } (8 keys)
  "status": "success"
  "duration_ms": 1234
  "steps": [ 3 items ]
  …
```

Press `↵` again to collapse. Expanded JSON is pretty-printed with syntax highlighting (keys in accent color). Large objects are truncated at 20 lines with an overflow indicator.


## GL1TCH Chat Panel

The right side of the Switchboard displays a streaming conversation with the GL1TCH AI assistant. The assistant provides analysis, suggestions, and direct answers based on your pipeline runs and prompts.

### Chat Context

The assistant's context includes:
- **Current environment**: Working directory, active model, pipeline paths
- **Recent runs**: Last 5 activity feed entries, their status, output, and step details
- **Your prompts**: Full conversation history within the session
- **Run analysis**: OTel traces, exit codes, error messages from pipeline steps

When a pipeline fails, the assistant automatically analyzes the failure and suggests remediation steps. When you ask a question, it answers based on recent context without necessarily triggering a new run.

### Sending Messages

| Key | Action |
|---|---|
| `c` | Focus the chat input area (from any Switchboard region) |
| `ctrl+↵` | Send your message (normal `↵` adds a newline) |
| `esc` | Cancel an in-flight stream and return focus to the Switchboard |

**Input area**:
```
You: __________________________________
    [Type your prompt here]
    [Ctrl+Enter to send]
```

### Response Format

Responses appear as timestamped entries in the chat panel:

```
gl1tch: The deployment finished with exit code 0. All
        three steps completed in 2m 15s. The manifest
        was successfully applied to prod.
```

If the assistant suggests running a pipeline, it formats the suggestion as an action:

```
gl1tch: I recommend running schema-sync --force to
        resolve the schema validation error.
```

You can type `/pipeline schema-sync --force` in the chat to execute this immediately, or select it from the launcher.

### Streaming

Long responses stream token-by-token with a blinking cursor (`▌`) to show activity. Press `esc` to cancel the stream; any partial response is preserved in the history.

### Clarifications

If a pipeline reaches a step with `AskClarification` signal, the question appears in the chat panel with an input box. Answer the question directly in the chat, and execution resumes automatically.

```
glitch: Schema validation failed. Should I rollback
        to the previous version? (yes/no)
You: __________________________________
```


## Chord Shortcuts

Chord shortcuts start with `^spc` (ctrl+space) followed by a key. This is the primary way to navigate your GL1TCH workspace beyond the Switchboard.

| Chord | Action |
|---|---|
| `^spc h` | Show help screen (full keybinding reference) |
| `^spc c` | Create a new window |
| `^spc d` | Detach from the session (safely exit tmux) |
| `^spc r` | Reload GL1TCH (pick up a new binary without restarting) |
| `^spc q` | Quit GL1TCH entirely (tears down all tmux sessions) |
| `^spc [` | Previous window in the session |
| `^spc ]` | Next window in the session |
| `^spc x` | Kill the current pane |
| `^spc X` | Kill the current window |
| `^spc a` | Jump to the GL1TCH assistant (shell/REPL pane) |
| `^spc t` | Open the theme picker |
| `^spc n` | New workspace session (alternative to `^spc c` for named sessions) |
| `^spc p` | Open the pipeline/prompt builder |

### Terminal and Pipeline Commands

The `/terminal` and `/pipeline` commands control your workspace without requiring modal dialogs or leaving the Switchboard.

#### `/terminal` — Open Panes with Natural Language

The `/terminal` command supports natural-language syntax for opening and laying out tmux panes:

```
/terminal                              # bottom pane, 25%
/terminal 50%                          # 50% size
/terminal half                         # 50% (alias)
/terminal third bottom                 # bottom 33%, vertical split
/terminal left 40%                     # left side, 40%
/terminal 3 shells                     # open 3 panes stacked
/terminal in /tmp                      # pane in /tmp
/terminal cwd to ~/a ~/b ~/c           # 3 panes, different directories
/terminal htop                         # pane runs `htop` (shell fallback)
```

**Size keywords**: `25%`, `50%`, `half`, `third`, `quarter`, `full` (or raw percentage).

**Direction keywords**: `bottom`, `vertical`, `below` (v-split); `left` (h-split to the left).

**Count**: `<N> shells`, `<N> terminals`, `<N> panes` (opens that many splits).

**Working directory**: `in <path>` (single); `cwd to <path1> <path2> …` (per-pane).

If the input doesn't match any natural-language pattern, it's treated as a raw shell command and executed in a new pane.

#### `/pipeline` — Run and Schedule Pipelines

Run a saved pipeline with optional input and scheduling:

```
/pipeline backup                           # run with defaults
/pipeline deploy --prod                    # run with flags
/pipeline backup --input "weekly"          # pass input data
/pipeline schema-sync --at "2pm tomorrow"  # schedule for later
```

Output streams to the Activity Feed. If the pipeline takes longer than a few seconds, a dedicated tmux window opens so you can tail scrollback while continuing other work in the Switchboard.

#### Other `/` Commands

| Command | Action |
|---|---|
| `/help` | Show command reference |
| `/quit` | Exit GL1TCH and tear down all tmux windows |

Pipeline panes (opened when you run a pipeline) are laid out automatically: first job splits horizontally, subsequent jobs stack vertically on the right. Use `/terminal equalize` to rebalance panes after several runs.


## Modal Workflows

Several features open as full-screen modals layered on top of the Switchboard. You can always press `esc` to close and return to the main view.

### Theme Picker (`T` or `^spc t`)

Opens a modal showing available themes. Use `j`/`k` to navigate, press `↵` to select. The theme applies immediately to all GL1TCH panels (Switchboard, Inbox, prompts, etc.) and persists across sessions.

Currently available themes:
- **Dracula** (default) — Dark mode with purples, cyans, and high contrast
- **Nord** — Cool, arctic colors
- **Gruvbox** — Warm, earthy tones

### Inbox Detail (`↵` from Activity Feed entry)

Selecting an entry in the Activity Feed and pressing `↵` opens the **Inbox Detail Modal**:

```
┌──────────────────────────────────────────────────┐
│ Pipeline: backup.yaml                   [12 of 47]│
│ Status: done | Exit: 0 | Duration: 12.3s        │
├──────────────────────────────────────────────────┤
│ started:   2026-04-02 3:45:22 PM                 │
│ finished:  2026-04-02 3:48:45 PM                 │
│ cwd:       ~/projects/myapp                      │
│ model:     neural-chat:7b                        │
│ ────────────────────────────────────────────────│
│ steps:                                           │
│   ├ ✓ backup-database      2.1s                  │
│   ├ ✓ compress-files       1.5s                  │
│   ├ ✓ verify-integrity     0.8s                  │
│   └ ✓ upload-to-archive    7.9s                  │
│                                                  │
│ [p]rev [n]ext [d]elete [r]erun [o]tel [esc]back │
└──────────────────────────────────────────────────┘
```

**Run Metadata**:
- **started** / **finished**: Timestamps when the run began and completed
- **duration**: Wall-clock time elapsed
- **exit**: Exit status (0 = success, non-zero = failure)
- **cwd**: Working directory the pipeline ran in
- **model**: LLM model used (if applicable)

**Steps Section**: Shows each pipeline step with a badge:
- `✓` (green) — completed successfully
- `✗` (red) — failed with error
- `·` (dim) — pending or in progress

Each step shows duration in seconds. If OTel tracing is enabled, step durations come from span timings (more accurate than wall-clock).

**Navigation inside the modal**:
- `j`/`k` or arrows — Scroll through the output
- `p` — Jump to the previous result in the Inbox
- `n` — Jump to the next result in the Inbox
- `d` — Delete the current result (removes from Inbox)
- `r` — Re-run the pipeline or agent with the same parameters
- `o` — Open the OTel trace tree (if traces are available)
- `esc` — Close and return to the Switchboard

### OTel Trace View

When a pipeline includes instrumented steps, press `o` in the Inbox Detail to view the trace tree:

```
fetch-schema                                    OK · 850ms
  ├ http.request                                OK · 145ms
  ├ http.response                               OK · 12ms
  └ json.parse                                  OK · 3ms
transform                                       OK · 680ms
  ├ schema.validate                             OK · 450ms
  └ schema.transform                            OK · 230ms
publish                                         OK · 120ms
  └ http.request                                OK · 115ms
```

Spans are indented by depth. Each shows status (`OK` or `ERR`) and duration in milliseconds. This view is useful for diagnosing performance bottlenecks in slow pipelines.

### Prompt Builder (`^spc p`)

Opens a full-screen accordion-style editor for authoring and testing new pipelines or agent prompts. This is a separate view from the Switchboard and is useful for iterative development.

```
┌──────────────────────────────────────────────────┐
│ PROMPT BUILDER                                   │
├──────────────────────────────────────────────────┤
│ Title: [New Pipeline              ]              │
│                                                  │
│ ▼ Step 1: Input                                  │
│   Type: [user-input      ▼]                      │
│   Prompt: [Ask user...   ]                       │
│                                                  │
│ ▼ Step 2: Brain                                  │
│   Model: [gpt-4          ▼]                      │
│   System: [You are...    ]                       │
│                                                  │
│ ▼ Step 3: Output                                 │
│   Format: [markdown      ▼]                      │
│                                                  │
│ [save] [test] [esc] back                         │
└──────────────────────────────────────────────────┘
```

**Key actions**:
- `tab` — Move between sections
- `↵` — Expand/collapse a step
- `j`/`k` — Navigate between steps
- `space` — Expand/collapse the current step
- `[save]` button — Save your pipeline to `~/.config/glitch/pipelines/`
- `[test]` button — Run a test execution and show output inline
- `esc` — Return to the Switchboard

### Theme Persistence

The active theme is stored in `~/.config/glitch/state.json` and loaded automatically when you reattach to your GL1TCH session. All panels (Switchboard, modals, pane borders, text colors) respect the theme.


## Status Indicators

The Switchboard shows several status indicators to keep you aware of what's happening:

### Chat Panel Subtitle

The Switchboard window title and subtitle show:
- Current time (top right)
- Active job badge (`[1 running]` if pipelines are active)
- Theme indicator (brief name of the active theme)

### Exit Status

When a pipeline or agent finishes, the Activity Feed entry shows:
- `[done]` — Exited cleanly (status 0)
- `[failed]` — Non-zero exit code or error
- Exit code displayed in the Inbox Detail modal

### Provider/Model Availability

If a provider is unavailable (Ollama not running, cloud API key invalid), the Agent Runner shows a warning and disables the Model dropdown until the provider is reachable again.


## Tips and Patterns

### Quick Pipeline Testing

1. Navigate to your pipeline in the launcher
2. Press `↵` to run it
3. Watch output stream in the Activity Feed
4. If it fails, press `^spc p` to open the builder, make edits, save, and test again
5. When satisfied, the pipeline is already saved and ready for production

### Agent Iteration

1. Open the Agent Runner section (use `tab`)
2. Write your prompt in the textarea
3. Press `↵` to send
4. Read the response in the Activity Feed
5. Use `↵` on the activity entry to open the full result
6. Refine your prompt and press `↵` again
7. Responses are saved to the Inbox for reference

### Multi-Window Workflow

While a pipeline is running in the Switchboard:
1. Press `^spc c` to open a new window
2. Type commands directly in the shell (e.g., `tail -f /var/log/app.log`)
3. Press `^spc j` to jump back to the Switchboard
4. Check the Activity Feed for your pipeline's progress
5. When done, use `^spc ]` to cycle back through windows

### Saving and Re-running Results

Pipeline outputs are automatically saved to the **Inbox**. To re-run an old pipeline:
1. Open the Activity Feed in the Switchboard
2. Navigate to the result you want to repeat
3. Press `↵` to open the Inbox Detail modal
4. Press `r` to re-run with the same parameters
5. Output from the new run streams to the Activity Feed

## See Also

- [Pipelines](./pipelines.md) — Author and structure `.pipeline.yaml` files
- [Sidecars & Agents](./sidecars.md) — Configure AI providers and models
- [Router & Intent Dispatch](./router.md) — How the assistant interprets your prompts and commands
- [Signals & Handlers](./signals.md) — Set up narration, achievements, and game events
- [Themes & Colors](./themes.md) — Customize the Switchboard appearance

