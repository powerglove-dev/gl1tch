---
name: zsh-history
category: shell
trigger:
  mode: interval
  every: 10m
sink:
  index: true
invoke:
  command: sh
  args:
    - -c
    - |
      tail -n 200 ~/.zsh_history 2>/dev/null \
        | sed 's/^: [0-9]*:[0-9];//' \
        | jq -R -c 'select(length > 0) | {type:"shell.cmd",source:"zsh",author:"user",message:.,timestamp:now}'
  parser: jsonl
---

# zsh-history

Indexes the most recent zsh shell commands so the assistant can answer
questions about what you ran in the terminal.

Use this when the user asks:

- "what did I run earlier"
- "what was that git command I used yesterday"
- "did I install X on this machine"

The capability tails the last 200 lines of `~/.zsh_history`, strips zsh's
extended-history timestamp prefix, and emits one JSONL document per command
into `glitch-events`. Re-runs every 10 minutes; duplicate commands across ticks
are tolerated because the brain dedupes by content when summarising.

Drop this file in `~/.config/glitch/capabilities/` to enable it. No code
changes required — the capability runner picks it up at boot.
