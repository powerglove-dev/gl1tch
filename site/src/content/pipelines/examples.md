---
title: "Pipeline Examples"
description: "Copy-paste pipelines for real developer workflows — standup digests, PR review, security scans, and more."
order: 5
---

Real pipelines you can copy, adjust, and run today. Each one is a complete working example built around a task developers actually repeat. Change the `vars` to match your setup and run it.


## Quick Start

Save any example as `<name>.pipeline.yaml`, then run:

```bash
glitch pipeline run <name>.pipeline.yaml
```

To save it for repeated use:

```bash
cp <name>.pipeline.yaml ~/.config/glitch/pipelines/
glitch pipeline run <name>
```


## Morning Standup Digest

Reads your last 24 hours of commits and drafts a standup update. Run it before your daily sync.

```yaml
name: standup
version: "1"
vars:
  repo: "your-org/your-repo"

steps:
  - id: commits
    executor: gh
    vars:
      args: "api repos/{{vars.repo}}/commits?since=$(date -u -v-1d +%Y-%m-%dT%H:%M:%SZ) --jq '.[].commit.message'"

  - id: draft
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [commits]
    prompt: |
      Here are my git commits from the last 24 hours:

      {{steps.commits.output}}

      Write a brief daily standup with three sections:
      - Yesterday: what I completed
      - Today: what I'm working on next (infer from the trajectory)
      - Blockers: say "none" if nothing is obvious

      Keep it under 10 lines. No preamble.

  - id: save
    executor: write
    needs: [draft]
    vars:
      path: "./standup.md"
    input: "{{steps.draft.output}}"
```

**Customize:** Change `vars.repo` to your repository. Adjust the date window in the `gh api` call — swap `-1d` for `-7d` to get a week's worth.


## Review a Pull Request

Fetches the diff for a PR, lists the changed files, and gives you an AI review with specific line-level feedback.

```yaml
name: pr-review
version: "1"
vars:
  repo: "your-org/your-repo"
  pr: "42"

steps:
  - id: get-diff
    executor: gh
    vars:
      args: "pr diff {{vars.pr}} --repo {{vars.repo}}"

  - id: get-meta
    executor: gh
    vars:
      args: "pr view {{vars.pr}} --repo {{vars.repo}} --json title,body,author,additions,deletions,changedFiles"

  - id: review
    executor: claude
    model: claude-sonnet-4-6
    needs: [get-diff, get-meta]
    prompt: |
      Review this pull request.

      Metadata:
      {{steps.get-meta.output}}

      Diff:
      {{steps.get-diff.output}}

      Provide:
      1. A one-paragraph summary of what the PR does
      2. Any bugs or logic errors you can spot
      3. Style or naming issues worth fixing
      4. One thing that's done well
      5. A recommended verdict: Approve / Request Changes / Needs Discussion

      Be specific — reference file names and line ranges where relevant.

  - id: save
    executor: write
    needs: [review]
    vars:
      path: "./pr-{{vars.pr}}-review.md"
    input: "{{steps.review.output}}"
```

**Customize:** Set `vars.pr` to the PR number before each run. For a lighter-weight review, swap `claude-sonnet-4-6` for `claude-haiku-4-5-20251001`.


## Scan for Security Issues

Runs a static analysis pass on your codebase and produces a prioritized list of security findings.

