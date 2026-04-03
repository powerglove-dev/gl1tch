---
title: "Console — The Switchboard"
description: "Navigate the GL1TCH Switchboard TUI, launch pipelines, run agents, and manage your workspace."
order: 2
---

GL1TCH runs inside a tmux session. The **Switchboard** (window 0) is your primary control panel — a full-screen BubbleTea TUI that integrates pipeline launching, agent invocation, activity monitoring, and conversational AI. You can launch pipelines or agents from the left sidebar, see live output in the Activity Feed with status badges and timestamps, chat with the GL1TCH assistant for analysis and guidance, and manage multiple named sessions for separate conversation threads.


## Technologies

The Switchboard is built on a set of mature Go libraries and local services:

| Technology | Role |
|---|---|
| **BubbleTea** | Event-driven TUI framework for terminal rendering and input handling |
| **Lipgloss** | ANSI styling and layout management (panels, borders, colors) |
| **Ollama** | Local LLM inference for the GL1TCH assistant |
| **busd** | Local Unix socket message bus for cross-component events (pipelines, steps, crons) |
| **SQLite** (`internal/store`) | Persistence of run metadata, step details, and conversation history |
| **FSNotify** | File system watching for pipeline `.yaml` changes in real-time |
| **Glamour** | Markdown rendering for formatted output and documentation display |
| **OTel** (`internal/telemetry`) | Distributed tracing for performance profiling and step-level diagnostics |
| **Cron** (`robfig/cron`, `internal/cron`) | Scheduled pipeline execution and time-based triggers |

The Switchboard delegates rendering consistency to `internal/panelrender` (header sprites, width calculations, ANSI stripping) and theming to `internal/themes` (Dracula palette, color bindings, dynamic theme switching).


## Named Sessions and Status Badges

GL1TCH supports **named chat sessions** — separate conversation threads you can switch between without losing context. Each session maintains its own message history, conversation state, and status badge. The active session is marked with a `●` indicator in the status bar (top right); background sessions show an attention badge if they have new activity.

### Session States

| Badge | State | Meaning |
|-------|-------|---------|
| `●` | **Active** | Currently visible; keyboard focus is here |
| ` ` | **Idle** | Background session with no new activity |
| `◐` | **Unread** | Background session has new messages since last viewed |
| `⚠` | **Attention** | High-priority activity (errors, clarifications, game alerts) requiring action |

**Switching sessions**: Press `^spc S` to cycle between open sessions. The previous session is marked idle; the new one becomes active. Sessions with unread messages or attention states remain highlighted in the footer.

**Creating a session**: Press `^spc n` (new session) and type a name (e.g., "debug", "refactor", "game"). A new empty session is created and immediately becomes active.

### Status Bar Session Indicators

The tmux status bar (bottom right) displays:
- Current time (always visible)
- Chord hints: `^spc n new` (create session), `^spc p build` (prompt builder), `^spc t themes` (theme picker)
- Attention badge if any background session needs your attention

When a background session receives a new message or encounters an error, its badge changes from idle (`·`) to unread (`◐`) or attention (`⚠`), and the status bar briefly highlights it.


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


## Console Architecture

The Switchboard is built on BubbleTea, GL1TCH's event-driven TUI framework. Understanding the architecture helps when customizing behavior or debugging layout issues.

### Core Components

**Deck** (`internal/console/deck.go`) — The root model. It orchestrates all panels (GL1TCH chat, pipeline launcher, agent runner, activity feed), handles keybindings, manages the session registry, and routes events. Deck is the entry point for all user input and screen rendering.

**GL1TCH Panel** (`internal/console/glitch_panel.go`) — Streaming chat interface. Handles prompt input, LLM communication via Ollama, token streaming, and cancellation. Maintains conversation turns and detects intent (routing to built-in handlers or the LLM).

**Signal Board** (`internal/console/signal_board.go`) — Activity feed. Subscribes to pipeline/step/cron events via `internal/busd`, maintains a ring buffer of up to 200 feed entries, supports fuzzy search and status filtering, and renders the visible portion with proper height calculations.

**Session Registry** (`internal/console/session.go`) — Manages named chat sessions, each with its own message history and status badge. Sessions are in-memory; the conversation history persists in the active session until you switch or quit.

### Event Flow

