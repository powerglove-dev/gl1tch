---
title: "Architecture"
description: "System design: three-column TUI, tmux session structure, component map, and CLI-to-console flow."
order: 10
---

GL1TCH is a tmux-native application built around a single BubbleTea TUI called the **Deck**. The Deck is the control surface: you launch pipelines, run agents, inspect results, and monitor activity all from one full-screen interface. Behind the scenes, a plugin system executes workloads, a SQLite store captures run history, and tmux provides the durable session container.


## Three-Column TUI Layout

The Deck divides into three regions optimized for parallel visibility:

```
┌───────────────────────────────────────────────────────────────┐
│  GL1TCH                                              hh:mm:ss  │
├──────────────────┬──────────────────┬─────────────────────────┤
│                  │                  │                         │
│  LEFT COLUMN     │  CENTER COLUMN   │   RIGHT COLUMN          │
│  ────────────    │  ──────────────  │   ──────────────        │
│                  │                  │                         │
│  • Pipelines     │  • GL1TCH Chat   │   Activity Feed         │
│    (launcher)    │    (main prompt) │   (live output,         │
│                  │  • Agent Grid    │    results, status)     │
│  • Agent Runner  │  • Signal Board  │                         │
│    (form)        │  • Inbox Detail  │                         │
│                  │  • Overlays:     │                         │
│  (switchable     │    - Theme Picker│                         │
│   with Tab)      │    - Editors     │                         │
│                  │    - Modals      │                         │
│                  │                  │                         │
└──────────────────┴──────────────────┴─────────────────────────┘
```

**Left column** (fixed ~22 chars): Pipelines list (searchable, scrollable) and the Agent Runner form (3-step wizard with model/prompt selection). Both panels are keyboard-navigable; `Tab` cycles focus.

**Center column** (flexible, ~33 chars): Host to multiple views—GL1TCH AI assistant chat (primary), agent execution grid, signal board for live job monitoring, or inbox detail for reviewing past runs. Overlays (theme picker, pipeline/brain editors, re-run modal) float above. All driven by keyboard input and TUI state.

**Right column** (fixed ~30 chars): Activity Feed—a scrollable log of every launched pipeline and agent run. Each entry shows title, status (running/done/failed), elapsed time, and a collapsible step list. Selecting an entry expands it; live output streams in as the job runs.


## Tmux Session Structure

GL1TCH runs entirely within a single tmux session named `glitch`, created and managed by the bootstrap layer.

**Session layout:**

- **Window 0** (immutable): The GL1TCH Deck TUI. Protected from deletion (`^spc x` is a no-op on this window); only `q` or `^c` tears down the entire session.
- **Windows 1+** (ephemeral): Ad-hoc shells, pipeline execution windows, and job panes. Created on-demand by the user or by the Deck when launching a job in a background window.

**Tmux configuration** (set via `tmux new-session` + `-c` wrapper script at bootstrap):

- Status bar: Minimal—shows current window name in center (e.g., `0:GLITCH`), chord hints (e.g., `^spc c new window`) and clock on the right. Dracula theme colors pulled from the active GL1TCH theme bundle.
- Keybindings: `ctrl+space` enters the `glitch-chord` key table (modal, exits with `Esc` or `ctrl+space` again). Chord keys handle window/pane navigation (`c` new, `[` `]` prev/next, `|` `-` split), detach (`d`), reload (`r`), and kill (`x` pane, `X` window).
- Pane borders: Styled with the theme's border color; active pane highlighted in accent color.
- Mouse support: Enabled for pane focus and scrolling; `shift+click` for text selection (terminal default).


## Core Component Map

GL1TCH is composed of these interconnected layers:

