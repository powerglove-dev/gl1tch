## Context

The Switchboard (`internal/switchboard/`) is a full-screen BubbleTea TUI that currently runs as a tmux `display-popup` (via `^spc t`). Window 0 is the sysop/switchboard process but can be killed by the user; ^spc n opens a provider picker that bypasses the Switchboard entirely; the status bar lists all tmux windows making the UI noisy; the existing `activeJob` field allows only one job at a time; and the ollama/opencode plugins unconditionally pull models on every invocation. The integration test suite covers unit logic but not end-to-end pipeline/agent execution.

The bootstrap is configured in `internal/bootstrap/bootstrap.go`. Tmux chord bindings and status-bar format strings are assembled there and applied via a temp conf file at session start.

## Goals / Non-Goals

**Goals:**
- Window 0 is the permanent Switchboard; it runs as a normal tmux window (not a popup), is keyboard-accessible via ^spc t, and cannot be killed
- Parallel jobs: multiple pipeline/agent runs can be active simultaneously, each in its own background tmux window
- Status bar shows only `ORCAI 0:SWITCHBOARD`, active key hints (^spc t, ^spc c), and clock — no window list
- ^spc n removed; new shell windows remain creatable via ^spc c (already bound to `new-window`)
- Preview popup inside the Switchboard TUI shows live tail of selected job's log; Enter navigates to that tmux window
- ollama and opencode plugins check model presence before pulling
- Integration tests exercise full pipeline execution and single-step agent pipelines

**Non-Goals:**
- Changing the pipeline YAML schema or runner DAG logic
- Replacing the BubbleTea TUI framework
- Supporting remote/SSH tmux sessions differently

## Decisions

### 1. Switchboard runs in window 0, not as a popup

**Current**: `^spc t` fires `display-popup -E -w 100% -h 100% orcai-sysop`. The switchboard runs inside a floating popup and is ephemeral.

**Decision**: Change `^spc t` to `select-window -t orcai:0`. The switchboard binary (`orcai-sysop`) is started in window 0 at session bootstrap (already the case). Removing `display-popup` from the binding makes window 0 the canonical home.

**Why not popup**: A popup can be dismissed accidentally, hides the underlying window, and does not allow the user to "enter" a background job window from within it without awkward nesting.

**Rationale**: The Switchboard IS window 0. ^spc t should focus it, not re-spawn it.

### 2. Window 0 kill protection via tmux `before-kill-window` hook

**Current**: Nothing prevents `kill-window` or `kill-pane` on window 0.

**Decision**: Register a tmux `set-hook -g before-kill-window` that checks `#{window_index}` and cancels the kill if the target is window 0 in the `orcai` session. Similarly guard `kill-pane` when the last pane in window 0 would close it. The `^spc x` (kill-pane) and `^spc X` (kill-window) chord bindings are replaced with guarded variants that refuse to act on window 0.

**Alternative considered**: Override `confirm-before-kill-window` — but that only adds a prompt, doesn't block.

### 3. Status bar hides window list; shows only ORCAI + hints + clock

**Current**: `window-status-format` and `window-status-current-format` are set in bootstrap, so all windows appear in the centre of the status bar.

**Decision**: Set `status-justify left` and override `window-status-format ""` and `window-status-current-format ""` so no windows appear. `status-left` becomes `" ORCAI 0:SWITCHBOARD "` and `status-right` becomes `"^spc t switchboard  ^spc c new-shell   %H:%M "` (removing ^spc n and ^spc p references).

### 4. Parallel jobs: activeJob → activeJobs map

**Current**: `m.activeJob *jobHandle` — single slot; Enter is blocked while any job runs.

**Decision**: Replace with `m.activeJobs map[string]*jobHandle` keyed by feed ID. Multiple concurrent jobs are allowed. The "busy" guard is removed from Enter in the launcher and quick-run sections. Each job owns its own background tmux window and log file.

**Why map not slice**: Feed entries are already keyed by ID string; O(1) lookup on job termination.

### 5. Preview popup is the existing debugPopup, renamed and focused

**Current**: `m.debugPopupOpen bool` / `m.debugPopupJobID string` — a debug overlay showing tail output. Activated from the signal board (^d or Enter on a signal board row).

**Decision**: Rename the concept to "preview popup"; activate it when the user highlights a feed entry in the activity feed and presses Enter (or space). The popup renders the last N lines of the job's log file, overlaid on the Switchboard using a lipgloss border box sized ~80% width, centred. A second Enter (while the popup is open) closes the popup and calls `tmux select-window -t <window>` to navigate into the job's window.

**Why not navigate immediately on first Enter**: The preview gives the user a chance to see what's happening before committing to leaving the Switchboard.

### 6. Model pull guard: proactive check via `ollama list`

**Ollama plugin (`orcai-ollama/main.go`)**: Currently, on HTTP 404 from the local server it runs `ollama pull`. This is reactive. Change to call `ollama list` once at startup, parse the model names, and skip the pull entirely if the model is already present. The 404 fallback remains as a safety net.

**Opencode plugin (`orcai-opencode/main.go`)**: `pullOllamaModel` always runs `ollama pull <name>`. Change to call `ollama list` first; skip pull if the model is present.

**Why `ollama list` not `/api/tags` HTTP**: Simpler, no port dependency, same output, consistent with the approach already taken for listing models.

### 7. Integration tests use real local models (llama / qwen)

**Decision**: Add `internal/integration/` (or `internal/switchboard/integration_test.go`) with `//go:build integration` tag. Tests call `pipeline.Load` + `pipeline.Run` with test pipeline YAML files that use `llama3.2` and `qwen2.5` models via the ollama provider. Tests assert exit with no error and non-empty output. A separate agent test builds a single-step pipeline inline.

**Why build tag**: Avoids running heavy model tests in standard `go test ./...`. CI can opt in with `-tags integration`.

**Why llama and qwen**: Free local models; user confirmed these are available.

## Risks / Trade-offs

- [Risk] tmux hook for kill protection may interfere with legitimate user `kill-window` calls outside window 0 → Mitigation: hook checks `#{window_index}` and `#{session_name}` before blocking
- [Risk] Parallel jobs consume more tmux windows and log files → Mitigation: add a cap (default 8, matching pipeline max_parallel); display warning when cap reached
- [Risk] Integration tests require ollama running with models pulled → Mitigation: tests check `ollama list` and skip cleanly (`t.Skip`) if required model not found
- [Risk] Status bar with blank window-status-format loses discoverability of background job windows → Mitigation: the Switchboard feed shows all active windows; the user navigates from there

## Migration Plan

1. Bootstrap change (remove ^spc n, change ^spc t, update status-bar format) — applied at next `orcai start` / session reload
2. Switchboard binary continues to be placed in window 0 at bootstrap — no change to start sequence
3. Plugin changes (model pull guard) are backwards-compatible — behaviour only changes when model is already present
4. Integration tests are opt-in via build tag — no CI disruption

## Open Questions

- Should `^spc n` binding be removed entirely from the chord table or just hidden from the status bar? (Proposed: remove entirely — avoids confusion)
- Should the preview popup log tail line count be configurable or hard-coded at 30 lines? (Proposed: hard-code 30 for now)
