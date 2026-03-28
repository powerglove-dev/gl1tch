# Inbox Metadata, Markdown Rendering, and Pipeline Bug Fix — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix scheduled agent pipelines leaking into the PIPELINES panel, record pipeline runs in the inbox, show pipeline-file/cwd metadata in the detail view, and render markdown stdout with a toggle.

**Architecture:** Four independent changes ordered by dependency. Store layer first (RecordRunStart gains metadata param), then pipeline integration, then display (metadata section + glamour markdown). Each task ships with tests before implementation.

**Tech Stack:** Go, BubbleTea, SQLite (`modernc.org/sqlite`), `charmbracelet/glamour` (already in go.mod, used in `internal/gitui/model.go`).

---

## Task 1: Fix — Scheduled Agent Pipelines Leaking into PIPELINES Panel

**Problem:** `writeSingleStepPipeline` writes auto-generated YAML to `~/.config/orcai/pipelines/`. `ScanPipelines` scans that dir and includes every `*.pipeline.yaml` — so scheduled-agent files appear in the launcher.

**Files:**
- Modify: `internal/switchboard/switchboard.go` (`writeSingleStepPipeline` function, ~line 626)

**Step 1: Identify the write path**

Open `internal/switchboard/switchboard.go` and find `writeSingleStepPipeline`. Currently:
```go
func writeSingleStepPipeline(name, providerID, modelID, prompt string) (string, error) {
    dir := pipelinesDir()
```

**Step 2: Change the write directory to a hidden subdirectory**

Change `dir` to write into `.agents/` inside `pipelinesDir()`. `ScanPipelines` already skips dotfiles and dot-dirs so no filter logic changes are needed.

```go
func writeSingleStepPipeline(name, providerID, modelID, prompt string) (string, error) {
    dir := filepath.Join(pipelinesDir(), ".agents")
```

**Step 3: Verify the build passes**

```bash
cd /Users/stokes/Projects/orcai && go build ./...
```
Expected: no errors.

**Step 4: Commit**

```bash
git add internal/switchboard/switchboard.go
git commit -m "fix: write scheduled-agent pipelines to .agents/ subdir to hide from launcher"
```

---

## Task 2: Add `metadata` Parameter to `RecordRunStart`

The `store.Run.Metadata` field and column already exist but are never populated. This task threads a metadata string through from callers.

**Files:**
- Modify: `internal/store/writer.go` (line 52–68)
- Modify: `internal/cron/scheduler.go` (interface line 20, call line 161)
- Modify: `internal/pipeline/runner.go` (call line 81)
- Modify: `internal/store/store_test.go` (8 call sites)

**Step 1: Write a failing test for metadata persistence**

In `internal/store/store_test.go`, add after `TestRecordRunStart`:

```go
func TestRecordRunStart_WithMetadata(t *testing.T) {
    s := openTestStore(t)

    id, err := s.RecordRunStart("pipeline", "meta-test", `{"cwd":"/tmp","pipeline_file":"/tmp/foo.yaml"}`)
    if err != nil {
        t.Fatalf("RecordRunStart: %v", err)
    }

    runs, err := s.QueryRuns(1)
    if err != nil {
        t.Fatalf("QueryRuns: %v", err)
    }
    if len(runs) == 0 {
        t.Fatal("want 1 run, got 0")
    }
    if runs[0].ID != id {
        t.Errorf("want id %d, got %d", id, runs[0].ID)
    }
    if runs[0].Metadata != `{"cwd":"/tmp","pipeline_file":"/tmp/foo.yaml"}` {
        t.Errorf("want metadata blob, got %q", runs[0].Metadata)
    }
}
```

**Step 2: Run the test to verify it fails**

```bash
cd /Users/stokes/Projects/orcai && go test ./internal/store/... -run TestRecordRunStart_WithMetadata -v
```
Expected: compile error — `RecordRunStart` called with 3 args, wants 2.

**Step 3: Update `RecordRunStart` signature and INSERT**

In `internal/store/writer.go`, change the function signature and the INSERT:

