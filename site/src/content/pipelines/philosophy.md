---
title: "GL1TCH Philosophy"
description: "Foundational design principles: local-first, terminal-native, pipeline-driven, ownership-centered."
order: 0
---

GL1TCH exists because AI should be *your* tool, not a service you call. The web and SaaS platforms move intelligence to someone else's cloud. GL1TCH moves it into your terminal—on your hardware, under your control, with no subscription, no API keys, no surveillance of your prompts. You own the workspace, the workflows, the brain context, all of it.


## Why Tmux

GL1TCH is a tmux session, not a web UI or electron app. This is intentional.

Tmux is durable: you can detach at any moment and reconnect later. Your pipelines keep running. Your tmux history survives terminal crashes. Tmux is composable: you can tile multiple panels, jump between windows, pipe output to other Unix tools. Tmux exists everywhere—local machine, remote server, container, CI runner. And crucially: tmux is *yours*. It has zero network calls, zero external dependencies on platforms, zero "service status" pages that can take your workspace down.

The Switchboard (GL1TCH's control window) fills your terminal. One full-screen session where you manage pipelines, read agent responses, launch jobs, and inspect output. Everything is text. Everything is keyboard-driven. You can script around it, fork it, mod it, share your config.


## Why Local Ollama

GL1TCH uses Ollama for all critical LLM decisions in the hot path: pipeline steps, brain vector embeddings, workflow routing, agent dispatch. Ollama runs on your machine. The models (llama3.2, mistral, codestral) are open-weight and run locally. No prompt leaves your terminal except if you explicitly route a step to a cloud provider.

This is not a purity choice—it's a necessity. Sending every prompt to the cloud:
- Leaks your work context, your code, your business logic
- Adds network latency on every decision
- Requires persistent internet; your AI stops working offline
- Makes you dependent on a company's API pricing, uptime, rate limits, and terms of service
- Means the model can be yanked or changed without your consent

Local Ollama is slower than Claude or GPT for some tasks. That's the tradeoff. But it's fast *enough*, it's under your control, and it never wakes up a stranger with your secrets.

Pipeline steps can reference cloud providers (e.g., `provider: claude/claude-sonnet-4-6`) if you choose. But the orchestrator itself—the decision engine that routes, branches, and scores—stays local.


## Why Pipelines

A pipeline is a directed acyclic graph of steps declared in YAML. Each step has a provider (local Ollama or cloud LLM), a prompt, optional tags for brain context injection, and input/output contracts.

The pipeline model is *declarative and auditable*. You can read a `.pipeline.yaml` file and understand exactly what will run, in what order, with what context, and what the expected outcomes are. You don't hand-stitch bash scripts that lose history. You don't build hidden chains of function calls inside a notebook. You write the flow once, version it, run it deterministically, and trust it.

Pipelines solve the "lost context" problem: once a pipeline finishes, its outputs are checkpointed to the brain—your embedding store. The next pipeline that runs can see what happened before, without asking the user to re-explain. The brain makes your AI workspace *smarter over time* instead of reset-on-every-session.

GL1TCH also adds orchestration (workflows) above the pipeline layer: you can sequence multiple pipelines, make branching decisions, fan out parallel jobs, and checkpoint the entire run. All declarative. All auditable.


## What GL1TCH Is Not

**Not a SaaS platform.** No account. No servers. No vendor lock-in.

**Not a closed AI product.** The source code is open. The config files are plain YAML. The brain is a SQLite database in your home directory. You can inspect, backup, migrate, or nuke it anytime.

**Not a web UI disguised as an app.** No electron wrapper. No "cloud sync" of your workspace. No slow network round-trip on every keystroke.

**Not centralized control.** There is no "admin console" or "enterprise dashboard." GL1TCH is not a platform for managing other people's work. It's a personal automation workspace.

**Not a closed LLM ecosystem.** You can mix providers: Ollama for local reasoning, Claude for complex analysis, custom scripts for deterministic logic. The pipeline runner doesn't care. You own the decision of where the intelligence lives.

**Not forced to the cloud.** GL1TCH works offline. It requires Ollama and tmux. It does not require Docker, Kubernetes, GitHub, or a cloud account. You can run it on a laptop, a Raspberry Pi, a home server, or a VPS—anywhere Go runs.


## Core Principles

**Ownership.** You own your workspace, your data, your workflows, and your AI context. Not a company. Not an API vendor. You.

**Visibility.** Everything happens in one terminal session. You can see pipelines run, agents think, jobs queue, and results land. Debugging is reading the session. Monitoring is watching the screen.

**Durability.** Detach and come back later. Your jobs keep running. Your prompts stay private. Your context persists in the brain.

**Auditability.** Pipelines are YAML. Decisions are explicit. Runs are logged. You can replay a run, inspect its inputs, trace where it broke.

**Composability.** Pipelines call other pipelines. Agents output goes to the brain. The brain feeds future pipelines. Your workspace learns and chains together over time.

**Sovereignty.** No vendor lock-in. No cloud dependency. No terms of service gatekeeping your automation. Run your AI on *your* infrastructure.


## See Also

- [Console Overview](./console-overview.md) — The Switchboard and control panel
- [Pipelines](./pipelines.md) — Writing and running pipeline YAML
- [Brain](./brain.md) — Context storage and embedding system

