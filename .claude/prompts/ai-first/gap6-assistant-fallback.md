# Gap #6 — Assistant fallback for commands with insufficient args

You are working in `/Users/stokes/Projects/gl1tch`. Your job is to make gl1tch feel AI-first at the CLI layer: when a user runs a command with insufficient arguments, the command should route through `glitch assistant` with a synthesized prompt instead of failing with a usage string.

## Step 1 — worktree

```bash
cd /Users/stokes/Projects/gl1tch
git check-ignore -q .worktrees && echo "ignored ok" || { echo "ABORT: .worktrees not ignored"; exit 1; }
git worktree add .worktrees/gap6-assistant-fallback -b feature/assistant-fallback
cd .worktrees/gap6-assistant-fallback
go mod download
go test ./cmd/... 2>&1 | tail -10
```

If baseline fails, stop.

## Background

Research from 2026-04-08. The full cobra command inventory and their current zero-arg behavior:

| Command | File | Args | Zero-arg behavior |
|---|---|---|---|
| `ask [prompt]` | `cmd/ask.go` | MaximumNArgs(1) | fails: "a prompt is required" |
| `assistant [message]` | `cmd/assistant.go` | MinimumNArgs(1) | fails: "a message is required" |
| `observe [question]` | `cmd/observe.go:20-34` | MinimumNArgs(1) | fails |
| `game` | `cmd/game.go:29-32` | — | parent, no RunE, prints help |
| `workflow` | `cmd/workflow.go` | — | parent, no RunE, prints help |
| `security` | `cmd/security` | — | parent, no RunE, prints help |
| `chat` | `cmd/chat.go:41-59` | — | runs interactive REPL |

`glitch search foo` is not a registered command — cobra prints "unknown command". There is no implicit-to-assistant routing anywhere.

## The change

Three coordinated pieces:

### 6a. Shared fallback helper

Create `cmd/assistant_fallback.go`:

```go
// RunAssistantFallback dispatches a synthesized prompt through the
// same code path as `glitch assistant <prompt>`. Used by commands
// that want to route ambiguous or empty invocations through the AI
// surface instead of failing with a usage string.
//
// The synthesized prompt is built by the caller — this helper does
// not invent intent. It only plumbs the prompt into the assistant.
func RunAssistantFallback(cmd *cobra.Command, prompt string) error
```

Implementation: factor out the body of `cmd/assistant.go`'s RunE into a private function `runAssistant(ctx, prompt string) error`, then call it from both the existing `assistantCmd.RunE` and from `RunAssistantFallback`. Do not duplicate the router setup.

### 6b. Retro-fit the four worst offenders

Pick these four. Each gets a `RunE` (or `PreRunE` for commands that already have one) that detects insufficient args and calls `RunAssistantFallback` with a command-specific synthesized prompt:

1. **`observe`** — zero args today errors out. Change: if `len(args) == 0`, run `RunAssistantFallback(cmd, "observe my workspace and tell me what's happening")`.
2. **`game`** — no subcommand, no RunE today. Add a RunE that calls `RunAssistantFallback(cmd, "show me my current game status: streak, achievements, ICE")`.
3. **`workflow`** — no subcommand, no RunE today. Add a RunE that calls `RunAssistantFallback(cmd, "show me my saved workflows and which ones I ran most recently")`.
4. **`security`** — no subcommand, no RunE today. Add a RunE that calls `RunAssistantFallback(cmd, "show me recent security alerts for this workspace")`.

Do NOT touch `ask` or `assistant` themselves — they already point at the AI surface, and adding a fallback to `ask` would be circular.

Do NOT touch `chat` — it's an interactive REPL, different shape, out of scope.

### 6c. Help text update

Each fallback command should print a one-liner above the help output explaining the AI fallback:

```
game — run with no args to ask the assistant "show me my current game status"
```

Do this by setting `Long` on each command, not by overriding the help template.

## Tests

- `cmd/assistant_fallback_test.go` — new file. Table-driven:
  - `RunAssistantFallback` with empty prompt returns an error
  - `RunAssistantFallback` with a prompt calls the inner `runAssistant` (use a test-time override hook: a package-level `var runAssistantFn = runAssistant` and swap it in the test)
- For each of the four retro-fitted commands, add a test case that invokes the command with zero args and asserts `runAssistantFn` was called with the expected synthesized prompt. If `cmd/` already has a test harness pattern, reuse it; otherwise, put all four test cases in `cmd/assistant_fallback_test.go`.

## Verify

```bash
go build ./...
go vet ./...
go test ./cmd/... 2>&1 | tail -20

# Smoke test — all should route through assistant, not error
./bin/glitch observe
./bin/glitch game
./bin/glitch workflow
./bin/glitch security
```

Each smoke test should produce assistant output, not a cobra usage error. If any fails with usage, your RunE is not wired correctly.

## Commit + report

```bash
git add -A
git status
git commit -m "feat(cmd): route zero-arg commands through the assistant instead of usage errors"
```

Report: commit SHA, which four commands were retro-fitted, and one example of the synthesized prompt that routed through cleanly.

## Hard rules

- Do not invent synthesized prompts for commands you weren't asked to retro-fit. Four is the scope — observe, game, workflow, security.
- Do not modify `ask`, `assistant`, or `chat`.
- Do not add a generic "unknown command" interceptor that routes everything through the assistant. Cobra's unknown-command handling is separate and out of scope.
- The synthesized prompt is a constant string per command. Do not dynamically build it from flags or env. If the user wants to customize, they pass an explicit prompt to `glitch assistant`.
- Do not print "routing through assistant..." noise. The fallback should feel seamless — stdout should look like the assistant ran, not like a command caught an error.
- Reuse the existing assistant code path. If factoring out `runAssistant` forces you to change the public shape of any function, stop and reconsider — it should be a pure lift-and-shift into a private helper.
