---
title: "Real-World Pipelines"
description: "Four complete pipelines you can copy and run today."
order: 5
---

These are working pipelines. Copy the YAML, adjust the variables, and run them. Each one demonstrates a different pattern.

---

## 1. Daily standup generator

Reads your recent git commits from GitHub and drafts a standup update.

```yaml
name: standup
version: "1"
vars:
  repo: "8op-org/gl1tch"

steps:
  # Fetch commits from the last 24 hours
  - id: recent-commits
    executor: gh
    vars:
      args: "api repos/{{vars.repo}}/commits?since=$(date -u -v-1d +%Y-%m-%dT%H:%M:%SZ) --jq '.[].commit.message'"

  # Generate a standup summary
  - id: draft-standup
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [recent-commits]
    prompt: |
      Here are my git commits from the last 24 hours:

      {{steps.recent-commits.output}}

      Write a brief daily standup update with three sections:
      - What I did yesterday
      - What I'm doing today (infer from commit trajectory)
      - Any blockers (say "none" if nothing obvious)

      Keep it under 10 lines. No preamble.

  # Save to a file
  - id: save
    executor: write
    needs: [draft-standup]
    vars:
      path: "./standup.md"
    input: "{{steps.draft-standup.output}}"
```

**How to run:**

```bash
glitch pipeline run standup.pipeline.yaml
```

**Customize:** Change `vars.repo` to your repository. Adjust the date math in the `gh api` call if you want a different time window.

---

## 2. Codebase health check

Indexes your codebase using Ollama embeddings, then asks Claude about the architecture using brain context.

```yaml
name: health-check
version: "1"

steps:
  # Index the codebase for semantic search
  - id: index
    executor: builtin.index_code
    args:
      root: "."
      model: "qwen2.5-coder:latest"

  # Local model analyzes patterns and writes brain notes
  - id: analyze-structure
    executor: ollama
    model: qwen2.5-coder:latest
    needs: [index]
    prompt: |
      Based on the codebase index provided, analyze:
      1. How many packages/modules exist
      2. Which areas have the most complexity
      3. Any circular dependencies or code smells

      Record your key findings in a <brain tags="architecture"> block.

  # Claude synthesizes recommendations — brain notes from analyze-structure arrive automatically
  - id: recommendations
    executor: claude
    model: claude-sonnet-4-6
    needs: [analyze-structure]
    prompt: |
      You have brain context from a codebase analysis.

      Based on those findings, provide:
      1. A health score (1-10) with justification
      2. The top 3 areas that need refactoring
      3. Any architectural concerns

      Be specific. Reference file paths and package names.

  # Save the report
  - id: save-report
    executor: write
    needs: [recommendations]
    vars:
      path: "./health-report.md"
    input: "{{steps.recommendations.output}}"
```

**How to run:**

```bash
# Make sure Ollama is running with the model pulled
ollama pull qwen2.5-coder:latest
glitch pipeline run health-check.pipeline.yaml
```

**Customize:** Change `args.root` to target a specific directory. Swap the Ollama model for a larger one if you want deeper analysis.

---

## 3. GitHub PR triage

Lists open PRs, groups them by risk level, and posts a triage summary as an issue comment.

```yaml
name: pr-triage
version: "1"
vars:
  repo: "8op-org/gl1tch"
  triage_issue: "1"

steps:
  # Fetch open PRs with metadata
  - id: list-prs
    executor: gh
    vars:
      args: "pr list --repo {{vars.repo}} --json number,title,author,additions,deletions,changedFiles,labels --limit 20"

  # Classify by risk
  - id: classify
    executor: claude
    model: claude-sonnet-4-6
    needs: [list-prs]
    prompt: |
      Here are the open pull requests:

      {{steps.list-prs.output}}

      Triage each PR into one of three risk categories:
      - HIGH: large diffs (>500 lines), touches core/critical paths, or no tests
      - MEDIUM: moderate changes, single feature area
      - LOW: small fixes, docs, config changes

      Format your output as a markdown table with columns:
      PR# | Title | Author | Risk | Reason

      Then add a "Recommended Review Order" section listing PR numbers
      from highest to lowest priority.

  # Post the triage as a comment on the tracking issue
  - id: post-triage
    executor: gh
    needs: [classify]
    vars:
      args: "issue comment {{vars.triage_issue}} --repo {{vars.repo}} --body '{{steps.classify.output}}'"
```

**How to run:**

```bash
glitch pipeline run pr-triage.pipeline.yaml
```

**Customize:** Set `vars.repo` to your repository and `vars.triage_issue` to the issue number where you want triage reports posted. You could also swap the `post-triage` step to use `executor: write` if you'd rather save locally instead of posting to GitHub.

---

## 4. Parallel model comparison

Sends the same prompt to Claude and Ollama simultaneously, then compares their outputs. Demonstrates parallel execution with no dependencies.

```yaml
name: model-compare
version: "1"
vars:
  test_prompt: |
    Write a function in Go that reverses a linked list in place.
    Include the struct definition and a brief explanation.

steps:
  # These two steps have no `needs`, so they run in parallel
  - id: claude-response
    executor: claude
    model: claude-sonnet-4-6
    prompt: "{{vars.test_prompt}}"

  - id: ollama-response
    executor: ollama
    model: qwen2.5-coder:latest
    prompt: "{{vars.test_prompt}}"

  # Wait for both, then compare
  - id: compare
    executor: claude
    model: claude-sonnet-4-6
    needs: [claude-response, ollama-response]
    prompt: |
      I sent the same coding prompt to two different AI models.
      Compare their responses on these criteria:
      1. Correctness (does the code actually work?)
      2. Clarity (is the explanation helpful?)
      3. Idiomatic Go (does it follow Go conventions?)
      4. Edge cases (does it handle nil, single element, etc?)

      Give a score out of 10 for each criterion, then declare a winner.

      --- CLAUDE RESPONSE ---
      {{steps.claude-response.output}}

      --- OLLAMA RESPONSE ---
      {{steps.ollama-response.output}}

  # Log completion
  - id: done
    executor: builtin.log
    needs: [compare]
    args:
      message: "Model comparison complete."

  # Save the comparison
  - id: save
    executor: write
    needs: [compare]
    vars:
      path: ./model-comparison.md
    input: "{{steps.compare.output}}"
```

**How to run:**

```bash
# Both providers need to be available
ollama pull qwen2.5-coder:latest
glitch pipeline run model-compare.pipeline.yaml
```

**Customize:** Change `vars.test_prompt` to any coding or writing task. Swap models to compare different Claude tiers (`haiku` vs `sonnet`) or different Ollama models. Add more parallel steps to compare three or four models at once -- just add them to the `compare` step's `needs` list.

---

## Patterns to steal

A few things these examples demonstrate that you can apply to your own pipelines:

**Pipeline-level vars** for configuration that might change between runs. Put repo names, file paths, and thresholds in `vars` instead of hardcoding them in prompts.

**Brain blocks are opt-in by the model.** Steps that fetch raw data (gh, http_get) won't emit `<brain>` blocks — they return JSON, not prose. No configuration needed; the model simply won't write one if you don't prompt it to.

**Parallel fan-out, serial fan-in.** Steps without `needs` run in parallel. A downstream step that `needs` all of them waits for the slowest one. Use this for A/B testing, parallel data fetching, or redundant processing.

**builtin.log for checkpoints.** Drop `builtin.log` steps between expensive operations so you can see progress in the terminal output.

**Write results to files.** Every pipeline that produces useful output should end with an `executor: write` step. Terminal output scrolls away. Files persist.