```yaml
name: security-scan
version: "1"
vars:
  dir: "."

steps:
  - id: find-secrets
    executor: shell
    prompt: |
      grep -rn \
        -e "api_key\s*=" \
        -e "secret\s*=" \
        -e "password\s*=" \
        -e "PRIVATE KEY" \
        --include="*.go" \
        --include="*.yaml" \
        --include="*.env" \
        {{vars.dir}} 2>/dev/null | head -50 || echo "none found"

  - id: find-sql-injection
    executor: shell
    prompt: |
      grep -rn \
        -e 'fmt\.Sprintf.*SELECT\|INSERT\|UPDATE\|DELETE' \
        --include="*.go" \
        {{vars.dir}} 2>/dev/null | head -30 || echo "none found"

  - id: analyze
    executor: claude
    model: claude-sonnet-4-6
    needs: [find-secrets, find-sql-injection]
    prompt: |
      I ran two security checks on a codebase.

      Potential hardcoded secrets:
      {{steps.find-secrets.output}}

      Potential SQL injection patterns:
      {{steps.find-sql-injection.output}}

      For each finding:
      1. Confirm if it's an actual risk or a false positive
      2. Rate severity: Critical / High / Medium / Low
      3. Suggest the fix in one sentence

      End with a summary: total confirmed findings and one immediate action.

  - id: save
    executor: write
    needs: [analyze]
    vars:
      path: "./security-report.md"
    input: "{{steps.analyze.output}}"
```

**Customize:** Add more `grep` steps for other patterns. Set `vars.dir` to a specific subdirectory to narrow the scan.


## Triage Open Issues

Lists your open GitHub issues, groups them by priority, and posts the triage as a comment on a tracking issue.

```yaml
name: issue-triage
version: "1"
vars:
  repo: "your-org/your-repo"
  tracking_issue: "1"

steps:
  - id: list-issues
    executor: gh
    vars:
      args: "issue list --repo {{vars.repo}} --state open --json number,title,labels,createdAt,comments --limit 30"

  - id: triage
    executor: claude
    model: claude-sonnet-4-6
    needs: [list-issues]
    prompt: |
      Triage these open GitHub issues:

      {{steps.list-issues.output}}

      Group them into three buckets:
      - 🔴 High priority: bugs, regressions, blocking other work
      - 🟡 Medium priority: features in progress, active discussion
      - 🟢 Low priority: enhancements, nice-to-haves, stale threads

      Format as a markdown table per group: Issue# | Title | Age | Reason.
      End with "Recommended next 3 to close" with brief reasoning.

  - id: post
    executor: gh
    needs: [triage]
    vars:
      args: "issue comment {{vars.tracking_issue}} --repo {{vars.repo}} --body '{{steps.triage.output}}'"
```

**Customize:** Change `vars.tracking_issue` to your backlog or sprint tracking issue number. Swap the `post` step for a `write` executor if you'd rather save locally instead of posting to GitHub.


## Generate a Changelog Entry

Compares two git refs and writes a formatted changelog entry — useful before cutting a release.

```yaml
name: changelog
version: "1"
vars:
  from: "v1.2.0"
  to: "HEAD"
  repo: "your-org/your-repo"

steps:
  - id: commits
    executor: shell
    prompt: "git log {{vars.from}}..{{vars.to}} --oneline --no-merges"

  - id: prs
    executor: gh
    vars:
      args: "pr list --repo {{vars.repo}} --state merged --json number,title,author,mergedAt --limit 50"

  - id: draft
    executor: claude
    model: claude-sonnet-4-6
    needs: [commits, prs]
    prompt: |
      Draft a changelog entry for the release from {{vars.from}} to {{vars.to}}.

      Commits:
      {{steps.commits.output}}

      Merged PRs:
      {{steps.prs.output}}

      Use these sections (omit any that are empty):
      ### Features
      ### Bug Fixes
      ### Performance
      ### Breaking Changes

      Each entry: one sentence, past tense, no technical jargon.
      Lead with the user-visible impact, not the implementation detail.

  - id: save
    executor: write
    needs: [draft]
    vars:
      path: "./CHANGELOG-draft.md"
    input: "{{steps.draft.output}}"
```

**Customize:** Set `vars.from` to your last tag and `vars.to` to your release branch.


## Compare Two AI Models

Sends the same prompt to two models simultaneously and judges which answer is better. Good for evaluating a new local model before committing to it.

