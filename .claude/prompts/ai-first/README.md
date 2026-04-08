# AI-First Redesign — Worktree Prompts

Four self-contained prompts for the remaining gaps in gl1tch's AI-first redesign. Each prompt is designed to run in an isolated git worktree so they can be implemented in parallel or independently without stepping on each other.

Gaps #1 (observer workspace grounding) and #3 (centralized local model constant) were completed on main — see git log for commit. These four remain:

| File | Gap | Summary |
|---|---|---|
| `gap2-router-learning.md` | Router learning loop | Persist router picks + outcomes so the router can read its own history |
| `gap4-workflow-cli.md` | Workflow promotion from CLI | Let `glitch ask` save a generated pipeline as a reusable workflow |
| `gap5-session-memory.md` | Session memory | Persist assistant Q&A turns to SQLite with recency + semantic recall |
| `gap6-assistant-fallback.md` | Assistant CLI fallback | Commands with insufficient args fall back to the assistant instead of failing |

## How to run one

Copy the entire body of a prompt file and paste it as a single user message into a fresh Claude Code session started in `/Users/stokes/Projects/gl1tch`. The prompt begins with the worktree setup it expects Claude to perform.

## Ordering

All four are independent of each other. Recommended order if doing sequentially:

1. **gap2** — smallest surface area, lowest risk, unlocks telemetry for the others
2. **gap5** — builds on gap2's event-sink path and is the biggest UX win
3. **gap6** — pure cmd/ layer, no store schema changes
4. **gap4** — touches pipeline discovery + cmd/ask, most entangled

## Hard rules (apply to every prompt)

- Pre-1.0: no migration code, no schema repair, no backwards-compat shims — wipe and restart instead
- Local Ollama only, default model `capability.DefaultLocalModel` (do not hardcode `"qwen2.5:7b"` — that constant is now authoritative)
- Do not recreate anything under `internal/collector` — that package is deleted forever
- LLM never constructs shell commands; routing is by capability name + structured args
- Table-driven tests in the style of `.claude/skills/golang-testing`
