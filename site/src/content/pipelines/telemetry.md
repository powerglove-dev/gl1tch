---
title: "Telemetry"
description: "Understand what gl1tch tracks, where it stays, and how to use it."
order: 99
---

gl1tch records timing and status data for every pipeline run. All of it stays on your machine. None of it is sent anywhere by default.

## What Gets Tracked

For every pipeline run, gl1tch records:

- Which pipeline ran and when
- Each step's name, status (success or failure), and duration
- Token counts and cost estimates for AI steps

This data has two uses: the activity feed in your workspace shows it in real time, and it's written to disk so you can analyze it later.

## Where It Lives

```text
~/.local/share/glitch/traces.jsonl   # Per-step timing and status
~/.local/share/glitch/metrics.jsonl  # Aggregate counters and histograms
```

Both files are newline-delimited JSON. Read them with `jq`:

```bash
# See what just ran
jq '.name, .duration_ns, .status.code' ~/.local/share/glitch/traces.jsonl | tail -20

# Find slow steps (over 5 seconds)
jq 'select((.endTime - .startTime) > 5000000000) | {name, duration_ns: (.endTime - .startTime)}' \
  ~/.local/share/glitch/traces.jsonl
```

## Sending to an External Tool

By default, traces go only to the local file. To forward them to Jaeger, Tempo, Datadog, or any compatible collector, set the endpoint:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317 glitch
```

When this variable is set, traces go to the collector instead of the local file. The collector must be reachable; if it isn't, the pipeline still runs and the error is logged to stderr.

## Privacy

gl1tch does not send telemetry to Anthropic, to any gl1tch service, or to any third party. The data stays on your disk unless you configure an external endpoint yourself.

## See Also

- [Architecture](/docs/pipelines/architecture) — how the activity feed and run history work
- [CLI Reference](/docs/pipelines/cli-reference) — `glitch pipeline run` and related commands
