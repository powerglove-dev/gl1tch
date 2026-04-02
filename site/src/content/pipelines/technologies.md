---
title: "Technologies"
description: "Why gl1tch chose tmux, Ollama, BubbleTea, robfig/cron, OpenTelemetry, and GitHub CLI—the architecture decisions and tradeoffs."
order: 10
---

GL1TCH's technology stack is shaped by a single constraint: *your AI runs on your machine, in your terminal, under your control*. Every major technology choice is either a direct enabler of this goal or a force-multiplier that makes local-first AI practical. This page documents the why, the integration points, and the alternatives we considered.


## Architecture Overview

GL1TCH is a single Go binary that runs inside a tmux session. It has no external processes, no network calls on the hot path, and no cloud dependencies for critical operations. The stack is organized into four layers:

- **Session layer (tmux)**: Durability, composability, and detach/reattach semantics. Every GL1TCH workspace is a tmux session; you can detach and reconnect anytime.
- **UI layer (BubbleTea)**: Event-driven TUI rendering and input handling. The Deck (control panel) is a single BubbleTea application occupying window 0 of the tmux session.
- **Intelligence layer (Ollama)**: Local LLM inference for decisions, embeddings, and agent dispatch. Models run on your machine; prompts never leave unless you explicitly route them to a cloud provider.
- **Coordination layers (scheduler, tracing, pipelines)**: Scheduling with cron expressions, observability with OpenTelemetry, and workflow orchestration with YAML pipelines.

Data flows from terminal input through the TUI event loop → dispatcher → pipeline runners or agent handlers → Ollama or cloud LLM → SQLite for persistence → OpenTelemetry for tracing. The scheduler reads `.pipeline.yaml` files from disk, watches for changes with FSNotify, and triggers runs on cron expressions. Pipelines output to activity feeds in the TUI and to structured logs.


## Technologies


### Tmux — Durability and Composability

**Why:** Tmux solves three hard problems that web UIs and Electron apps cannot. **Durability**: You can detach and reconnect anytime; pipelines keep running offline, and your session survives terminal crashes. **Composability**: Tmux windows and panes are Unix primitives—you can tile multiple views, pipe output to other tools, and script around the UI. **Sovereignty**: Tmux runs locally with zero network calls, zero external dependencies, and zero "service status" pages.

**Integration:** Every GL1TCH instance is a named tmux session (default: `glitch`). The Deck occupies window 0. Pipelines open new windows (1, 2, 3, ...) or panes within existing windows, keeping your workspace organized. The tmux status bar shows the current time, session name, and keyboard hints (via `~/.config/glitch/tmux.conf`). Detaching and reattaching is transparent; the Deck's state is persisted to SQLite and restored on reconnect. The plugin manager shells out to tmux commands (`tmux split-pane`, `tmux new-window`) to create pipeline output panes. All GL1TCH data (logs, traces, activity) is streamed to tmux panes and logged to disk simultaneously.

**Tradeoffs:** Tmux is modal and has a steep learning curve, but once you know it, it becomes invisible—you're working in a terminal, not an app.

**Alternatives considered:**
- Screen (older, less composable)
- Web UI with WebSocket (requires external server, network dependency, loses session on browser close)
- Electron app (bloated, slow startup, not composable with Unix tools)


### BubbleTea — Event-Driven TUI Framework

**Why:** BubbleTea is Go's standard for full-screen TUI applications. It provides Elm-style functional architecture—event-driven rendering with predictable state transitions. Combined with Lipgloss (ANSI styling and layout), it enables pixel-perfect terminal rendering without wrestling with raw escape codes.

**Integration:** The Deck is a single BubbleTea application that occupies the full terminal inside tmux (window 0). Major panels—left sidebar (pipelines/agents), center column (chat/inbox), right column (activity feed)—are composed from smaller BubbleTea models. Keyboard and mouse input flow through the TUI event loop: tmux captures input → terminal forwarding → BubbleTea's Update() method → reducer-style state changes → View() re-renders to ANSI. Overlays (theme picker, inbox detail, agent modal) are layered BubbleTea models using Lipgloss for z-ordering. The Activity Feed subscribes to BUSD events (pipeline runs, step completions, cron triggers) and updates the view in real-time. Chat input flows from BubbleTea → prompt builder → Ollama inference → response streaming back to the center pane. Every interaction is deterministic; tests verify behavior without mocking the TUI.

**Tradeoffs:** BubbleTea is single-threaded; complex interactions require careful state management. But the functional model prevents entire classes of state bugs (race conditions, stale renders, dropped events).

**Alternatives considered:**
- ncurses (C library, low-level, verbose)
- Ratatui (Rust, but GL1TCH is Go)
- Custom ANSI codes (fragile, unmaintainable)


### Ollama — Local LLM Inference

**Why:** Ollama is the only mature tool for running open-weight models locally without Docker complexity. It implements the OpenAI API, so GL1TCH doesn't depend on Ollama's internal architecture.