1. **User input** → Deck's Update() method
2. **Keypress routing** → Region-specific handlers (chat, launcher, feed)
3. **Intent detection** → Router (built-in handlers or LLM)
4. **Execution** → Pipeline executor, agent invocation, or LLM call
5. **Completion** → busd event published (pipeline_done, step_complete, etc.)
6. **Feed update** → Signal Board subscribes, adds entry, re-renders
7. **UI re-render** → Deck's View() method calculates layout, calls region renderers, outputs ANSI

### Event Bus (busd)

GL1TCH uses `internal/busd` — a local Unix socket message bus — to decouple the Deck from the executor, cron scheduler, and other services. The Deck subscribes to topics like:

| Topic | Event | Meaning |
|---|---|---|
| `pipeline.started` | A pipeline run has been launched | Add entry to Activity Feed with `[running]` badge |
| `pipeline.completed` | A pipeline finished successfully | Update badge to `[done]`, capture final output |
| `pipeline.failed` | A pipeline exited with error | Update badge to `[failed]`, capture error message |
| `step.started` | A step within a pipeline began | Update Inbox detail metadata |
| `step.completed` | A step finished | Record step duration (from OTel span or wall-clock) |
| `step.failed` | A step encountered an error | Mark step as failed in Inbox detail |
| `cron.executed` | A scheduled pipeline fired | Log to Activity Feed as cron job entry |
| `clarification.requested` | A step is asking for user input | Emit question to chat panel, block pipeline until answered |

Subscriptions are managed in `glitch_panel.go` and `deck.go` with message handlers that:
1. Update the Activity Feed in real time (Signal Board updates entry, re-renders)
2. Update the session state (new messages added to the active session)
3. Trigger re-renders to reflect new activity

This asynchronous architecture allows pipelines and crons to run independently in background processes while the TUI remains responsive to keyboard input and feed scrolling.

### Styling and Themes

Console styling is provided by `internal/themes` and `internal/styles`. Themes define a color palette (Dracula, Nord, Gruvbox, etc.); the active theme is stored in `~/.config/glitch/state.json` and applied on startup. Panel rendering delegates to `internal/panelrender` for consistent header rendering, ANSI sprite handling, and width calculations.

### Storage

Run metadata, step details, and conversation history are persisted to `internal/store` (SQLite). When you exit GL1TCH, conversation state is lost unless you save it explicitly. Pipeline outputs and OTel traces are logged to:
- `~/.local/share/glitch/traces.jsonl` — OTel trace spans (for `/trace` inspection)
- `~/.cache/glitch/inbox-read-state.json` — Which runs you've marked as read


## Sidebar and Focus Navigation

The Switchboard divides into three focusable regions: **Pipeline Launcher** (top-left), **Agent Runner** (bottom-left), and **Activity Feed** (center/right).

### Sidebar Toggling

The sidebar (Pipeline Launcher + Agent Runner) divides the left pane into two stacked sections: the Pipeline Launcher on top and the Agent Runner below. Each section has its own focus region and keybindings.

