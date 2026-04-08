# internal/research

The research loop is gl1tch's bounded iterative researcher. It replaces the
single-shot `prompt → answer` path with a structured `plan → gather → draft
→ critique → score` loop that refuses to invent identifiers and reports its
own confidence.

It exists because of one observed failure mode (see
`memory/project_research_loop_negative_example.md`):

> User: "there have been recent updates to the pr's, can you verify their
> statuses?"
>
> Old assistant: invented five PR numbers from training data, dated them
> 2023, and presented them as fact. No tool was called.

The loop's job is to make that response impossible.

## The five stages

```
                  ┌──────────┐
   question  ───▶ │   plan   │  qwen2.5:7b picks researcher names from registry
                  └────┬─────┘
                       ▼
                  ┌──────────┐
                  │  gather  │  parallel researcher dispatch, partial bundle on errors
                  └────┬─────┘
                       ▼
                  ┌──────────┐
                  │  draft   │  qwen2.5:7b writes an answer grounded ONLY in the bundle
                  └────┬─────┘
                       ▼
                  ┌──────────┐
                  │ critique │  qwen2.5:7b labels each claim grounded/partial/ungrounded
                  └────┬─────┘
                       ▼
                  ┌──────────┐
                  │  score   │  composite of cross_cap + evidence_coverage + judge + self_consistency
                  └────┬─────┘
                       ▼
                  ┌─────────────────────┐
                  │ accept or refine?   │  cleared threshold? → accept. else: feed ungrounded claims
                  └────┬────────────────┘  back into plan, gather more, redraft.
                       │
              budget exhausted? ────▶ best-effort or escalate to paid verifier
```

Hard rules baked in:

- The planner is forbidden from inventing researcher names. Names not in
  the registry are dropped before dispatch.
- The drafter is forbidden from citing identifiers (PR numbers, SHAs,
  paths, dates) that do not appear verbatim in the bundle.
- The drafter is forbidden from saying "you should run X" — the model is
  the agent, not a docs surface.
- qwen2.5:7b is the local default for every stage.
- Escalation to a paid verifier is OFF by default (`MaxPaidTokens=0`).

## How to write a researcher

A researcher is anything that satisfies the `Researcher` interface:

```go
type Researcher interface {
    Name() string
    Describe() string
    Gather(ctx context.Context, q ResearchQuery, prior EvidenceBundle) (Evidence, error)
}
```

There are three ways to author one:

1. **CapabilityResearcher** wraps any `internal/capability.Capability` so a
   capability that already exists for indexing or assistant pick is
   available to the loop without duplication.

2. **PipelineResearcher** wraps a `.glitch/workflows/*.workflow.yaml` whose
   final step prints an `Evidence` JSON object. This is the user-extensible
   path: anyone who can write a workflow can add a researcher with no Go
   code. The canonical examples live in `.glitch/workflows/`:
   `github-prs`, `github-issues`, `git-log`, `git-status`.

3. **Implement the interface directly** in Go for stateful researchers.

The Evidence wire schema is documented in `evidence_schema.go` and reproduced
inline in `EvidenceSchemaDoc` so a pipeline that emits Evidence JSON can read
the spec without leaving its own file.

## Default registry

`research.DefaultRegistry(mgr, workflowsDir)` returns a Registry pre-loaded
with the canonical pipeline researchers. Researchers whose backing workflow
file is absent are silently skipped — a workspace that hasn't adopted
`github-prs` still gets a useful (smaller) menu.

To add a new default researcher:

1. Drop a `<name>.workflow.yaml` in `.glitch/workflows/` whose final step
   prints an Evidence JSON (use `github-prs.workflow.yaml` as the template).
2. Add a `DefaultPipelineSpec` entry to `DefaultPipelineResearchers` in
   `defaults.go` with a planner-friendly `Describe`.

## Confidence scoring

The four signals (cheap-first, short-circuit-aware):

| Signal | Cost | Computed by |
| --- | --- | --- |
| `cross_capability_agreement` | free | `CrossCapabilityAgree(bundle)` — structural source count |
| `evidence_coverage` | 1 LLM call | `CritiquePrompt` + `EvidenceCoverage(critique)` |
| `judge_score` | 1 LLM call | `JudgePrompt` + `ParseJudgeScore` |
| `self_consistency` | N+1 LLM calls | `SelfConsistencyPrompt` + N redrafts |

`Composite(score)` is an equal-weight average that *skips* nil pointers, so
a missing signal does not pull the composite down. The accept threshold
defaults to `0.7`.

## Brain events

Every iteration emits one `research_attempt` event (with the bundle) and one
`research_score` event (without). Every escalation emits one
`research_escalation` event with the paid model name, paid token count, and
verdict (`confirm` / `rewrite` / `error`).

The default file sink lives at `~/.glitch/research_events.jsonl` with a
7-day TTL on bundle bodies. Tests use the in-memory sink:

```go
sink := research.NewMemoryEventSink()
loop := research.NewLoop(reg, llm).WithEventSink(sink)
```

## Escalation

Escalation is the paid-verifier escape hatch for the rare cases where local
iterations don't clear the threshold. The hard defaults make it safe:

- `MaxPaidTokens=0` is the kill switch. With it set, the loop NEVER calls
  the verifier — even if a Verifier is wired and even if the local score
  is rock-bottom.
- The Verifier is asked to verify-or-correct the local draft against the
  same evidence bundle. It is never asked the original question with no
  context — that would burn paid tokens to recreate work qwen2.5:7b
  already did.
- The verifier's response is `CONFIRM` (keep local draft) or a rewritten
  draft grounded in the same bundle.

To enable from the CLI:

```
glitch ask "..." --escalate=claude --max-paid-tokens=20000
```

## Smoke test

`glitch research smoke -v` runs the loop end-to-end against the canonical
default registry. With Ollama running and `gh auth status` green, it
should produce a grounded summary of the actual open PRs in the current
repository — no hallucinated PR numbers, no "you should run gh pr list"
deflections.