**Integration:** GL1TCH connects to Ollama via HTTP at `http://localhost:11434` (configurable in `~/.config/glitch/config.yaml`). Three use cases: **Hot-path decisions** (pipeline routing, agent dispatch, brain retrieval) always use local Ollama—fast, private, deterministic. **Brain embeddings** embed step outputs locally and store vectors in SQLite for future retrieval. **Agent chat** streams Ollama responses to the center pane with the GL1TCH assistant label. Pipeline steps declare `provider: local` to invoke Ollama's `/api/generate` endpoint. Cloud providers (e.g., `provider: claude/claude-sonnet-4-6`) are explicitly routed to cloud APIs; decision logic stays local. If Ollama is unreachable, pipelines show an error badge in the activity feed and offer a fallback to cloud providers. Brain context (recent conversation history, relevant embeddings) is passed as system messages to Ollama.

**Tradeoffs:** Local Ollama is slower than Claude or GPT-4 (5–30 seconds per inference depending on hardware). But it's never rate-limited, never leaks prompts, works offline, and stays under your control.

**Alternatives considered:**
- LLaMA.cpp (single-binary but immature CLI)
- Hugging Face Transformers (Python runtime dependency, overkill for serving)
- Cloud-only LLMs (leaks context, requires internet, vendor lock-in)


### robfig/cron — Scheduling

**Why:** Go's standard library only offers simple intervals. `robfig/cron` is the industry-standard cron parser, supporting full POSIX cron syntax (minute, hour, day-of-month, month, day-of-week) with minimal overhead.

**Integration:** Every `.pipeline.yaml` can define a `cron` field with a cron expression (e.g., `cron: "0 2 * * *"` for 2 AM daily). The scheduler (`internal/scheduler`) watches pipeline files via FSNotify and re-registers cron jobs as files change—no restart needed. When a cron expression matches, the scheduler publishes a BUSD event, which triggers the pipeline runner in-process with default parameters. Output is captured to SQLite run history and streamed to the Activity Feed with a clock icon. The activity feed shows next-scheduled run times and allows pausing/resuming cron jobs. All cron state is protected by a mutex for thread-safety.

**Tradeoffs:** Cron expressions are cryptic to non-Unix users. GL1TCH mitigates this with an optional TUI editor showing the next five run times as you type.

**Alternatives considered:**
- APScheduler (Python runtime dependency)
- Custom time-based scheduler (reinvents the wheel)


### OpenTelemetry — Distributed Tracing

**Why:** OpenTelemetry is the CNCF standard for traces and metrics. It decouples instrumentation (what you measure) from export (where you send it). This keeps GL1TCH independent of any single observability vendor.

**Integration:** The `internal/telemetry` package sets up OTel traces. Every pipeline run creates a trace with the pipeline ID as the root span. Each step (shell, LLM, conditional, fork) is a child span with attributes: step ID, provider, prompt hash, latency, token counts, success/failure. LLM calls emit detailed spans showing prompt length, model, temperature, and response tokens. Spans are exported to: **local JSONL file** (`~/.local/share/glitch/traces.jsonl`) for post-mortem analysis, **optional gRPC endpoint** for external dashboards (Jaeger, Tempo, Datadog), **BUSD channel** so the Activity Feed can render latency summaries in real-time. This gives three tiers of observability: **real-time** (watch the TUI as runs happen), **session-level** (scroll the activity log), **historical** (parse JSONL for trend analysis). Sampling is configurable to trade detail for performance.

**Tradeoffs:** OTel adds small overhead; mitigated via sampling.

**Alternatives considered:**
- Custom JSON logging (fragmented, no standard exporters)
- Structured logs only (less causality tracking)
- No observability (opaque failures)


### GitHub CLI — Secure Authentication

**Why:** `gh auth token` is the simplest way to get a valid GitHub PAT without asking the user to manage secrets. GitHub CLI handles authentication, rate limiting, and API versioning. Pipeline steps can orchestrate GitHub workflows without embedding credentials.

**Integration:** When GL1TCH detects a plugin or pipeline step requires GitHub auth (e.g., fetching from a private repo), it shells out to `gh auth token` to get the current user's PAT. No password prompts, no token storage in GL1TCH config—the token is inherited from your local `gh` auth state (`~/.config/gh/hosts.yml`). Pipeline steps can invoke GitHub CLI subcommands: `gh issue list`, `gh pr create`, `gh workflow run`, etc. Output is captured and passed to downstream steps. The plugin manager uses `gh` to fetch private packages from GitHub during installation.

**Tradeoffs:** GitHub CLI is optional; if you don't use GitHub plugins or steps, you never need it. Requires `gh` to be pre-installed and authenticated locally.

**Alternatives considered:**
- SSH keys (more secure but requires key management)
- Config file token (simple but encourages storing secrets in plain text)
- OAuth browser flow (overkill for local plugins)


### Go — Language and Runtime

