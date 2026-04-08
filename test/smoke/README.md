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

Next iteration expands to many scenarios per repo and adds the copilot/haiku
review step for end-to-end "one working fix" validation.
