---
title: "Technologies"
description: "List what runs under the hood and what each piece does for you."
order: 10
---

gl1tch is built from a small set of tools chosen for one reason: they let your AI run locally, in your terminal, under your control. Here's what each piece does for you.

## Stack

| Technology | What it does for you |
|-----------|---------------------|
| **Ollama** | Runs AI models on your machine. No API key. No data leaves your terminal. Used for routing decisions, brain embeddings, and local pipeline steps. |
| **SQLite** | Stores your run history, brain context, and saved prompts locally. No database server to manage. Query it with any SQLite tool if you want. |
| **Go** | Ships as a single binary with no runtime dependencies. Copy it to `~/bin/` and run it. Works on Linux, macOS, and anywhere else Go runs. |
| **GitHub CLI** | Handles GitHub authentication without storing tokens in gl1tch config. Pipeline steps that need GitHub just call `gh`. |
| **YAML** | Your pipelines and workflows are plain YAML. Read them, edit them, version them, share them. |

## Why Local-First

Every routing decision, brain retrieval, and dispatch call in gl1tch uses a model running on your machine. The goal is simple: your work context never leaves your terminal unless you explicitly send it somewhere.

Cloud models (Claude, etc.) are available for individual pipeline steps when you need their capabilities. But the orchestration layer — what runs when, in what order, with what context — stays local. You decide what goes to the cloud. gl1tch doesn't make that call for you.

## See Also

- [Philosophy](/docs/pipelines/philosophy) — the reasoning behind these choices
- [Architecture](/docs/pipelines/architecture) — how the pieces fit together
- [Executors](/docs/pipelines/executors) — configuring AI providers in your pipelines
