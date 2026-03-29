## Why

The ORCAI website is a bare-minimum install guide — it doesn't showcase the product's power, doesn't excite hackers, and leaves new users without a path from zero to running their first pipeline with local AI. Ollama+local model integration is a killer feature that goes completely unmentioned.

## What Changes

- **Overhaul the Getting Started screen** into a full multi-step walkthrough: install orcai → install recommended plugins → set up Ollama → run a pipeline → run an agent
- **Add a new `use_brain` documentation section** explaining the brain context system with usage examples, annotated pipeline YAML, and tips for building a smarter AI workspace
- **Add a new Labs screen** with 4–6 anonymized, runnable pipeline examples derived from real local pipelines (brain feedback loop, Ollama E2E, code review, activity digest, etc.)
- **Add real screenshots** of the ORCAI TUI (switchboard, agent modal, theme picker, cron, jump-to-window) embedded in the site with the existing BBS aesthetic
- **Overhaul the Home screen** with sharper hacker-voice marketing copy, emphasis on "runs local, no cloud required", and a prominent Ollama callout
- **Add a new Docs screen** (nav slot [8]) covering: use_brain reference, pipeline YAML reference, brain tips
- Fully keyboard-driven site navigation already exists — extend key hints to new screens

## Capabilities

### New Capabilities

- `site-screenshots`: Embed real TUI screenshots in the site; captured PNGs rendered inline with BBS-styled frames and captions
- `site-labs-screen`: New Labs screen ([8] LABS) showcasing anonymized runnable pipeline examples with annotations and keyboard-driven navigation
- `site-docs-screen`: New Docs screen ([9] DOCS) with use_brain reference, pipeline YAML guide, and workspace tips
- `site-getting-started-overhaul`: Expanded multi-step Getting Started screen covering install, plugins, Ollama, first pipeline, first agent
- `site-home-marketing`: Updated Home screen with hacker-voice copy, Ollama supercharger callout, and feature highlights

### Modified Capabilities

- `site-nav`: Nav bar gains two new entries ([8] LABS, [9] DOCS) and updated key hints

## Impact

- `site/src/components/screens/` — new LabsScreen.astro, DocsScreen.astro; modified HomeScreen.astro, GettingStartedScreen.astro
- `site/src/components/Nav.astro` — add labs and docs nav entries
- `site/src/pages/index.astro` — import and mount new screens
- `site/public/screenshots/` — new directory for TUI PNG screenshots
- `site/src/content/labs/` — MDX/YAML lab definitions (anonymized pipeline stubs)
- No backend changes, no Go changes, no breaking API changes