```yaml
name: model-compare
version: "1"
vars:
  prompt: |
    Explain the difference between a mutex and a semaphore.
    Give a practical Go example for each.

steps:
  # These two steps have no `needs`, so they run in parallel
  - id: cloud-answer
    executor: claude
    model: claude-haiku-4-5-20251001
    prompt: "{{vars.prompt}}"

  - id: local-answer
    executor: ollama
    model: llama3.2:latest
    prompt: "{{vars.prompt}}"

  - id: judge
    executor: claude
    model: claude-sonnet-4-6
    needs: [cloud-answer, local-answer]
    prompt: |
      I sent the same question to two AI models.

      Question: {{vars.prompt}}

      Answer A:
      {{steps.cloud-answer.output}}

      Answer B:
      {{steps.local-answer.output}}

      Rate each on: accuracy, clarity, code correctness, and completeness.
      Score each criterion 1-10. Declare a winner with a one-sentence justification.

  - id: save
    executor: write
    needs: [judge]
    vars:
      path: "./model-comparison.md"
    input: "{{steps.judge.output}}"
```

**Customize:** Change `vars.prompt` to any question or coding task. Swap in different models to compare Claude tiers, different Ollama models, or add a third parallel step for a three-way comparison.


## Explain an Unfamiliar Codebase

Reads the directory structure, samples key files, and gives you a plain-English walkthrough of an unfamiliar repo.

```yaml
name: explain-repo
version: "1"
vars:
  dir: "."

steps:
  - id: structure
    executor: shell
    prompt: "find {{vars.dir}} -type f -name '*.go' -o -name '*.py' -o -name '*.ts' | head -60"

  - id: readme
    executor: shell
    prompt: "cat {{vars.dir}}/README.md 2>/dev/null || echo 'no README found'"

  - id: sample-files
    executor: shell
    prompt: |
      for f in $(find {{vars.dir}} -maxdepth 3 -type f \( -name 'main.go' -o -name 'app.py' -o -name 'index.ts' -o -name 'server.go' \) | head -5); do
        echo "=== $f ==="; head -60 "$f"; echo
      done

  - id: explain
    executor: claude
    model: claude-sonnet-4-6
    needs: [structure, readme, sample-files]
    prompt: |
      Help me understand this codebase.

      File structure:
      {{steps.structure.output}}

      README:
      {{steps.readme.output}}

      Key source files:
      {{steps.sample-files.output}}

      Give me:
      1. What this project does (2-3 sentences)
      2. The main entry points and what they do
      3. The high-level data flow (how data moves through the system)
      4. The three most important files to read first and why
      5. Any patterns or conventions I should know before diving in

  - id: save
    executor: write
    needs: [explain]
    vars:
      path: "./repo-explainer.md"
    input: "{{steps.explain.output}}"
```

**Customize:** Set `vars.dir` to any local repo path. Adjust the `find` patterns to match the language(s) in the codebase.


## Patterns Worth Stealing

**Pipeline-level `vars` for config.** Put repo names, file paths, and thresholds in `vars` instead of hardcoding them in prompts. Easier to change and easier to read.

**Parallel fan-out, serial fan-in.** Steps without `needs` run at the same time. A step that lists multiple IDs in `needs` waits for the slowest one. Use this pattern for A/B testing, parallel fetches, or multi-source data collection.

**Fast model for extraction, smart model for reasoning.** Use a local Ollama model or `claude-haiku` for pulling data out of text. Reserve `claude-sonnet` for the final analysis or decision step.

**Always save results.** Every pipeline that produces useful output should end with an `executor: write` step. Terminal output scrolls away. Files persist and are re-readable by your next pipeline.

**`builtin.log` for progress checkpoints.** Add a log step between expensive operations so you can see where a long pipeline is in its run.


## See Also

- [Pipelines](/docs/pipelines/pipelines) — Full guide to writing pipelines
- [Your First Pipeline](/docs/pipelines/quickstart) — Start here if you're new
- [Console](/docs/pipelines/console) — Launch and monitor from your workspace