```
main
 ├─ bootstrap (setup tmux session, keybindings, pane layout)
 │  └─ tmux session "glitch" (window 0: Deck TUI)
 │
 └─ console.Run() (inside tmux: BubbleTea event loop)
    ├─ Model (Deck) — main TUI state machine
    │  ├─ launcher (pipelines list + focus)
    │  ├─ agent (agent runner form)
    │  ├─ activeJobs (map[id]*jobHandle — in-flight work)
    │  ├─ feed (activity feed entries)
    │  └─ center panel (chat, grid, inbox, editors, modals)
    │
    ├─ pluginmanager (execute pipelines + agents)
    │  ├─ manifest.go (parse glitch-plugin.yaml)
    │  ├─ exec.go (spawn executor binaries)
    │  ├─ fetch.go (pull plugin source from GitHub)
    │  └─ installer.go (go install / release download)
    │
    ├─ store (SQLite result database)
    │  └─ ~/.local/share/glitch/glitch.db (Run, StepRecord, UserScore, etc.)
    │
    ├─ busd (event bus — IPC via JSONL topics)
    │  └─ pipelineBusEventMsg (RunStarted, StepComplete, RunFinished)
    │
    ├─ themes (Dracula palette + TUI styling)
    │ └─ ~/.config/glitch/themes/ (user themes)
    │
    └─ activity (JSONL-backed timeline for feed display)
```

**Data flow for a pipeline run:**

1. User selects a pipeline from the Launcher → Model queues a pipeline launch.
2. Deck calls `pluginmanager.Exec()` with the pipeline YAML path.
3. Executor (e.g., `claude-executor`) spawns in a tmux window; runs steps sequentially.
4. Executor writes step status to stdout (`[step:<id>] status:running`, `status:done`, etc.).
5. Deck tail-reads the tmux pane or log file; parses step messages; updates `feed[].steps[].status`.
6. When the job finishes, Store records a `Run` record + all `StepRecord` outputs.
7. Activity Feed displays the entry; user can expand to see step details or click through to the inbox.

**Data flow for an agent run:**

1. User fills in Agent Runner form (provider, model, prompt) → submits.
2. Deck invokes the agent executor (e.g., `claude-agent` sidecar) with the prompt.
3. Executor returns a response; Deck buffers it in `glitchChat` panel.
4. User can edit, regenerate, or accept the response.
5. If accepted, Store records a `Run` (kind="agent"); Inbox can retrieve it later.


## CLI Commands Wire Into Console

GL1TCH has two entry points, unified by the `TMUX` environment variable:

**From outside tmux:**
```bash
$ glitch
```
→ `main()` detects no `TMUX` env var → calls `bootstrap.Run()` → creates/reattaches session → execs the glitch binary *inside* the new window → falls through to console case.

**From inside tmux (window 0):**
```bash
$ glitch
```
→ `main()` detects `TMUX` env var → calls `console.Run()` → BubbleTea event loop → TUI runs until user quits.

**CLI subcommands** (pipeline, ask, help, config, cron, workflow, completion, widget, backup, restore, game, plugin, etc.) bypass the TUI:
```bash
$ glitch pipeline deploy --prod
$ glitch ask "what's my account balance?"
```
→ `main()` routes to `cmd.Execute()` → Cobra CLI handles the subcommand → may write to Store or print to stdout, then exits.

**Special commands:**

- `glitch _reload` — bootstrap-internal; writes a marker file and detaches. On detach, `bootstrap.Run()` sees the marker and re-execs the glitch binary (picks up a newly compiled binary without losing the session).
- `glitch _opsx` — (internal) display the OpenSpec explorer popup (tmux display-popup).

**Keybinding-driven TUI actions** — within the Deck:

- `/pipeline <name>` — slash command to run a pipeline (parsed by the chat input handler).
- `/terminal [size] [dir]` — open a new tmux pane with optional working directory.
- `^spc r` (Deck forwards to tmux) → tmux chord executes `glitch _reload`.
- `^spc q` → tmux detach + exit; `bootstrap.Run()` catches the detach and returns; glitch exits.


## Lifecycle: Startup to Shutdown

**Startup (`glitch` command):**

1. `main()` runs; checks `TMUX` env var and `os.Args[1]`.
2. If no tmux and no subcommand → `bootstrap.Run()` starts.
3. Bootstrap creates/reattaches tmux session "glitch" with:
   - Window 0: runs `glitch` (re-execs binary).
   - Keybindings: registers chord key table.
   - Theme: loads active theme from `~/.config/glitch/themes/`.
4. Binary re-execs inside window 0 → `TMUX` env var now set → `console.Run()`.
5. BubbleTea initializes Deck Model:
   - Scans pipelines from `~/.config/glitch/pipelines/*.yaml`.
   - Loads agent providers (ollama, claude, opencode, etc.).
   - Opens Store (SQLite at `~/.local/share/glitch/glitch.db`).
   - Loads theme registry and applies styling.
   - Executes startup game maintenance (auto-resolve expired encounters, apply MUD reputation decay).
