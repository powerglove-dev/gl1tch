---
title: "Pipelines"
description: "Build multi-step workflows that sequence AI agents, tools, and decisions."
order: 2
---

Pipelines are YAML-defined workflows where each step chains together an executor (local model, cloud API, plugin, or builtin utility) with a prompt, optional context, and output handling. Pipeline execution forms a directed acyclic graph (DAG) so steps can run in parallel when dependencies allow, and the runner maintains structured context across step boundaries.

Pipelines are the foundation of gl1tch automation. You define them once in YAML, version them in git, run them from the CLI or the UI, schedule them as cron jobs, and analyze results in the activity feed. Brain context is injected automatically, so agents have access to your codebase history and previous conclusions.


## Pipeline structure

A pipeline YAML file declares a `name`, `version`, and ordered list of `steps`. Each step specifies an `executor` (the AI provider or tool that runs it) and a `prompt` (the instruction the executor receives).

```yaml
name: code-review
version: "1"

steps:
  - id: analyze
    executor: ollama/codellama
    prompt: |
      Review this code for bugs and security issues.
      
      {{input.code}}

  - id: polish
    executor: claude/claude-sonnet-4-6
    model: claude-sonnet-4-6
    prompt: |
      Improve the feedback from the previous step.
      Remove jargon, make it actionable.
      
      {{step.analyze.output}}
