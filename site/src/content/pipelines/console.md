---
title: "Console"
description: "Talk to your AI assistant, launch pipelines, and manage your session — all from one conversation screen."
order: 2
---

gl1tch is a conversation. You open it in your terminal, type what you want, and your assistant handles it — whether that's answering a question, running a pipeline, or kicking off an automation. Everything happens in one screen. Your session keeps running even when you step away.

Launch it:

```bash
glitch
```

The input prompt is at the bottom. Type anything and press `Enter`.


## Talking to Your Assistant

Ask questions in plain English:

```text
why did the backup pipeline fail?
what changed in the last 10 commits?
explain this error: connection refused on port 5432
```

Give direct commands:

```text
run my standup pipeline
show me my saved pipelines
what pipelines do I have?
```

Your assistant knows your current working directory, your recent pipeline runs, and your brain context. It answers based on what's actually in your project — not generic advice.


## Slash Commands

Slash commands take immediate action. Type `/` to see autocomplete.

### Pipelines

```bash
/pipeline standup          # run the "standup" pipeline
/pipeline pr-triage        # run any named pipeline
/rerun standup             # re-run the last run of a pipeline
```

### Terminal Splits

Open a terminal pane alongside your assistant without leaving gl1tch:

```bash
/terminal                  # new pane, 25% bottom
/terminal 50%              # 50% split
/terminal left 40%         # left side, 40% width
/terminal ~/projects/myapp # pane opens in that directory
/terminal htop             # pane runs a command directly
```

### Sessions

Keep separate conversation threads without losing context:

```bash
/session new debug         # create a session named "debug"
/session debug             # switch to it (shorthand: /s debug)
```

Each session has its own full history. Active session shows `●` in the footer.

### Switching Models

```bash
/model ollama/qwen2.5-coder:latest
/model claude/claude-sonnet-4-6
/models                    # open the model picker
```

### Other

```bash
/cwd ~/projects/myapp      # change working directory
/clear                     # clear the conversation
/help                      # show command reference
/quit                      # exit
```


## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Esc` | Unfocus input |
| `↑` / `↓` | Navigate input history |
| `/help` | Show all commands |

### Chord Shortcuts

Press `ctrl+space`, then the key:

| Chord | Action |
|-------|--------|
| `ctrl+space d` | Detach (session keeps running) |
| `ctrl+space r` | Reload gl1tch |
| `ctrl+space a` | Focus assistant input |
| `ctrl+space p` | Open pipeline builder |
| `ctrl+space b` | Open brain editor |


## Detaching and Reattaching

Your session keeps running after you detach. Scheduled pipelines fire, automations complete, results accumulate. Come back and it's all there.

Detach: `ctrl+space d`

Reattach from your shell:

```bash
glitch attach
```

> **TIP:** Run long pipelines, detach, and come back to results. You don't have to watch it run.


## Examples

### Ask about a failed run

```text
why did the standup pipeline fail last night?
```

Your assistant sees the run output, exit code, and step timing. It tells you exactly what went wrong.

### Run a pipeline and ask about the output

```text
run pr-triage
```

Once it finishes, ask:

```text
which PR should I look at first?
```

Your assistant uses the pipeline output as context for its answer.

### Work in parallel with a terminal split

```text
/terminal 40% right
```

A pane opens on the right. Run tests, edit files, do whatever — your assistant stays on the left, ready for questions.


## See Also

- [Your First Pipeline](/docs/pipelines/quickstart) — Get running in five minutes
- [Pipelines](/docs/pipelines/pipelines) — Write and chain pipeline steps
- [Brain](/docs/pipelines/brain) — Teach your assistant about your projects
- [Scheduling](/docs/pipelines/cron) — Run pipelines automatically