```go
// RecordRunStart inserts a new in-flight run row and returns its ID.
// started_at is recorded in unix milliseconds using Go's clock for millisecond precision.
// metadata is an optional JSON blob (pass "" to omit).
func (s *Store) RecordRunStart(kind, name, metadata string) (int64, error) {
    startedAt := time.Now().UnixMilli()
    var id int64
    err := s.writer.send(func(db *sql.DB) error {
        res, err := db.Exec(
            `INSERT INTO runs (kind, name, started_at, metadata) VALUES (?, ?, ?, ?)`,
            kind, name, startedAt, metadata,
        )
        if err != nil {
            return err
        }
        id, err = res.LastInsertId()
        return err
    })
    return id, err
}
```

**Step 4: Update the `StoreWriter` interface in cron**

In `internal/cron/scheduler.go` line 20:

```go
type StoreWriter interface {
    RecordRunStart(kind, name, metadata string) (int64, error)
    RecordRunComplete(id int64, exitStatus int, stdout, stderr string) error
}
```

And line 161, pass `""` for metadata (cron doesn't have it yet):

```go
id, err := s.store.RecordRunStart(entry.Kind, entry.Name, "")
```

**Step 5: Update pipeline runner**

In `internal/pipeline/runner.go` line 81, pass `""`:

```go
id, err := cfg.store.RecordRunStart("pipeline", p.Name, "")
```

**Step 6: Update all test call sites in `store_test.go`**

Find every `s.RecordRunStart(` in `internal/store/store_test.go` and add `""` as the third argument. There are 8 sites. Example:

```go
// before
id, err := s.RecordRunStart("pipeline", "my-pipeline")
// after
id, err := s.RecordRunStart("pipeline", "my-pipeline", "")
```

Do the same for the concurrent test goroutine on ~line 239.

**Step 7: Update the agent call site in switchboard**

In `internal/switchboard/switchboard.go` line 2112, pass `""` for now (agent metadata wired in Task 4):

```go
if runID, err := m.store.RecordRunStart("agent", title, ""); err == nil {
```

**Step 8: Run all tests to verify they pass**

```bash
cd /Users/stokes/Projects/orcai && go test ./internal/store/... ./internal/cron/... ./internal/pipeline/... -v
```
Expected: all pass including `TestRecordRunStart_WithMetadata`.

**Step 9: Commit**

```bash
git add internal/store/writer.go internal/store/store_test.go \
        internal/cron/scheduler.go internal/pipeline/runner.go \
        internal/switchboard/switchboard.go
git commit -m "feat: add metadata param to RecordRunStart, persist JSON blob to store"
```

---

## Task 3: Record Pipeline Runs in the Inbox

`launchPendingPipeline` launches tmux-window-mode jobs but never calls `RecordRunStart`, so pipeline runs are invisible in the inbox. The existing `jobDoneMsg`/`jobFailedMsg` handlers already call `RecordRunComplete` when `storeRunID != 0`, so only the start call is missing.

**Files:**
- Modify: `internal/switchboard/switchboard.go` (`launchPendingPipeline`, ~line 1910)

**Step 1: Add a helper to build run metadata JSON**

At the top of `launchPendingPipeline` (or as a small package-level helper near the function), add:

```go
// runMetadataJSON returns a compact JSON blob for run metadata.
// Both fields are optional; empty strings are omitted.
func runMetadataJSON(pipelineFile, cwd string) string {
    // Avoid encoding/json import for a trivial case.
    switch {
    case pipelineFile != "" && cwd != "":
        return fmt.Sprintf(`{"pipeline_file":%q,"cwd":%q}`, pipelineFile, cwd)
    case pipelineFile != "":
        return fmt.Sprintf(`{"pipeline_file":%q}`, pipelineFile)
    case cwd != "":
        return fmt.Sprintf(`{"cwd":%q}`, cwd)
    default:
        return ""
    }
}
```

**Step 2: Call `RecordRunStart` in `launchPendingPipeline`**

In `launchPendingPipeline` (~line 1956), where the `jobHandle` is created:

```go
// before
m.activeJobs[feedID] = &jobHandle{id: feedID, cancel: cancel, ch: ch, tmuxWindow: windowName, logFile: logFile}

// after
jh := &jobHandle{id: feedID, cancel: cancel, ch: ch, tmuxWindow: windowName, logFile: logFile}
if m.store != nil {
    if runID, err := m.store.RecordRunStart("pipeline", name, runMetadataJSON(yamlPath, cwd)); err == nil {
        jh.storeRunID = runID
    }
}
m.activeJobs[feedID] = jh
```

**Step 3: Build and verify**

```bash
cd /Users/stokes/Projects/orcai && go build ./...
```
Expected: no errors.

**Step 4: Commit**

```bash
git add internal/switchboard/switchboard.go
git commit -m "feat: record pipeline runs in inbox store with metadata"
```

---

## Task 4: Wire Agent Run Metadata into the Store

While we're here, pass cwd to the agent `RecordRunStart` call so the detail view can show it.

**Files:**
- Modify: `internal/switchboard/switchboard.go` (`submitAgentJob`, ~line 2112)

**Step 1: Locate the agent RecordRunStart call**

In `submitAgentJob` (~line 2112):
```go
if runID, err := m.store.RecordRunStart("agent", title, ""); err == nil {
```

**Step 2: Pass cwd metadata**

`cwd` is resolved a few lines above (line ~2065). Add the metadata:

```go
if runID, err := m.store.RecordRunStart("agent", title, runMetadataJSON("", cwd)); err == nil {
```

**Step 3: Build and verify**

```bash
cd /Users/stokes/Projects/orcai && go build ./...
```

**Step 4: Commit**

```bash
git add internal/switchboard/switchboard.go
git commit -m "feat: store cwd metadata on agent runs"
```

---

## Task 5: Show Metadata in the Inbox Detail Modal

`buildRunContent` in `inbox_detail.go` currently shows timing/exit-status + stdout/stderr. Add a metadata block between the header and the separator.

**Files:**
- Modify: `internal/switchboard/inbox_detail.go` (`buildRunContent`, line 15)

**Step 1: Add a `parseRunMetadata` helper**

Add near the top of `inbox_detail.go` (after the imports):

```go
import (
    // existing imports ...
    "encoding/json"
    "os"
    "strings"
)

type runMeta struct {
    PipelineFile string `json:"pipeline_file"`
    CWD          string `json:"cwd"`
}

func parseRunMetadata(raw string) runMeta {
    if raw == "" {
        return runMeta{}
    }
    var m runMeta
    _ = json.Unmarshal([]byte(raw), &m)
    return m
}

// collapseTilde replaces the user home dir prefix with "~" for display.
func collapseTilde(path string) string {
    home, err := os.UserHomeDir()
    if err != nil || home == "" {
        return path
    }
    if strings.HasPrefix(path, home) {
        return "~" + path[len(home):]
    }
    return path
}
```

**Step 2: Render metadata in `buildRunContent`**

In `buildRunContent`, after the `exit status` line and before the separator `─` line, insert:

```go
// Metadata (pipeline file, cwd)
meta := parseRunMetadata(run.Metadata)
if meta.PipelineFile != "" {
    sb.WriteString(dim.Render("pipeline: ") + fg.Render(collapseTilde(meta.PipelineFile)) + "\n")
}
if meta.CWD != "" {
    sb.WriteString(dim.Render("cwd:      ") + fg.Render(collapseTilde(meta.CWD)) + "\n")
}
```

**Step 3: Build and verify**

```bash
cd /Users/stokes/Projects/orcai && go build ./...
```

**Step 4: Commit**

```bash
git add internal/switchboard/inbox_detail.go
git commit -m "feat: show pipeline_file and cwd metadata in inbox detail view"
```

---

## Task 6: Markdown Rendering in the Inbox Detail Modal

Use the already-imported `charmbracelet/glamour` (see `internal/gitui/model.go:1042` for usage pattern) to render stdout as markdown in the detail modal. Add a toggle so users can switch between rendered and raw.

**Files:**
- Modify: `internal/switchboard/switchboard.go` (model fields, key handler)
- Modify: `internal/switchboard/inbox_detail.go` (`viewInboxDetail`, `buildRunContent`)

**Step 1: Add `inboxMarkdownMode` field to Model**

In `internal/switchboard/switchboard.go`, find the Model struct fields (around line 284 where `inboxDetailOpen` is). Add:

```go
inboxMarkdownMode bool // true = render stdout as markdown in detail view
```

**Step 2: Auto-detect markdown when opening the detail view**

In `handleKey` where `m.inboxDetailOpen = true` is set (~line 1253), add auto-detection after setting the flag:

```go
m.inboxDetailOpen = true
m.inboxDetailIdx = m.inboxPanel.selectedIdx
m.inboxDetailScroll = 0
// Auto-enable markdown if the run's stdout looks like markdown.
if m.inboxDetailIdx < len(runs) {
    m.inboxMarkdownMode = looksLikeMarkdown(runs[m.inboxDetailIdx].Stdout)
}
```

Add the helper near the other helpers (e.g., near `runMetadataJSON`):

```go
// looksLikeMarkdown returns true when s contains common markdown signals.
func looksLikeMarkdown(s string) bool {
    return strings.Contains(s, "# ") ||
        strings.Contains(s, "**") ||
        strings.Contains(s, "```")
}
```

**Step 3: Handle `m` key toggle in the detail view**

In `handleKey`, inside the `if m.inboxDetailOpen` block (~line 1022), add a case for `"m"`:

```go
case "m":
    m.inboxMarkdownMode = !m.inboxMarkdownMode
    m.inboxDetailScroll = 0
    return m, nil
