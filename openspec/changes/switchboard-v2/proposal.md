## Why

The Switchboard is the control center of ORCAI but currently lacks the focus, durability, and operational depth it needs: the ^spc n shortcut exposes a raw shell entry point that competes with the Switchboard's role; background tmux windows opened by pipelines are invisible and inaccessible from the UI; parallel job execution is not surfaced; the status bar lists all tmux windows instead of anchoring on the Switchboard; and the ollama/opencode plugins blindly pull models on every run even when they are already present. These issues collectively undermine the Switchboard-as-control-center vision established in v1.

## What Changes

- **Remove** the `^spc n` new-session chord; hide it from the status bar hints. New shell windows remain creatable via an explicit key inside the Switchboard.
- **Make `^spc t` always jump to window 0 (Switchboard)**; window 0 SHALL NOT be killable — only `q`/`ctrl+c` (quit ORCAI) terminates it.
- **Run pipelines and agents in background tmux windows** instead of in-process goroutines; a preview popup overlays the Switchboard showing live tail output; `Enter` jumps the user into that window.
- **Support parallel job execution**: multiple pipelines/agents can run concurrently; all are visible as live feed entries with their own background windows.
- **Clean up the tmux status bar**: show only `ORCAI 0:SWITCHBOARD` plus key hints and clock; all other windows are hidden from the status bar window list.
- **Guard model pulls in ollama and opencode plugins**: check whether the requested model is already present before invoking pull; skip pull when already available.
- **Add integration tests** for full pipeline execution and single-step agent execution using the llama and qwen models (free, local).

## Capabilities

### New Capabilities

- `switchboard-window-guard`: Window 0 is the permanent Switchboard; ^spc t always focuses it; window 0 cannot be killed; ^spc n is removed; new shell windows are still creatable via an explicit Switchboard action.
- `switchboard-tmux-runner`: Pipelines and agents run inside background tmux windows (one window per job); the Switchboard shows a live-tail preview popup on selection; Enter navigates the user into the job's tmux window.
- `status-bar-switchboard-only`: The tmux status bar shows only `ORCAI 0:SWITCHBOARD`, active key hints (new-shell, switchboard), and the clock; no other tmux windows appear in the status bar.
- `plugin-model-pull-guard`: Before pulling a model, ollama and opencode plugins check whether the model is already present locally and skip the pull if so.
- `pipeline-integration-tests`: Automated tests that launch full pipelines and single-step agent pipelines end-to-end, verifying output is produced and exit status is clean, using llama and qwen models.

### Modified Capabilities

- `status-bar-session-controls`: Remove ^spc n hint; replace with ^spc t switchboard-focus hint; status bar no longer lists non-home tmux windows.
- `pipeline-parallel-execution`: The Switchboard leverages the existing parallel DAG runner to support multiple concurrently active jobs, each with its own background tmux window and feed entry.

## Impact

- `internal/switchboard/` — add tmux-window launcher, preview popup overlay, parallel job tracking (replace in-process goroutine runner with tmux-window runner)
- `cmd/tmux_status.go` (or equivalent) — rewrite status-bar format to hide non-home windows and show only SWITCHBOARD hint + clock
- `internal/keybindings/` or `cmd/` tmux key table setup — remove ^spc n binding; ensure ^spc t sends focus to window 0
- `../orca-plugins/plugins/ollama/main.go` — add model-presence check before pull
- `../orca-plugins/plugins/opencode/main.go` — add model-presence check before pull
- `internal/pipeline/` and `internal/switchboard/` — integration test files using llama3 and qwen models
