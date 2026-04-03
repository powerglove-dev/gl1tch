---
title: "Pipelines"
description: "Write YAML workflows that chain shell commands, AI models, and your own tools to automate any developer task."
order: 2
---

A pipeline is a YAML file that tells gl1tch what to run, in what order, and how to pass results between steps. Write one pipeline for your standup. Another for PR review. Another for your deploy. gl1tch runs them on demand, chains their outputs together, and keeps a history of every run.


## Quick Start

Here's a complete, working pipeline. Copy it, save it as `summarize.pipeline.yaml`, and run it:

```yaml
name: summarize
version: "1"
steps:
  - id: get-logs
    executor: shell
    vars:
      cmd: "git log --oneline -10"

  - id: summarize
    executor: ollama
    model: llama3.2:latest
    needs: [get-logs]
    prompt: |
      Summarize these recent git commits in plain English.
      Focus on what changed, not how:

      {{get "step.get-logs.data.value" .}}
```

```bash
glitch pipeline run summarize.pipeline.yaml
```

Your last 10 commits feed straight into the model. The summary streams to your terminal.


## Building a Pipeline with gl1tch

You don't have to write YAML by hand. Ask gl1tch to build it.

Open your workspace (`glitch`):

![gl1tch workspace ready](/screenshots/pipelines/pipeline-build-01-ready.png)

Describe what you want:

```text
you:     build me a pipeline that summarizes my recent git activity
         and flags anything that looks risky

gl1tch:  got it. pulling commits and diffstat, then routing through
         a local model for the summary and risk pass.

         here's the pipeline — i'll save it to
         ~/.config/glitch/pipelines/git-digest.pipeline.yaml

         ┌─────────────────────────────────────────────────────┐
         │ name: git-digest                                     │
         │ version: "1"                                         │
         │                                                      │
         │ steps:                                               │
         │   - id: log                                          │
         │     executor: shell                                  │
         │     prompt: "git log --oneline -20"                  │
         │                                                      │
         │   - id: diffstat                                     │
         │     executor: shell                                  │
         │     prompt: "git diff HEAD~10 --stat"                │
         │                                                      │
         │   - id: summarize                                    │
         │     executor: ollama                                 │
         │     model: qwen2.5-coder:latest                      │
         │     needs: [log, diffstat]                           │
         │     prompt: |                                        │
         │       Summarize recent git activity.                 │
         │       Flag any changes that look risky (large        │
         │       deletions, core files, no tests nearby).       │
         │                                                      │
         │       commits:                                       │
         │       {{steps.log.output}}                           │
         │                                                      │
         │       diffstat:                                      │
         │       {{steps.diffstat.output}}                      │
         └─────────────────────────────────────────────────────┘

         run it?

you:     yes

gl1tch:  running git-digest…

         ✓  log       0.1s
         ✓  diffstat  0.1s
         ✓  summarize 3.2s

         ─────────────────────────────────────────────────────
         Recent activity spans 20 commits over the last 3 days.
         Main changes: pipeline runner refactor (runner.go +340
         / -180), new executor discovery logic, test coverage
         added for retry behavior.

         ⚠ Flagged: internal/pipeline/runner.go saw significant
         rewrites with no accompanying test file changes in the
         same window. Worth a focused review pass before merge.
         ─────────────────────────────────────────────────────
```

The pipeline is saved. Run it any time:

```bash
glitch pipeline run git-digest
```

Or run it from the console:

```text
/pipeline git-digest
```

![gl1tch launching git-digest](/screenshots/pipelines/pipeline-run-03-result.png)

---

Here's the full pipeline that was generated and tested:

```yaml
name: git-digest
version: "1"

steps:
  - id: log
    executor: shell
    vars:
      cmd: "git log --oneline -20"

  - id: diffstat
    executor: shell
    vars:
      cmd: "git diff HEAD~10 --stat"

  - id: summarize
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [log, diffstat]
    prompt: |
      Summarize recent git activity.
      Flag any changes that look risky (large deletions, core files, no tests nearby).

      commits:
      {{get "step.log.data.value" .}}

      diffstat:
      {{get "step.diffstat.data.value" .}}
```

`log` and `diffstat` run in parallel. `summarize` waits for both, then feeds both outputs to the model in a single pass.


## How Pipelines Work

### Steps

Every pipeline is a list of steps. Each step has:

- an `id` — a unique name you use to reference its output
- an `executor` — what runs the step (`shell`, `claude`, `ollama`, `gh`, `write`, etc.)
- a `prompt` or `args` — what to pass to the executor

```yaml
steps:
  - id: fetch
    executor: shell
    vars:
      cmd: "curl -s https://api.example.com/status"
```

### Dependencies with `needs`

By default, steps with no `needs` run immediately (and in parallel if there are multiple). Add `needs` to make a step wait for another:

```yaml
steps:
  - id: fetch           # runs first
    executor: shell
    vars:
      cmd: "curl -s https://api.example.com/status"

  - id: analyze         # waits for fetch
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [fetch]
    prompt: |
      What does this API status response mean?
      {{get "step.fetch.data.value" .}}
```

