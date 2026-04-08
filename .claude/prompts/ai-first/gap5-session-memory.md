# Gap #5 — Session memory for the assistant

You are working in `/Users/stokes/Projects/gl1tch`. Your job is to add persistent Q&A memory for `glitch assistant` so sessions don't start cold. Every assistant turn gets saved to SQLite, and every new prompt warms the router + LLM with recent and semantically-similar past turns.

## Step 1 — worktree

```bash
cd /Users/stokes/Projects/gl1tch
git check-ignore -q .worktrees && echo "ignored ok" || { echo "ABORT: .worktrees not ignored"; exit 1; }
git worktree add .worktrees/gap5-session-memory -b feature/session-memory
cd .worktrees/gap5-session-memory
go mod download
go test ./internal/store/... ./internal/brainrag/... ./internal/assistant/... ./cmd/... 2>&1 | tail -10
```

If baseline fails, stop and report.

## Background

Research from 2026-04-08:

- `internal/chatui/sessions_registry.go:88-182` — reads Claude Code's `~/.claude/projects/**/*.jsonl` for display only. Not gl1tch's own memory.
- `internal/chatui/sessions_registry.go:186-200` — reads Gemini's `~/.stok/sessions/history/**`, also display only.
- `internal/store/schema.go:83-94` — `drafts` table has `turns_json` for draft refinement history. This proves the turn-array pattern works; we just need another table for the assistant.
- `internal/brainrag/store.go:1-80` — `RAGStore` uses ES `dense_vector` + kNN for notes/code. Good template for embedding past turns if we want semantic recall.
- `cmd/assistant.go:54-102` — the assistant command. Picks a capability via router, invokes it, exits. No turn persistence.
- `internal/assistant/backend.go` — tiny discovery helpers only. No session logic.

Conclusion: add a new `assistant_turns` table to SQLite and a small store API. Recency window first; add semantic recall via RAGStore if it fits without blowing scope.

## Step 2 — schema

In `internal/store/schema.go`, add:

```sql
CREATE TABLE IF NOT EXISTS assistant_turns (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id      TEXT NOT NULL DEFAULT '',
  session_id        TEXT NOT NULL,
  role              TEXT NOT NULL,  -- 'user' | 'assistant'
  content           TEXT NOT NULL,
  capability_picked TEXT NOT NULL DEFAULT '',
  created_at        INTEGER NOT NULL  -- unix seconds
);
CREATE INDEX IF NOT EXISTS idx_assistant_turns_session
  ON assistant_turns(session_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_assistant_turns_workspace_time
  ON assistant_turns(workspace_id, created_at DESC);
```

Follow the conventions already in `schema.go` — if other tables use `TEXT` timestamps or a different PK style, match them.

## Step 3 — store API

New file `internal/store/assistant_turns.go`:

```go
type Role string
const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
)

type Turn struct {
    ID               int64
    WorkspaceID      string
    SessionID        string
    Role             Role
    Content          string
    CapabilityPicked string
    CreatedAt        time.Time
}

// InsertTurn appends a turn to the session. Returns the new row ID.
func (s *Store) InsertTurn(ctx context.Context, t Turn) (int64, error)

// RecentTurns returns the last N turns for a session, ordered by
// created_at ASC (oldest first, so the slice reads like a transcript).
func (s *Store) RecentTurns(ctx context.Context, sessionID string, limit int) ([]Turn, error)

// RecentWorkspaceTurns returns the last N turns across all sessions
// for a given workspace, used when starting a fresh session in a
// workspace that already has history.
func (s *Store) RecentWorkspaceTurns(ctx context.Context, workspaceID string, limit int) ([]Turn, error)
```

Table-driven tests in `internal/store/assistant_turns_test.go` covering: insert, ordering, session isolation, workspace cross-session recall, empty workspace returns empty.

## Step 4 — session ID derivation

Pick a session ID scheme and document it in a comment at the top of `cmd/assistant.go`:

- On each `glitch assistant` invocation with no explicit `--session`, derive a session id from `(workspace_id, date)` — all same-day, same-workspace invocations share a session.
- Add a `--session <id>` flag for explicit session control. Empty workspace uses just the date.
- A `--new-session` flag forces a fresh uuid.

Put the derivation in a helper `internal/assistant/session.go`:

```go
func DeriveSessionID(workspaceID string, now time.Time) string
func NewSessionID() string  // uses google/uuid, already a dep
```

Test both in `internal/assistant/session_test.go`.

## Step 5 — wire cmd/assistant.go

In `cmd/assistant.go`:

1. Open the store via whatever helper `cmd/serve.go` or `cmd/root.go` uses (reuse, do not duplicate).
2. Resolve the session id from flags.
3. Before calling `router.Pick`, load `RecentTurns(ctx, sessionID, 5)`. If empty and a workspace id is set, load `RecentWorkspaceTurns(ctx, workspaceID, 3)`.
4. Render the recent turns into a compact transcript string and prepend it to the user prompt handed to the router. Keep it small — a 300-token cap is fine. Truncate oldest first.
5. After the router returns a pick, `InsertTurn` for the user turn (role=user, capability_picked=pick).
6. After the capability executes, `InsertTurn` for the assistant turn (role=assistant, content=the synthesized answer, capability_picked="").
7. On error, still insert the user turn and an assistant turn with content="(error: …)".

## Step 6 — integration with gap 2 (optional, only if gap 2 is already merged)

If `feature/router-learning` is already merged to main, the assistant's `--session` flag should also thread the workspace id into `Router.WorkspaceID` so both tables line up on the same key. If gap 2 is not merged, skip this step — just record turns.

## Step 7 — do NOT add semantic recall in this pass

Semantic recall via `brainrag.RAGStore` is tempting but out of scope. Recency-only is enough to prove the pattern and delivers real value immediately. Leave a TODO comment at the top of `internal/assistant/session.go`:

```go
// TODO: semantic recall. Recency is the first pass. A follow-up
// should embed turn content via brainrag.NewEmbedder and do kNN
// search on prior turns keyed by workspace_id. Do not add this
// without a separate worktree.
```

## Step 8 — tests

- `internal/store/assistant_turns_test.go` — table-driven store tests
- `internal/assistant/session_test.go` — session id derivation
- `cmd/assistant_test.go` — if a test harness exists, add a case that runs two invocations in the same workspace+day and asserts the second sees the first's content in its prompt. If no harness, skip — do not build one from scratch.

## Step 9 — verify

```bash
go build ./...
go vet ./...
go test ./internal/store/... ./internal/assistant/... ./cmd/... 2>&1 | tail -30

# Smoke test
./bin/glitch assistant "what's the weather"
./bin/glitch assistant "and the day after"   # should show prompt includes prior turn
```

## Step 10 — commit + report

```bash
git add -A
git status
git commit -m "feat(assistant): persist Q&A turns and warm prompts with recent history"
```

Report: commit SHA, whether RecentTurns is rendered into the router prompt or only the LLM synthesize prompt (pick one, document why), and observed token size of the transcript on the smoke test.

## Hard rules

- Pre-1.0: no migration. Drop and recreate if the schema changes.
- Recency only. No embeddings, no RAGStore integration, no fuzzy search. Just `created_at DESC LIMIT N`.
- Do not read from `~/.claude/projects/**` for memory — that is Claude Code's data, not gl1tch's. The chatui display code can keep reading it, but the memory layer must be self-contained.
- Do not log turn content at info level. Turns may contain secrets. Debug-level or nothing.
- Session transcripts in the prompt are bounded by a token cap, not a turn count. Truncate oldest first.
- Do not introduce a new package. Everything lands in `internal/store` and `internal/assistant`.