**Sidebar visibility**: The sidebar can be toggled with `^spc \` (ctrl+space backslash) to maximize the Activity Feed width when reading large amounts of output. The sidebar state is remembered across sessions.

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

The Activity Feed maintains a **rolling history of up to 200 entries**. When the cap is reached, the oldest non-running entry is evicted first; if all entries are still running (a rare case), the feed truncates hard. This ensures the Switchboard remains responsive even during long sessions with many pipeline runs.

### Search and Filter

The Signal Board (Activity Feed) supports efficient searching and filtering:

| Key | Action |
|---|---|
| `f` | Cycle through status filters: `running` → `all` → `done` → `failed` → `archived` |
| `/` | Enter fuzzy search mode; type to match entry titles; `Esc` to clear search |
| `j`/`k` | Navigate entries (normal mode) or navigate + mark (mark mode) |
| `m` | Toggle mark mode — hold position while marking/unmarking multiple entries |

**Mark Mode** (`m`): Toggles through three states:
- **Off** (normal) — `j`/`k` navigate without marking
- **Active** — `j`/`k` navigate **and** mark/unmark lines (highlighted with a badge)
- **Paused** — `j`/`k` navigate without marking; useful to position on an entry without toggling its mark
Press `m` to cycle between states. Useful for batch operations on related runs.

**Fuzzy Search** (`/`): Match entry titles by substring. The search resets when you select an entry or press `esc`.

### JSON Output Expansion

Pipeline output containing valid JSON objects or arrays is automatically detected and rendered compactly by the **feed parser** system. Detection is zero-cost: each line is checked for leading `{` or `[` and validated with `json.Valid()` before rendering.

**Collapsed (default)**:
```
▸ { … } (8 keys)
▸ [ … ] (3 items)
```

**Expanded** (press `↵` to toggle):
```
▾ { … } (8 keys)
  "status": "success"
  "duration_ms": 1234
  "steps": [ 3 items ]
  "cached": true
```

Expanded JSON is pretty-printed with syntax highlighting (keys in accent color, values in foreground). Large objects are truncated at 20 lines with an overflow indicator (`… N more lines`). Pressing `↵` again collapses the entry.

**Parser Registry** (`feed_parsers.go`):

The feed parser is extensible by design. The system maintains an ordered `feedParsers` slice where each parser is a `FeedLineParser` function:

```go
type FeedLineParser func(raw string, width int, pal ANSIPalette, expanded bool) (lines []string, matched bool)
```

When a feed entry is rendered:
1. Raw ANSI-stripped output is passed to each parser in order (first match wins)
2. If a parser matches and returns `matched=true`, its output lines are used
3. If no parser matches, the line is rendered as plain text

Built-in parsers:
- `jsonFeedLineParser` — Detects and formats JSON objects and arrays

Custom parsers can be added to the `feedParsers` slice to handle other formats (YAML, Protobuf, CSV, etc.) without modifying the core feed rendering logic.


## GL1TCH Chat Panel

The right side of the Switchboard displays a streaming conversation with the GL1TCH AI assistant. The assistant is powered by your local Ollama instance or a configured cloud provider, and provides analysis, suggestions, and direct answers based on your pipeline runs and prompts.

### Chat Context and Assistant Awareness

The assistant's context automatically includes:
- **Current environment**: Working directory, active model, pipeline paths
- **Recent runs**: Last 5 activity feed entries, their status, output, and step details (including OTel trace data)
- **Conversation history**: Full message history within the active session (but not other sessions)
- **Run analysis**: Exit codes, error messages, step durations, and structured output (JSON, ANSI) from pipeline steps
- **Game state** (if applicable): Current player stats, inventory, active quests, recent narration events

When a pipeline or agent job fails, the assistant automatically receives the failure event and can optionally analyze it, suggest debugging steps, or propose re-running with different parameters. When you ask a question, the assistant answers based on available context without necessarily triggering a new run — it's a true conversational interface, not just a command dispatcher.

### Streaming and Token Flow

Responses from the assistant stream **token-by-token** with a blinking cursor (`▌`) to show activity. Very long responses are chunked and displayed in real time as they arrive from Ollama or the cloud provider. You can press `esc` at any time to cancel an in-flight stream; any partial response is preserved in the session history and you can resume the conversation immediately.

Internally, streaming is driven by a pair of BubbleTea message types:

- **`glitchStreamMsg`** — Carries a single token string and a channel for reading the remaining token stream. Emitted when the assistant begins responding.
- **`glitchTickMsg`** — Fires every 120ms to drive the animation frame and pull the next token from the stream channel. Allows smooth 120ms-paced updates without blocking on I/O.
- **`glitchDoneMsg`** — Signals that the stream has finished (channel closed).
- **`glitchErrMsg`** — Carries an error if the stream fails (connection loss, LLM timeout, etc.).

The Update() handler for these messages updates the chat pane's current message buffer, re-renders the view, and continues the `glitchTick` loop until `glitchDoneMsg` arrives. When you press `esc`, a cancellation flag is set and the stream channel is closed, stopping all further pulls and rendering the partial response to history.

### Sending Messages and Intent Routing

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

### Intent Detection and Command Dispatch

When you send a message, the GL1TCH router (`internal/router`) analyzes your input and dispatches it to one of three paths:

**1. Built-in intent** (no LLM call):
Exact phrase matching triggers an immediate handler without consulting the LLM. These are parsed from the message and executed synchronously:

```
run backup                              # launch pipeline named "backup"
launch deploy --prod                    # launch with flags passed as env vars
create session debug                    # new named session, becomes active
switch to refactor                      # switch to existing session
show pipelines                          # list all available .pipeline.yaml files
/terminal 50% left                      # open tmux pane (left, 50% width)
```

The router maintains a pattern registry of these commands. When a match is found, the handler is called immediately and the output (success/failure) is published to busd as a special event that updates the chat panel.

**2. Assistant-driven intent** (full context passed to LLM):
Free-form questions and requests are sent to the configured LLM with full conversation history and recent activity context:

```
why did the backup fail?                # analysis of last failed run
what pipelines are running?             # query against current state
suggest a fix for the schema error      # open-ended suggestion
```

The router recognizes that these don't match a built-in handler and packages the full context (last 5 feed entries, current session history, environment state) into a system prompt before calling Ollama. The assistant's response appears in the chat panel as an analysis, suggestion, or clarification.

**3. Hybrid intent** (built-in + LLM elaboration):
Some messages match a built-in handler but also trigger LLM analysis:

```
run backup explain                      # launch pipeline AND explain what it does
```

The pipeline runs immediately; the LLM then receives the run output and provides commentary in the chat panel.

The routing decision is made in `glitch_panel.go`'s Update() method, which calls the router to parse the message and select the dispatch path.

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

### Clarifications and Blocking Questions

If a pipeline reaches a step with an `AskClarification` signal (e.g., a user-input prompt that blocks until answered), the question appears in the chat panel as a highlighted entry with an input box. Your answer is validated, stored, and published back to the pipeline via the bus (`internal/busd`), which unblocks the waiting `AskClarification` step and resumes execution automatically.

```
glitch: Schema validation failed. Should I rollback
        to the previous version? (yes/no)
You: __________________________________
     [your answer is stored as a ClarificationReply]
```

This enables interactive pipelines: a pipeline can pause mid-execution, ask a question in the UI, wait for your response, and then resume based on your answer. All clarification requests are loaded from the database on startup, so re-attaching to a GL1TCH session will show any pending questions that were unanswered.


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
| `^spc n` | New tmux window (alternative to `^spc c`) |
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

When a pipeline includes instrumented steps, press `o` in the Inbox Detail to view the trace tree. Traces are collected by `internal/telemetry` and written as JSONL to `~/.local/share/glitch/traces.jsonl`. The trace view parses this file, matches spans to the current run by run ID, and displays the call hierarchy:

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

**Note**: Trace data is only available if the pipeline or agent step was instrumented with OTel calls (via `internal/telemetry`). Most built-in steps emit traces automatically; custom pipelines may need explicit instrumentation.

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

### Session Status Badges

The status bar and chat panel footer display badges for each open session:

```
main ●  debug ◐  refactor ·
```

- `●` (solid circle) — Active session (currently visible)
- `◐` (half circle) — Unread messages in background session
- `·` (dot) — Idle background session
- `⚠` (warning) — Attention required (errors, clarification, game alerts)

When a background session transitions to unread or attention, it's briefly highlighted in the status bar and the footer title updates to show the session name.

### Chat Panel Header

The Switchboard window title and header show:
- Current time (top right)
- Active job count (`[1 running]` if pipelines are active; `[0 running]` when idle)
- Theme indicator (brief name of the active theme, e.g., "Dracula")

### Exit Status

When a pipeline or agent finishes, the Activity Feed entry shows:
- `[done]` — Exited cleanly (status 0)
- `[failed]` — Non-zero exit code or error
- Exit code displayed in the Inbox Detail modal

### Provider/Model Availability

If a provider is unavailable (Ollama not running, cloud API key invalid), the Agent Runner shows a warning and disables the Model dropdown until the provider is reachable again.


## Advanced: Tmux Window Integration

When a pipeline or long-running agent job completes, GL1TCH automatically creates a **detached tmux window** (if tmux is available) to display the full output scrollback. This allows you to inspect logs without blocking the Switchboard.

### How It Works

1. Job starts → GL1TCH creates a window named `glitch-<runID>` in the current session
2. Output is tee'd to `/tmp/glitch-<runID>.log` and displayed in the window
3. When the job finishes, the exit code is written to `/tmp/glitch-<runID>.done`
4. Window stays open (via `remain-on-exit on`); `automatic-rename` is disabled so the name doesn't change
5. You can attach to the window with: `tmux attach-session -t <target>`

The tmux window target is fully-qualified, e.g. `session:@123` (using the stable window ID rather than index). You can copy this from the feed entry or the Inbox Detail modal.

### Tail Output

If a pane is already running when you launch a pipeline, GL1TCH can optionally tail its output live with `tail -f /tmp/glitch-<runID>.log` instead of embedding a full shell command. This is a fallback for agent jobs that are managed in-process.

### Layout Stability

The console calculates pane heights precisely to avoid layout drift:
- `signalBoardPanelHeight()` accounts for header sprites, filter rows, search input, and footer
- `sbFixedBodyRows` locks the visible entry count to prevent scroll jumping
- All width calculations use `panelrender.VisibleWidth()` to strip ANSI codes

If you manually resize panes, the console may need a re-render. Press `ctrl+l` to force a redraw.


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
