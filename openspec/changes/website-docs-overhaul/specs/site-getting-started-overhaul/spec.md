## ADDED Requirements

### Requirement: Multi-step getting started walkthrough
The Getting Started screen SHALL present a 5-step sequential walkthrough: (1) Install orcai, (2) Install recommended plugins, (3) Set up Ollama, (4) Run your first pipeline, (5) Run your first agent.

#### Scenario: All 5 steps visible
- **WHEN** the Getting Started screen is rendered
- **THEN** it SHALL display exactly 5 numbered sections with BBS-bordered step headers

#### Scenario: Step 1 — Install orcai
- **WHEN** step 1 is displayed
- **THEN** it SHALL show the `go install` command and note that `orcai` lands in `$GOPATH/bin`

#### Scenario: Step 2 — Recommended plugins
- **WHEN** step 2 is displayed
- **THEN** it SHALL list the recommended plugins (claude, ollama, gh, jq) with install commands or config snippets

#### Scenario: Step 3 — Ollama setup
- **WHEN** step 3 is displayed
- **THEN** it SHALL show `ollama pull qwen2.5-coder:latest` and `ollama serve`, explain why Ollama supercharges orcai (local, free, fast, private), and call out that local models work with pipelines and the agent runner

#### Scenario: Step 4 — Run first pipeline
- **WHEN** step 4 is displayed
- **THEN** it SHALL show `orcai pipeline run brain-ollama-e2e` (or an equivalent starter pipeline), explain the switchboard pipeline panel, and show expected output

#### Scenario: Step 5 — Run first agent
- **WHEN** step 5 is displayed
- **THEN** it SHALL show how to open the agent modal (Enter key on the Switchboard), select Ollama, pick a model, enter a prompt, and submit with Ctrl+S

### Requirement: Ollama supercharger callout block
The Getting Started screen SHALL include a prominent callout block emphasizing that Ollama + local models enable fully offline, private AI workflows.

#### Scenario: Ollama callout styled distinctly
- **WHEN** the Ollama callout is rendered
- **THEN** it SHALL use a distinct BBS border style (e.g., double-line box with `fg-green` or `fg-yellow` color) and the text SHALL include the phrase "no API key required"

### Requirement: Copyable code blocks
All shell commands in the Getting Started screen SHALL be wrapped in copyable code blocks matching the existing site style.

#### Scenario: Code block has copy affordance
- **WHEN** a code block is rendered in Getting Started
- **THEN** it SHALL use the `copy-target` class pattern so the site's existing copy-on-click JS applies