```

**Step 4: Pass `markdownMode` to `viewInboxDetail`**

In `internal/switchboard/switchboard.go` where `viewInboxDetail` is called (~line 2385):

```go
// before
overlay = m.viewInboxDetail(w, h)
// after
overlay = m.viewInboxDetail(w, h, m.inboxMarkdownMode)
```

**Step 5: Update `viewInboxDetail` signature and thread markdown into `buildRunContent`**

In `internal/switchboard/inbox_detail.go`:

```go
// Change signature
func (m Model) viewInboxDetail(w, h int, markdownMode bool) string {

// Pass markdownMode to content builder
content := buildRunContent(run, mc, markdownMode)
```

**Step 6: Update `buildRunContent` signature and add glamour rendering**

```go
import (
    // add
    "github.com/charmbracelet/glamour"
)

func buildRunContent(run store.Run, mc modalColors, markdownMode bool) string {
    // ... existing timing/exit/metadata lines ...

    // Stdout — render as markdown if mode is on, else raw
    if run.Stdout != "" {
        if markdownMode {
            rendered, err := glamour.NewTermRenderer(
                glamour.WithStandardStyle("dark"),
                glamour.WithWordWrap(80),
            )
            if err == nil {
                out, rerr := rendered.Render(run.Stdout)
                if rerr == nil {
                    sb.WriteString(out)
                } else {
                    sb.WriteString(run.Stdout)
                    if !strings.HasSuffix(run.Stdout, "\n") {
                        sb.WriteString("\n")
                    }
                }
            }
        } else {
            sb.WriteString(run.Stdout)
            if !strings.HasSuffix(run.Stdout, "\n") {
                sb.WriteString("\n")
            }
        }
    } else {
        sb.WriteString(dim.Render("(no stdout)") + "\n")
    }
```

Note: glamour adds its own padding/newlines. The word-wrap width `80` is a safe default; ideally pass `innerW` from `viewInboxDetail`. See `internal/gitui/model.go:1042–1045` for the existing usage pattern.

**Step 7: Update the footer hint to show markdown toggle**

In `viewInboxDetail`, update the footer:

```go
mdHint := ""
if markdownMode {
    mdHint = accentStyle.Render("[m]") + dimStyle.Render("arkdown on  ")
} else {
    mdHint = dimStyle.Render("[m]arkdown off  ")
}

if total > visibleH {
    scrollHint := accentStyle.Render("j/k  [/]") + dimStyle.Render(" scroll  ")
    keyHints := dimStyle.Render("[n]ext  [p]rev  [q]uit")
    footer = lipgloss.NewStyle().Foreground(mc.dim).
        Width(innerW).Padding(0, 1).
        Render(scrollHint + mdHint + keyHints)
} else {
    footer = lipgloss.NewStyle().Foreground(mc.dim).
        Width(innerW).Padding(0, 1).
        Render(mdHint + dimStyle.Render("[n]ext  [p]rev  [q]uit"))
}
```

**Step 8: Build and run tests**

```bash
cd /Users/stokes/Projects/orcai && go build ./... && go test ./...
```
Expected: all pass.

**Step 9: Commit**

```bash
git add internal/switchboard/switchboard.go internal/switchboard/inbox_detail.go
git commit -m "feat: add markdown rendering toggle to inbox detail view"
```

---

## Verification

After all tasks, manually verify:
1. Schedule an agent run → it should NOT appear in the PIPELINES panel
2. Launch a pipeline → it should appear in the INBOX with a green/red dot
3. Open the inbox detail for the pipeline run → should show `pipeline:` and `cwd:` lines
4. Open a detail for an agent run that returns markdown → should auto-render; press `m` to toggle raw
5. Run `go test ./...` — all green
