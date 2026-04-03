---
title: "Philosophy"
description: "Explain what gl1tch is, what it is not, and why it works the way it does."
order: 0
---

gl1tch is a personal AI assistant that lives in your terminal. It runs your automations, learns your projects, and adapts to your workflow — entirely on your machine, without sending your work to a service you don't control.

That's the whole idea. Every design decision comes back to it.

## What You Get

Your assistant, your pipelines, your data. Nothing requires a cloud account. Nothing phones home. You can run gl1tch on a laptop, a home server, or a VPS — anywhere that runs a terminal.

When you run a pipeline, the output stays local. When gl1tch learns context from your work (via the brain store), it stays in a database on your disk. When you stop paying attention and come back later, your workspace is exactly where you left it.

## Why Pipelines

A pipeline is a YAML file. You can read it, version it, and share it. You know exactly what runs, in what order, with what input.

This matters because AI workflows have a trust problem. When logic lives inside a chat conversation or a hidden function chain, you can't inspect it. You can't replay it. You can't debug it. A pipeline solves that: the logic is declared up front, the execution is logged, and the results are stored locally.

gl1tch also builds on pipelines rather than hiding them. Your assistant uses pipelines internally for routing and dispatch — the same format you write yourself.

## Why Local-First

Sending every prompt to the cloud leaks your work context. It adds latency. It requires persistent internet. It makes you dependent on API pricing and uptime you don't control.

Local Ollama is slower than a large cloud model for some tasks. That's the tradeoff. But it's fast enough for routing decisions and quick transforms, it never wakes up a stranger's server with your code, and it works offline.

You can still use cloud models — Claude, for example — for steps that benefit from a larger model. But the decision engine (routing, branching, dispatch) stays local. You choose what leaves your terminal.

## What gl1tch Is Not

**Not a SaaS product.** No account. No subscription. No servers you don't own.

**Not a chat interface.** gl1tch is not ChatGPT in your terminal. You're orchestrating workflows and building context over time — not trading messages back and forth.

**Not a black box.** The config is plain YAML. The brain is a SQLite database in your home directory. The run history is inspectable. You can export it, back it up, or nuke it.

**Not locked to one AI provider.** Mix Ollama for local reasoning and Claude for complex tasks. The pipeline runner doesn't care. You decide where each step runs.

**Not for managing other people's work.** gl1tch is yours. It extends your workspace, not a team platform.

## Core Principles

**Ownership.** Your workspace. Your data. Your pipelines. Not a vendor's.

**Visibility.** Everything happens in your terminal. Pipelines run in front of you. Results are readable text.

**Durability.** Your workspace keeps running when you detach. Your context persists in the brain. Nothing resets between sessions.

**Auditability.** Pipelines are YAML. Runs are logged. You can replay any run and inspect exactly what happened.

**Composability.** Pipelines call other pipelines. The brain feeds future runs. Your workspace gets smarter over time.

## See Also

- [Architecture](/docs/pipelines/architecture) — what happens when you run a pipeline
- [Pipeline YAML Reference](/docs/pipelines/yaml-reference) — writing your first pipeline
- [Technologies](/docs/pipelines/technologies) — what runs under the hood
