# Gap #4 — Workflow promotion from the CLI

You are working in `/Users/stokes/Projects/gl1tch`. Your job is to close the chat→workflow promotion gap for CLI users: `glitch ask` today generates a pipeline on the fly but has no way to save it. Desktop has step-through; CLI has nothing. Fix the CLI half with the minimum viable promotion path.

## Step 1 — worktree

```bash
cd /Users/stokes/Projects/gl1tch
git check-ignore -q .worktrees && echo "ignored ok" || { echo "ABORT: .worktrees not ignored"; exit 1; }
git worktree add .worktrees/gap4-workflow-cli -b feature/workflow-cli-promotion
cd .worktrees/gap4-workflow-cli
go mod download
go test ./cmd/... ./internal/pipeline/... ./internal/router/... 2>&1 | tail -10
```

If baseline fails, stop and report.

## Background

Research from 2026-04-08:

- Saved workflows live at `.glitch/workflows/<name>.workflow.yaml` (see `cmd/workflow.go:44` and `pkg/glitchd/stepthrough.go:180-224`).
- `cmd/ask.go:99` calls `pipeline.DiscoverPipelines(workflowsDir)` on every invocation. No persistent registry; discovery is per-call.
- `internal/router/router.go:100` has a CacheDir-based embedding cache that SHA-256 fingerprints pipelines by name+description+trigger_phrases. New files in `.glitch/workflows/` show up on the next ask automatically.
- Desktop step-through: `pkg/glitchd/stepthrough.go:54-178` + `SaveStepThroughAs()` at `stepthrough.go:188-224`. CLI has no equivalent.
- `cmd/ask.go:122-123` — the generated pipeline for the current ask is in scope here. Today it runs once and is thrown away.
- `cmd/workflow.go:37-45` — `glitch workflow run <name>` already exists. That's the read side. We need the write side.

## The change

Two coordinated additions:

### 4a. `glitch ask --save-as <name>` flag

In `cmd/ask.go`, add a `--save-as` string flag. When set and the ask produces a runnable pipeline:

1. Run the pipeline as normal (do not gate execution on saving — user wants both behaviors).
2. After execution, serialize the in-scope pipeline to YAML using the existing marshal path that `SaveStepThroughAs` uses. Reuse that function if possible — if it's in `pkg/glitchd`, factor the YAML write into a smaller helper in `internal/pipeline/save.go` that both call sites use.
3. Write to `.glitch/workflows/<name>.workflow.yaml`.
4. If the file already exists, exit with a clear error — no overwrite without `--force`. Do not add `--force` in this pass; the error is enough.
5. Print the saved path so the user knows where to find it.

### 4b. `glitch workflow save` subcommand (thin layer)

In `cmd/workflow.go`, add a `save` subcommand that takes a pipeline YAML path and a name, copies it into `.glitch/workflows/<name>.workflow.yaml`. This is the manual path for users who have a pipeline file they want to promote without running it through `glitch ask`.

```
glitch workflow save --from ./my.pipeline.yaml --as linear-triage
```

## Shared helper

Create `internal/pipeline/save.go`:

```go
// SaveWorkflow writes a pipeline to .glitch/workflows/<name>.workflow.yaml.
// Returns an error if the target exists. The caller is responsible for
// resolving the workflows directory (it may live under a workspace).
func SaveWorkflow(workflowsDir, name string, p *Pipeline) (string, error)
```

It should:
- Validate `name` is a safe filename (no `/`, no `..`, non-empty).
- Create `workflowsDir` if missing.
- Reject if target exists.
- Marshal via `yaml.Marshal` with 2-space indent.
- Return the full path.

Both `cmd/ask.go` (save-as flag) and `cmd/workflow.go` (save subcommand) call this. `pkg/glitchd/stepthrough.go:SaveStepThroughAs` should also be refactored to call it — kill the duplication.

## Tests

- `internal/pipeline/save_test.go` — table-driven:
  - empty name → error
  - name with `/` → error
  - name with `..` → error
  - fresh save → file exists with expected content
  - existing file → error, original untouched
- `cmd/ask_test.go` — add a case that runs ask with `--save-as` against a stubbed pipeline and checks the file lands in the expected place. Reuse any existing ask test harness.
- `cmd/workflow_test.go` — if it exists, add a case for `workflow save`. If not, create the file with a minimal table-driven test.

## Router auto-indexing

You do not need to touch the router — the existing disk-discovery-per-ask path at `cmd/ask.go:99` will pick up the new file on the next invocation. Verify this in a final manual smoke test but do not add any explicit "reindex" call.

## Step — verify

```bash
go build ./...
go vet ./...
go test ./cmd/... ./internal/pipeline/... ./pkg/glitchd/... 2>&1 | tail -20

# Smoke test
./bin/glitch ask --save-as smoke-test "show me recent commits" || true
ls .glitch/workflows/smoke-test.workflow.yaml
rm .glitch/workflows/smoke-test.workflow.yaml
```

Do not commit the smoke-test artifact.

## Commit + report

```bash
git add -A
git status
git commit -m "feat(cmd): promote generated pipelines to saved workflows from CLI"
```

Report: commit SHA, whether the `SaveStepThroughAs` refactor worked cleanly or was skipped, and which discovery path picks up the new file.

## Hard rules

- Reuse the existing workflow directory discovery — do not hardcode `.glitch/workflows/`. Find the function that resolves it today (grep `workflowsDir` in `cmd/ask.go` and `cmd/workflow.go`) and call that.
- No `--force` flag. Overwriting existing workflows is out of scope.
- No background reindexing, no cron, no filesystem watcher. Discovery-per-ask is fine for pre-1.0.
- Do not touch `glitch-desktop/` — desktop promotion already works.
- Filename validation is a security boundary; reject `..`, `/`, empty, and anything over 100 chars.
