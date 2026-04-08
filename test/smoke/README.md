# smoke tests

End-to-end coverage for `glitch ask` against a real local clone of this repo
and live local services (Ollama + Elasticsearch). Guarded behind the `smoke`
build tag so they never run in the default `go test ./...` path.

## Running

```
go test -tags smoke -count=1 ./test/smoke/...
```

Requirements:
- Ollama running at `http://localhost:11434` with `qwen2.5:7b` and
  `nomic-embed-text` pulled.
- Elasticsearch running at `http://localhost:9200` with the `glitch-vectors`
  index writable.
- A local clone of this repo at `~/Projects/gl1tch`.

Missing dependencies skip rather than fail so the suite is safe to wire into
CI as an informational stage.

## What runs

`TestSmokeAsk_Gl1tch` — one foundation scenario against the gl1tch repo:

1. Enables `brainrag.EnableQueryProbe` to count vector reads.
2. Runs `builtin.index_code` against `~/Projects/gl1tch` (`.md,.go`).
3. Runs a two-step pipeline: `builtin.search_code` → `ollama` (qwen2.5:7b) that
   answers the scenario prompt using only the search results.
4. Asserts probe hits > 0 (proves the code index was actually consulted) and a
   non-empty final answer.

A markdown report lands at `test/smoke/out/foundation-report.md` after the
suite runs (gitignored).

`TestSmokeAttention_*` — coverage for the AI-first attention + deep-analysis
ladder (see `pkg/glitchd/attention.go`, `pkg/glitchd/deep_analysis.go`). The
programmatic counterpart to the `glitch attention` CLI.

1. `LoadResearchPromptBundled` — asserts the loader's fallback chain finds the
   bundled default `research_default.md` on a clean workspace.
2. `ClassifyReviewOnMyPR` — hands a synthetic reviewer-comment event to
   `ClassifyAttention` against live qwen2.5:7b and rejects a `low` verdict.
   Accepts `high` or `normal` because small models are not deterministic
   oracles; only a clearly-wrong answer fails the test.
3. `AnalyzeArtifactMode` — forces `Attention=high` and calls `AnalyzeOne`
   against live `opencode` + the coder model. Asserts the resulting markdown
   is non-empty and contains at least one of the section headers the artifact
   template asks for. Skips when `opencode` is not on PATH so dev boxes
   without the tool-using agent still run the classifier cases.

Next iteration expands to many scenarios per repo and adds the copilot/haiku
review step for end-to-end "one working fix" validation.
