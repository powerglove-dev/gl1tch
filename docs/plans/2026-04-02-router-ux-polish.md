# Router & UX Polish — Session Notes 2026-04-02

Changes shipped in this session. No pending tasks — all items are implemented and committed.

---

## 1. Router redesign — explicit-invocation-only pipeline dispatch

**Files:** `internal/router/router.go`, `internal/router/classify.go`, `internal/router/intent_test.go`, `internal/router/router_test.go`, `internal/router/contract_test.go`

`isImperativeInput` is now a hard gate that runs **before** any embedding lookup or LLM call. If the prompt does not start with an explicit run-verb the router returns `none` immediately — no Ollama call, no latency.

Accepted run-verbs: `run`, `execute`, `launch`, `rerun`, `re-run`, `start`, `trigger`, `kick off`, `kick-off`.

Everything else — questions (`why is the build failing?`), observations (`looks like there are conflicts`), generic task requests (`please review my PR`, `improve the docs`) — is returned as `none` so the AI handles it directly.

`buildPrompt` in `classify.go` was updated to match: the LLM prompt now describes the same two-step intent gate with inline examples covering both "always NONE" and explicit-invocation cases.

All router tests updated to reflect the new contract.

---

## 2. `/quit` kills tmux

**File:** `internal/console/deck.go`

`glitchQuitMsg` handler now calls `pkill tmux` before `tea.Quit`. All terminal sessions (including any detached workers) are torn down on exit.

---

## 3. Tilde expansion in pipeline CWD

**File:** `internal/console/jobwindow.go`

`createJobPane` calls `expandTilde()` on `startDir` before passing it to `cd` and the `tmux -c` flag. Paths like `/cwd ~/Projects/foo` now work correctly.

---

## 4. Esc cancels streaming

**File:** `internal/console/glitch_panel.go`

Pressing `Esc` while GL1TCH is responding:
- Calls `p.cancel()` to cancel the in-flight context
- Sets `p.streaming = false`, resets `animFrame`
- Flushes any partial `streamBuf` into the message history via `upsertStreamEntry`
- Restores input focus via `p.setFocused(true)`

If not streaming, `Esc` behaves as before (unfocus).

---

## 5. Expanded `isToolCallLine` filter

**File:** `internal/console/glitch_panel.go`

`isToolCallLine` now catches additional patterns emitted by CLI agent backends so only prose reaches the chat view:

| Pattern | Example |
|---|---|
| `✓ ` / `✗ ` unicode variants | `✓ Read file` |
| ASCII `x TitleCase (type)` failed-tool header | `x Read (tool)` |
| 2-space-indented continuation/body lines | `  /path/to/file` |
| `N lines...` count summaries | `42 lines...` |
| `Total usage est:` / `API time spent:` stats footer | |
| `Output too large` / `Permission denied and could not` noise | |

---

## 6. Terminal pane utilities + `/terminal` subcommand

**File:** `internal/console/jobwindow.go`

New types and helpers:

- `terminalPane` struct — holds `id`, `index`, `command`, `size` for a non-glitch pane
- `listTerminalPanes()` — lists all panes in the current tmux window except the gl1tch pane (identified by `TMUX_PANE`)

These back the `/terminal` subcommand (`list`, `kill`, `focus`, `equalize`, etc.) registered in `glitch_panel.go`.

The `^spc j` hint was removed from the tmux status bar (replaced by `/terminal`). The underlying jump-window chord remains functional.

---

## 7. Even-horizontal pane layout

**File:** `internal/console/glitch_panel.go`

Pane equalization now uses `even-horizontal` instead of `tiled`. This keeps pipeline panes in a side-by-side layout that matches how `createJobPane` splits (horizontal first, vertical for subsequent jobs).
