## GLITCH Database Context

### Schema: runs table (read-only)
Columns: id (INTEGER PK), kind (TEXT), name (TEXT), started_at (INTEGER unix-ms),
finished_at (INTEGER unix-ms, nullable), exit_status (INTEGER, nullable),
stdout (TEXT), stderr (TEXT), metadata (TEXT JSON), steps (TEXT JSON array).
This table is READ-ONLY. Do not issue INSERT, UPDATE, or DELETE against it.

## Brain Notes (this run)

[write_doc] [type:research title:GL1TCH Switchboard console documentation tags:docs,console,switchboard,ui] Created console.md documenting the interactive TUI interface. Key coverage:
- Three-region layout: Pipeline Launcher (left), Activity Feed (center), keybinding bar (bottom)
- Navigation: tab/shift+tab for focus, j/k for selection, enter to launch, esc to close
- Chord shortcuts: ^spc prefixed commands (h=help, j=jump, c=new window, d=detach, etc.)
- Pipeline Launcher: Browse and run .pipeline.yaml files from left sidebar
- Agent Runner: Provider/model picker + inline prompt textarea, streams to Activity Feed
- Activity Feed: Real-time output with status badges (running/done/failed), timestamped entries
- Modals: Theme picker (T or ^spc t), Inbox Detail (enter on entry), Prompt Builder (^spc p)
- Status indicators: Chat panel subtitle, exit codes, provider availability warnings
- Practical workflows: pipeline testing loop, agent iteration, multi-window usage, result re-runs

References internal/console/deck.go and design docs from switchboard-control-center OpenSpec change.

> Do NOT modify the runs table.

---
BRAIN NOTE INSTRUCTION: Include a <brain> block somewhere in your response to persist an insight for future steps in this pipeline.

Use the <brain> tag with structured attributes to categorize your note:

  <brain type="research" tags="optional,comma,tags" title="Human readable title">
  Your insight, analysis, or structured data here.
  </brain>

Available types:
- research  — background info, context, references
- finding   — concrete discovery (bug, pattern, fact)
- data      — structured output (metrics, counts, lists)
- code      — code snippet or file path reference

The <tags> attribute is optional. The <title> attribute is recommended.

Example:
  <brain type="finding" tags="auth,security" title="Session token stored in plain text">
  Found that session tokens are written to ~/.glitch/session without encryption.
  File: internal/auth/session.go line 42.
  </brain>

The brain note will be stored and made available to subsequent agent steps with use_brain enabled.
---

<brain type="research" tags="docs,console,switchboard,ui" title="GL1TCH Switchboard console documentation">
Created console.md documenting the interactive TUI interface. Key coverage:
- Three-region layout: Pipeline Launcher (left), Activity Feed (center), keybinding bar (bottom)
- Navigation: tab/shift+tab for focus, j/k for selection, enter to launch, esc to close
- Chord shortcuts: ^spc prefixed commands (h=help, j=jump, c=new window, d=detach, etc.)
- Pipeline Launcher: Browse and run .pipeline.yaml files from left sidebar
- Agent Runner: Provider/model picker + inline prompt textarea, streams to Activity Feed
- Activity Feed: Real-time output with status badges (running/done/failed), timestamped entries
- Modals: Theme picker (T or ^spc t), Inbox Detail (enter on entry), Prompt Builder (^spc p)
- Status indicators: Chat panel subtitle, exit codes, provider availability warnings
- Practical workflows: pipeline testing loop, agent iteration, multi-window usage, result re-runs

References internal/console/deck.go and design docs from switchboard-control-center OpenSpec change.
</brain>

Done. I've created `console.md` with comprehensive documentation covering the GL1TCH Switchboard TUI interface, including:

- **Layout overview** with ASCII diagram showing the three-region structure
- **Navigation controls** — tab/shift-tab for focus, j/k for selection, enter/esc flow
- **Pipeline Launcher** — browse and launch `.pipeline.yaml` files with live streaming
- **Agent Runner** — provider/model picker + inline prompt input
- **Activity Feed** — timestamped output with status badges
- **Chord shortcuts** — full `^spc` command table
- **Modal workflows** — theme picker, Inbox Detail, Prompt Builder
- **Status indicators** — job badges, exit codes, provider availability
- **Practical patterns** — testing loops, agent iteration, multi-window workflows

The doc follows your typography rules, includes working examples, and cross-references related docs. All information is derived from the implementation files and OpenSpec design docs provided.

