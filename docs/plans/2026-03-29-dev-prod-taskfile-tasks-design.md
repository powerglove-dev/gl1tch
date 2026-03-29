# Design: `dev` and `prod` Taskfile Tasks

**Date:** 2026-03-29
**Status:** Approved

## Problem

No single task covers a full clean-slate dev iteration (rebuild from source + new-user state) or a clean daily-driver prod startup (fresh install + non-destructive run).

## Design

### `dev` task — new-user clean build

Sequence:

1. Kill tmux sessions `orcai` and `orcai-cron` (soft-fail if not running)
2. Backup `~/.config/orcai/` to `~/.config/orcai.bak.YYYYMMDDHHMMSS`, keep last 5 backups
3. Backup `~/.local/share/orcai/` to `~/.local/share/orcai.bak.YYYYMMDDHHMMSS`, keep last 5 backups
4. Wipe `~/.config/orcai/` and `~/.local/share/orcai/` entirely
5. Delete `bin/orcai` and `bin/orcai-debug`
6. `go build -o bin/orcai .`
7. Run `./bin/orcai`

**Intent:** Simulate a first-run experience against a freshly compiled binary. State is backed up, never destroyed outright.

### `prod` task — daily driver

Sequence:

1. `go install .` (builds from source, installs to `GOPATH/bin`)
2. Ensure `~/.local/bin/orcai` symlink points to `GOPATH/bin/orcai`
3. Run `orcai` (installed binary, non-destructive — all state preserved)

**Intent:** Ship the latest source as your stable binary without touching runtime state.

## Key Decisions

- `dev` runs `./bin/orcai` (local build); `prod` runs the installed `orcai` — no cross-contamination.
- Backup pruning keeps last 5 per directory, consistent with existing `run:clean` db backup style.
- Both tasks are flat (not namespaced under `run:`), optimizing for daily ergonomics.
- Existing tasks (`run`, `run:clean`, `install`) are left untouched.