Add multiple IDs to `needs` to wait for several steps:

```yaml
needs: [fetch-prod, fetch-staging]
```

gl1tch runs `fetch-prod` and `fetch-staging` in parallel, then starts `analyze` as soon as both finish.

### Passing Output Between Steps

Use `{{get "step.<id>.data.value" .}}` in any prompt field to inject a previous step's output:

```yaml
prompt: |
  Here is the data: {{get "step.fetch.data.value" .}}
  Summarize it.
```

Template expressions work in `prompt`, `input`, `args`, and `vars` fields.

Other template forms:

| Expression | What it gives you |
|------------|-------------------|
| `{{get "step.fetch.data.value" .}}` | Full output of the `fetch` step |
| `{{vars.repo}}` | A pipeline-level variable |
| `{{env "HOME"}}` | An environment variable |


## Writing Your First Real Pipeline

A three-step pipeline that checks GitHub for open issues and writes a summary report:

```yaml
name: issue-digest
version: "1"
vars:
  repo: "your-org/your-repo"

steps:
  - id: list-issues
    executor: gh
    vars:
      args: "issue list --repo {{vars.repo}} --state open --json number,title,labels,createdAt --limit 25"

  - id: digest
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [list-issues]
    prompt: |
      Here are the open issues for {{vars.repo}}:

      {{get "step.list-issues.data.value" .}}

      Group them by label. For each group, list the issue numbers and titles.
      End with a one-sentence summary of the overall backlog health.

  - id: save
    executor: write
    needs: [digest]
    vars:
      path: "./issue-digest.md"
    input: "{{get \"step.digest.data.value\" .}}"
```

Run it:

```bash
glitch pipeline run issue-digest.pipeline.yaml
```

The report saves to `./issue-digest.md`.


## Executors

The executor tells gl1tch what kind of work a step does.

| Executor | What it does |
|----------|-------------|
| `shell` | Runs a shell command; `vars.cmd` is the command string |
| `claude` | Sends `prompt` to a Claude model |
| `ollama` | Sends `prompt` to a local Ollama model |
| `gh` | Runs a `gh` CLI command; `vars.args` is the argument string |
| `write` | Writes `input` to `vars.path` on disk |
| `builtin.log` | Logs a message to the terminal (useful for checkpoints) |
| `builtin.assert` | Fails the pipeline if a condition is false |

For `claude` and `ollama` steps, set `model` to the model name. For shell-based executors, use `vars.args` or `prompt` depending on the executor.

