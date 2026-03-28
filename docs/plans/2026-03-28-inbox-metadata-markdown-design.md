# Inbox Metadata, Markdown Rendering, and Pipeline Bug Fix

**Date:** 2026-03-28

## Summary

Three related improvements to the inbox and pipeline systems:

1. **Bug fix:** Scheduled agent pipelines leak into the PIPELINES panel
2. **Pipeline runs in inbox:** Launches from the PIPELINES panel are not recorded in the store
3. **Metadata in detail view:** Pipeline file path and working directory shown in run detail
4. **Markdown rendering:** Stdout rendered as markdown in the detail modal with a toggle

---

## 1. Bug Fix — Scheduled Agent Pipelines in PIPELINES Panel

**Problem:** `writeSingleStepPipeline` writes auto-generated YAML to `~/.config/orcai/pipelines/`. `ScanPipelines` scans that directory and picks up every `*.pipeline.yaml` file, so agent-scheduled pipelines appear in the PIPELINES launcher.

**Fix:** Write auto-generated pipelines to `~/.config/orcai/pipelines/.agents/` instead. `ScanPipelines` already skips dotfiles and dot-directories, so no filter logic changes are needed.

---

## 2. Pipeline Runs in Inbox

**Problem:** `launchPendingPipeline` never calls `store.RecordRunStart`, so pipeline runs are not stored and do not appear in the inbox.

**Fix:**
- Change `store.RecordRunStart` signature to accept a `metadata string` parameter (JSON blob). Update the schema `INSERT` to write it.
- In `launchPendingPipeline`, call `store.RecordRunStart("pipeline", name, metadataJSON)` where metadata contains `pipeline_file` and `cwd`. Store the returned ID on the `jobHandle.storeRunID`.
- The existing `jobDoneMsg` and `jobFailedMsg` handlers already call `RecordRunComplete` when `storeRunID != 0` — no changes needed there.
- Update the agent `submitAgentJob` call to also pass metadata (cwd, pipeline file if applicable).

---

## 3. Metadata in Detail Modal

**Problem:** The detail modal (`buildRunContent`) shows timing and output but not where the run came from.

**Fix:** Parse `run.Metadata` JSON at render time. If `pipeline_file` or `cwd` fields are present, render them before the separator line using the same dim/fg color style as the existing `started`/`finished` lines:

```
pipeline:  ~/path/to.pipeline.yaml
cwd:       ~/projects/myrepo
```

Fields are only shown when non-empty. Paths are `~`-collapsed for readability.

---

## 4. Markdown Rendering in Detail Modal

**Problem:** Agent stdout is often markdown, but the detail modal renders it as raw text.

**Fix:**
- Add `inboxMarkdownMode bool` field to the switchboard `Model`. Default `true` when stdout contains markdown signals (`# `, `**`, ` ``` `).
- In `viewInboxDetail`, when `markdownMode` is true, pass stdout through `charmbracelet/glamour` with word wrap set to `innerW`. Split the rendered output on newlines and pass each through `boxRow()` as before so borders remain intact.
- Add `m` key in the detail view to toggle markdown mode. Update footer hint to show `[m]arkdown on/off`.

**Dependency:** Add `github.com/charmbracelet/glamour` to `go.mod`.

---

## Data Flow

```
launchPendingPipeline
  └─ store.RecordRunStart("pipeline", name, {"pipeline_file":..., "cwd":...})
       └─ jobHandle.storeRunID = id

jobDoneMsg / jobFailedMsg (unchanged)
  └─ store.RecordRunComplete(storeRunID, exitCode, stdout, stderr)

inbox poll → store.QueryRuns → buildInboxSection (list row)
  └─ enter → viewInboxDetail
       ├─ buildRunContent: parse metadata → show pipeline_file + cwd
       └─ glamour render if markdownMode
```

---

## Files Affected

- `internal/store/writer.go` — `RecordRunStart` signature + INSERT
- `internal/store/schema.go` — no changes (metadata column exists)
- `internal/switchboard/switchboard.go` — `writeSingleStepPipeline` dir, `launchPendingPipeline` store call, `inboxMarkdownMode` field, `m` key handler
- `internal/switchboard/inbox_detail.go` — `buildRunContent` metadata section, glamour rendering
- `go.mod` / `go.sum` — add glamour dependency