6. Deck renders first frame; waits for user input.

**Runtime:**

- User interacts with the TUI (select/launch pipelines, fill agent form, etc.).
- Keyboard input or mouse clicks trigger Model.Update() → state changes → Cmd(s) spawned.
- Cmds may launch jobs (e.g., `exec.Cmd` for a shell, pluginmanager call for a pipeline).
- Job output streams back via channels → Model updates feed entries in real-time.
- Store records are appended on job completion.

**Shutdown:**

- User presses `q` or `^c` in the Deck → `tea.Quit()` → BubbleTea event loop exits.
- `console.Run()` returns → `main()` exits.
- Tmux session remains (user can reattach with `glitch` or `tmux attach-session -t glitch`).
- Explicit session teardown: `glitch q` or `^spc q` → all windows/panes killed; session destroyed.


## Technologies & Dependencies

- **BubbleTea** (`charmbracelet/bubbletea`) — TUI framework; drives the Model-Update-View loop.
- **Lipgloss** (`charmbracelet/lipgloss`) — terminal styling; applies theme colors and layout.
- **Tmux** (external; required) — session container; pane/window management via CLI.
- **SQLite** (`modernc.org/sqlite`) — result database; WAL mode enabled for concurrent read/write.
- **BUSD** (internal; JSONL topics) — event bus for cross-process communication (pipelines → Deck).
- **Ollama** (external; local) — LLM provider for agent runs and vector embeddings.
- **Plugins** (external; via pluginmanager) — executors loaded from `glitch-plugin.yaml` manifests.


## Configuration & YAML Reference

**Pipeline YAML** (in `~/.config/glitch/pipelines/`):

```yaml
name: backup
description: Daily backup of workspace
steps:
  - id: collect
    type: shell
    run: |
      tar czf /tmp/backup.tar.gz ~/work
  - id: summarize
    type: llm
    model: neural-chat
    prompt: |
      Summarize this backup operation:
      {{get "step.collect.stdout" .}}
```

**Plugin manifest** (`glitch-plugin.yaml` in a repo):

```yaml
name: claude-executor
description: Execute Claude API calls as pipeline steps
binary: claude-executor
version: v1.0.0
install:
  go: github.com/8op-org/claude-executor/cmd/claude-executor
sidecar:
  command: claude-executor
  args: ["--listen", "localhost:9090"]
  description: Claude AI executor
  kind: agent
  input_schema: |
    {
      "type": "object",
      "properties": {
        "prompt": { "type": "string" },
        "model": { "type": "string" }
      }
    }
```

**Theme bundle** (`~/.config/glitch/themes/dracula.yaml`):

```yaml
name: dracula
description: Dracula color scheme
palette:
  accent: "#88c0d0"
  bg: "#2e3440"
  fg: "#eceff4"
  dim: "#4c566a"
  border: "#3b4252"
strings:
  button_cancel: "↯ cancel"
  button_submit: "✓ go"
```

**Store schema** (SQLite tables):

- `runs` — `id`, `kind`, `name`, `started_at`, `finished_at`, `exit_status`, `stdout`, `stderr`, `metadata`, `steps` (JSON array of step records).
- `step_checkpoints` — `id`, `run_id`, `step_id`, `step_index`, `status`, `prompt`, `output`, `model`, `vars_json`, `started_at`, `finished_at`, `duration_ms`.
- `user_score` — `id`, `total_xp`, `level`, `streak_days`, `last_run_date`, `total_runs`.
- `score_events` — `id`, `run_id`, `xp`, `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_creation_tokens`, `cost_usd`, `provider`, `model`, `created_at`.
- `game_personal_bests` — `metric`, `value`, `run_id`, `recorded_at`.


## See Also

- [Philosophy](philosophy.md) — Why GL1TCH exists: local-first, terminal-native.
- [Console](console.md) — TUI navigation, keybindings, slash commands.
- [Pipelines](pipelines.md) — Writing and running pipelines.
- [Agents](agents.md) — Running AI agents and managing responses.