> **TIP:** The `gh` executor requires the [GitHub CLI](https://cli.github.com/) installed and authenticated.


## Control Flow

### Retry on Failure

```yaml
- id: flaky-api
  executor: shell
  vars:
    cmd: "curl -f https://api.example.com/data"
  retry:
    max_attempts: 3
    interval: "5s"
```

### Run a Step on Failure

```yaml
- id: deploy
  executor: shell
  vars:
    cmd: "./deploy.sh"
  on_failure: notify-slack

- id: notify-slack
  executor: shell
  vars:
    cmd: "curl -X POST $SLACK_WEBHOOK -d '{\"text\":\"deploy failed\"}'"
```

### Conditional Execution

```yaml
- id: check
  executor: shell
  vars:
    cmd: "test -f ./lockfile && echo 'locked' || echo 'free'"
  condition: "contains:free"

- id: proceed
  executor: claude
  needs: [check]
  # only runs if check output contains "free"
```

Condition values: `always`, `not_empty`, `contains:<string>`, `matches:<regex>`, `len > <n>`.

### Repeat a Step for Each Item

```yaml
- id: summarize-each
  executor: claude
  model: claude-haiku-4-5-20251001
  for_each: "{{get \"step.list-files.data.value\" .}}"
  prompt: |
    Summarize this file: {{item}}
```

`for_each` clones the step once per line of the input and collects all outputs.


## Pipeline-Level Settings

```yaml
name: my-pipeline
version: "1"
description: "What this pipeline does"
vars:
  repo: "your-org/your-repo"
  threshold: "50"
max_parallel: 4       # max steps running at once (default: 8)
steps:
  - ...
```

| Field | Default | What it does |
|-------|---------|-------------|
| `name` | — | Pipeline identifier; used when running by name |
| `version` | — | Schema version (use `"1"` for now) |
| `description` | — | One-line summary shown in the launcher |
| `vars` | — | Pipeline-level variables, accessed as `{{vars.key}}` |
| `max_parallel` | `8` | Maximum steps running concurrently |


## Customizing

### Save Pipelines for Quick Access

Drop your pipeline into `~/.config/glitch/pipelines/`:

```bash
cp issue-digest.pipeline.yaml ~/.config/glitch/pipelines/
```

Then run it by name from anywhere:

```bash
glitch pipeline run issue-digest
```

It also appears in the Pipeline Launcher in your workspace.

### Use Variables for Flexibility

Put anything that changes between runs in `vars`:

```yaml
vars:
  env: "staging"
  notify: "true"
```

Access them as `{{vars.env}}` in any step. Override them at runtime by passing environment variables:

```bash
GLITCH_VAR_ENV=production glitch pipeline run deploy
```

### Mix Local and Cloud Models

Use a fast local model for data-heavy steps, a smarter cloud model for the reasoning step:

```yaml
steps:
  - id: extract        # fast local extraction
    executor: ollama
    model: llama3.2:latest
    prompt: "Extract all TODO comments: {{get \"step.read-code.data.value\" .}}"

  - id: prioritize     # cloud model for nuanced analysis
    executor: claude
    model: claude-sonnet-4-6
    needs: [extract]
    prompt: "Prioritize these TODOs by risk: {{get \"step.extract.data.value\" .}}"
```


## Examples

### Morning standup

Reads your last 24 hours of commits and writes a standup draft.

```yaml
name: standup
version: "1"

steps:
  - id: commits
    executor: shell
    vars:
      cmd: "git log --since='24 hours ago' --oneline --no-merges"

  - id: draft
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [commits]
    prompt: |
      Write a standup update from these commits.
      Format: Yesterday / Today / Blockers. Keep it under 8 lines.

      {{get "step.commits.data.value" .}}
```

```bash
glitch pipeline run standup
```

Actual output:

```text
[step:commits] status:done
[step:draft]   status:done

**Yesterday:** Fixed release pipeline issues — moved install.sh to correct
tar path, fixed goreleaser formula config, corrected HOMEBREW_TAP_GITHUB_TOKEN
secret name.

**Today:** Shipped `glitch pipeline results` command (new feature) and
documented it with architecture guidance and view navigation fixes.

**Blockers:** None.
```

> **NOTE:** Examples here use Claude Haiku for speed. Swap in a larger local model
> like `qwen2.5-coder:latest` or `llama3.1:8b` via the `ollama` executor if you
> prefer to keep everything local — they produce good results but take a few
> seconds longer to load.


### Code review on a diff

Fetches your current diff against main and routes it through a model for a focused review pass.

```yaml
name: diff-review
version: "1"

steps:
  - id: diff
    executor: shell
    vars:
      cmd: "git diff main --stat && git diff main"

  - id: review
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [diff]
    prompt: |
      Review this diff. Focus on:
      - correctness issues
      - missing error handling
      - anything that would fail in production

      {{get "step.diff.data.value" .}}
```

Actual output:

```text
[step:diff]   status:done
[step:review] status:done

**Diff Review: ✅ Safe to merge**

The change itself is correct and follows the existing pattern:
- Adds "busd" to the recognized subcommand list alongside other commands
- Matches the alphabetical ordering convention
- Delegates to cmd.Execute() like all other subcommands

**Verification:**
- ✅ Command properly defined with init() registering to rootCmd
- ✅ Subcommand structure in place (busd publish)
- ✅ Error handling present in RunE function
- ✅ Proper argument validation with cobra.RangeArgs(1, 2)

**Potential concern (not in diff, but noted):**
Line 37 assigns json.RawMessage(args[1]) as the payload. Verify
busd.PublishEvent() accepts raw bytes — if it expects a parsed
object this will fail at runtime.
```


### Parallel model comparison

Same question to two models, then a judgment pass. `claude-answer` and `local-answer` run in parallel — `judge` waits for both:

```yaml
name: compare
version: "1"
vars:
  question: "Explain Go interfaces in two sentences."

steps:
  - id: claude-answer
    executor: claude
    model: claude-haiku-4-5-20251001
    prompt: "{{vars.question}}"

  - id: local-answer
    executor: ollama
    model: qwen2.5-coder:latest
    prompt: "{{vars.question}}"

  - id: judge
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [claude-answer, local-answer]
    prompt: |
      Compare these two answers to: "{{vars.question}}"

      Answer A: {{get "step.claude-answer.data.value" .}}
      Answer B: {{get "step.local-answer.data.value" .}}

      Which is clearer and more accurate? One paragraph.
```


## Reference

### Step Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Unique step identifier |
| `executor` | Yes | What runs the step |
| `model` | For LLM steps | Model name |
| `prompt` | Most executors | Command or LLM prompt |
| `args` | Some executors | Structured key/value arguments |
| `vars` | Some executors | Flat string arguments |
| `needs` | No | List of step IDs to wait for |
| `condition` | No | Branch condition after execution |
| `retry` | No | Retry policy object |
| `on_failure` | No | Step ID to run on failure |
| `for_each` | No | Repeat step for each line of input |
| `input` | `write` steps | Content to write |
| `no_brain` | No | Suppress brain context injection |

### Retry Policy Fields

| Field | Default | Description |
|-------|---------|-------------|
| `max_attempts` | `1` | Total attempts including first |
| `interval` | `"0s"` | Wait between attempts |
| `backoff` | `false` | Exponential backoff (reserved) |


## See Also

- [Your First Pipeline](/docs/pipelines/quickstart) — Five-minute intro
- [Examples](/docs/pipelines/examples) — Copy-paste pipelines for real workflows
- [Console](/docs/pipelines/console) — Launch and monitor from your workspace