**Why:** GL1TCH ships as a single statically-linked binary with no runtime dependencies. Go's fast compilation, concurrency model (goroutines, channels), and comprehensive standard library (net, io, encoding/json, context, sync) make it ideal for a long-running server coordinating concurrent pipeline runs, cron jobs, and TUI events. The binary runs everywhere—Linux, macOS, Windows—with the same behavior; no platform-specific runtime needed.

**Integration:** Main entry point is `main.go`. Core packages: `internal/console` (Deck TUI and models), `internal/pluginmanager` (pipeline execution and cron registration), `internal/store` (SQLite wrapper), `internal/telemetry` (OTel setup). Each package is focused on one concern and communicates via clean interfaces. Goroutines handle concurrent operations: one for the Deck's event loop, one for the scheduler, one for each pipeline step execution. Channels coordinate between components. No monolithic god-file; structure enables testing and modification. The binary is statically compiled with all necessary libraries embedded, so you just copy `glitch` to `~/bin/` and run it.

**Tradeoffs:** Go adds a small startup delay (30–100ms) compared to C, but this is negligible for a long-running TUI. Go's error handling is verbose but explicit.

**Alternatives considered:**
- Python (slower startup, requires runtime, harder to distribute as a single binary)
- Rust (steeper learning curve, overkill for an IO-heavy workload)
- TypeScript/Node (heavy runtime, harder to package as a single binary)


### SQLite — Persistence and History

**Why:** GL1TCH needs to remember what pipelines you've run, what you've chatted about, what errors happened, and when. SQLite is serverless, zero-configuration, and embedded. It's fast enough for millions of rows and supports simple querying. No external database server to manage.

**Integration:** The `internal/store` package wraps SQLite. Tables include: `pipelines` (metadata), `runs` (execution history with start time, status, parameters), `steps` (step output, logs, duration), `messages` (chat history), `sessions` (named conversation contexts). Every pipeline run inserts a row in `runs`; every step inserts details in `steps` with duration and error. The Deck queries `runs` and `steps` to populate the activity feed and inbox detail view. Messages are persisted so you can reattach to a session and see previous conversation. All data is stored in `~/.local/share/glitch/glitch.db`.

**Tradeoffs:** SQLite is single-writer; if GL1TCH tries to write while another process holds the lock, it backs off (configurable timeout). This is acceptable for a single-user terminal application.

**Alternatives considered:**
- Postgres (requires external service, overkill)
- JSON files (slow to query, harder to update)
- In-memory only (lose history on detach)


### Additional Libraries

| Library | Role |
|---|---|
| **Lipgloss** | ANSI styling, colors, layout (panels, borders, alignment). Gives the Deck its visual polish without raw escape codes. |
| **Glamour** | Markdown rendering for documentation, formatted step output, inline help. Renders markdown to ANSI in the TUI. |
| **FSNotify** | File system watching. The scheduler watches `~/.config/glitch/pipelines/` for new/changed `.pipeline.yaml` files in real-time. |
| **busd** | In-process Unix socket message bus for cross-component events (pipeline runs, step completions, cron triggers). Decouples components via pub/sub. |


## Concepts

### The Pipeline DAG

A pipeline is a directed acyclic graph of steps. Each step has:
- A **provider** (`ollama/mistral`, `claude/claude-sonnet-4-6`, `bash`, etc.)
- A **prompt** (the input to the provider)
- Optional **brain tags** (context injection from the embedding store)
- Optional **env vars** (passed to the step's execution environment)
- An **output contract** (what the step produces)

Steps are executed in topological order. A step's output can be used as input to downstream steps via template syntax: `{{get "step.<step_id>.data.<field>" .}}`.

### The Brain

The brain is the embedding store—a SQLite database in `~/.local/share/glitch/brain/`. It stores:
- Vector embeddings of code chunks, documents, and agent outputs
- Metadata (source file, timestamp, pipeline run ID)
- Tags (human-readable labels for filtering)

When a pipeline step includes a `brain:` directive, gl1tch embeds the step's output, stores it with metadata, and makes it available for retrieval in future runs. This solves the "lost context" problem: your AI workspace learns over time.

### Hot-path vs. Cloud

The **hot path** is the critical path in a pipeline that must be fast and private:
- Pipeline routing (which branch to take?)
- Agent dispatch (which agent should handle this?)
- Brain retrieval (what context is relevant?)

These always use local Ollama. Cloud LLMs are used for *expensive* tasks (complex analysis, code review, writing) where the latency tradeoff is worth it.

### BUSD — The Event Bus

BUSD (bus daemon) is an in-process event broker. All major subsystems publish events:
- Pipeline run starts/finishes
- Agent response ready
- Theme changed
- Cron entry triggered

Subscribers (the TUI, the Activity Feed, the trace exporter) listen on topics and update their views. This decouples components and makes the system testable.


## See Also

- [Philosophy](/docs/philosophy.md) — Design principles: local-first, ownership-centered, pipeline-driven
- [Architecture](/docs/architecture.md) — Three-column TUI layout, tmux session structure, component map
- [Console — The Switchboard](/docs/console.md) — Navigate the Deck, launch pipelines, run agents

