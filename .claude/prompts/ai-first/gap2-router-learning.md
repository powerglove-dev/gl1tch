# Gap #2 — Router learning loop

You are working in `/Users/stokes/Projects/gl1tch`. Your job is to add a learning loop to `capability.Router` so every pick and its outcome is persisted, and so future picks can read recent picks as additional context.

## Step 1 — set up an isolated worktree

```bash
cd /Users/stokes/Projects/gl1tch
git check-ignore -q .worktrees && echo "ignored ok" || { echo "ABORT: .worktrees not ignored"; exit 1; }
git worktree add .worktrees/gap2-router-learning -b feature/router-learning
cd .worktrees/gap2-router-learning
go mod download
go test ./internal/capability/... ./internal/store/... ./internal/router/... 2>&1 | tail -10
```

If baseline tests fail, report and stop — do not proceed.

## Background

Research from the research phase (2026-04-08):

- `internal/capability/router.go:73-96` — `Pick()` asks qwen2.5:7b once, returns a capability name. No persistence.
- `internal/capability/router.go:113` — `Route()` invokes the runner. No outcome recording.
- `internal/capability/event_sink.go:18` — `EventSink` only handles indexed documents, not routing metadata.
- `internal/capability/runner.go:195` — `runOnce()` fires event-sink callback after execution, for Doc events only.
- `internal/capability/runner.go:24, 67-71` — already has an `AfterInvokeHook` entry point that is the natural place to record a routing outcome.
- No historical routing lookup exists anywhere. Each `Pick()` starts from scratch.

The fix is purely additive: add a new storage path, wire it in, then thread history into the next `Pick()`.

## Step 2 — schema

Add a new table to `internal/store/schema.go`:

```go
// RoutingDecision records one router Pick + Route outcome. Used by the
// router to warm its next Pick with recent patterns for this workspace.
CREATE TABLE IF NOT EXISTS routing_decisions (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id  TEXT NOT NULL DEFAULT '',
  prompt        TEXT NOT NULL,
  picked_name   TEXT NOT NULL,
  success       INTEGER NOT NULL DEFAULT 0,  -- 0=unknown, 1=ok, 2=error, 3=user_corrected
  correction    TEXT NOT NULL DEFAULT '',    -- name of capability the user actually wanted, if any
  created_at    INTEGER NOT NULL              -- unix seconds
);
CREATE INDEX IF NOT EXISTS idx_routing_workspace_time
  ON routing_decisions(workspace_id, created_at DESC);
```

Mirror whatever pattern `internal/store/schema.go` already uses for other tables (check the `drafts` table for the style).

## Step 3 — store API

Add to `internal/store/` (new file `routing.go`):

```go
type RoutingOutcome int
const (
    OutcomeUnknown       RoutingOutcome = 0
    OutcomeOK            RoutingOutcome = 1
    OutcomeError         RoutingOutcome = 2
    OutcomeUserCorrected RoutingOutcome = 3
)

type RoutingDecision struct {
    ID           int64
    WorkspaceID  string
    Prompt       string
    PickedName   string
    Outcome      RoutingOutcome
    Correction   string
    CreatedAt    time.Time
}

func (s *Store) InsertRoutingDecision(ctx context.Context, d RoutingDecision) (int64, error)
func (s *Store) UpdateRoutingOutcome(ctx context.Context, id int64, outcome RoutingOutcome, correction string) error
func (s *Store) RecentRoutingDecisions(ctx context.Context, workspaceID string, limit int) ([]RoutingDecision, error)
```

Write table-driven tests in `internal/store/routing_test.go` covering: insert, update outcome, recent returns ordered desc, empty workspace id returns global recent.

## Step 4 — wire router

In `internal/capability/router.go`, add to the `Router` struct:

```go
// Store, when non-nil, causes Router to persist every Pick to the
// routing_decisions table and to warm future Picks with the most
// recent N decisions for the current workspace.
Store         RoutingStore
WorkspaceID   string
HistoryLookback int // default 5 if 0
```

Define a minimal interface in the same file so `internal/store` can satisfy it without capability importing store:

```go
type RoutingStore interface {
    InsertRoutingDecision(ctx context.Context, prompt, pickedName, workspaceID string) (int64, error)
    UpdateRoutingOutcome(ctx context.Context, id int64, ok bool, correction string) error
    RecentRoutingDecisions(ctx context.Context, workspaceID string, limit int) ([]RoutingDecisionSummary, error)
}

type RoutingDecisionSummary struct {
    Prompt     string
    PickedName string
}
```

Then in `internal/store/routing.go` add adapter methods that match the interface shape (the richer types live in store, the thin interface lives in capability).

### Pick() changes

In `router.go:73-96` Pick:
1. Before calling the LLM, if `r.Store != nil` and `r.HistoryLookback > 0`, load recent decisions.
2. If any exist, inject them into the system prompt as examples: `"Previously in this workspace: prompt='...' → capability='...'"`. Keep this compact — at most 5 lines.
3. After the LLM returns a pick, call `r.Store.InsertRoutingDecision(ctx, prompt, picked, r.WorkspaceID)` and stash the returned ID on the context or return it alongside the pick.

### Route() changes

In `router.go:113` Route:
1. Accept the decision ID from Pick (thread it through).
2. After the runner returns, call `r.Store.UpdateRoutingOutcome(ctx, id, err == nil, "")`.
3. On `ErrNoMatch`, record `ok=false` with empty correction.

## Step 5 — wire the store at Router construction

Find every site that constructs a `Router` (grep `capability.Router{` and `NewRouter` in `cmd/` and `internal/`). Each needs a `Store` and `WorkspaceID` passed in. For CLI sites, open the store via existing store init helper (check `cmd/serve.go` or `cmd/root.go` for the canonical pattern).

## Step 6 — tests

- `internal/capability/router_test.go` — add a test that uses a fake `RoutingStore` (in-memory slice) and verifies:
  - Pick calls InsertRoutingDecision
  - History is injected into subsequent Pick prompts
  - Route calls UpdateRoutingOutcome with success/failure
- `internal/store/routing_test.go` — table-driven insert/update/query tests

## Step 7 — verify

```bash
go build ./...
go vet ./...
go test ./internal/store/... ./internal/capability/... 2>&1 | tail -20
```

All must pass. If any existing test breaks because a router construction site wasn't updated, fix the site — do not skip the test.

## Step 8 — commit + report

```bash
git add -A
git status
git commit -m "feat(router): persist pick outcomes and warm Pick with recent history"
```

Report back: commit SHA, file count touched, test pass count, and any sites where you had to add a nil-store fallback (ideally zero — every construction site should have a store now).

## Hard rules

- Pre-1.0: no migration code. If the schema already existed under a different shape, drop and recreate.
- Do not introduce a new package for this; reuse `internal/store` and `internal/capability`.
- The `RoutingStore` interface lives in capability so capability does not import store — that's the whole point of the interface.
- Do not record full LLM responses, system prompts, or synthesized answers. Only the user prompt, the picked name, and the outcome. Keep the table narrow.
- Default `HistoryLookback` is 5. Do not make it configurable via YAML in this pass.
