## ADDED Requirements

### Requirement: Hacker-voice marketing copy on Home screen
The Home screen SHALL display punchy, hacker-voice copy that communicates orcai's value proposition: local-first AI workspace, keyboard-driven, pipeline orchestration, no cloud lock-in.

#### Scenario: Taglines present
- **WHEN** the Home screen is rendered
- **THEN** it SHALL display at least 3 short taglines in the BBS `>` prompt style that convey the core value (e.g., "your agents. your terminal. your rules.")

#### Scenario: Feature highlight grid
- **WHEN** the Home screen is rendered
- **THEN** it SHALL include a feature grid or list calling out: pipelines, agent runner, brain context, themes, keyboard navigation

### Requirement: Ollama supercharger callout on Home screen
The Home screen SHALL include a visible callout that specifically highlights Ollama + local model support as a differentiator.

#### Scenario: Ollama callout renders
- **WHEN** the Home screen is rendered
- **THEN** it SHALL display a callout section with heading "OLLAMA SUPERCHARGER" or equivalent, explaining that local models run with zero API cost and full privacy

#### Scenario: Ollama callout includes model examples
- **WHEN** the Ollama callout is rendered
- **THEN** it SHALL list at least 2 example local models (e.g., `qwen2.5-coder`, `llama3.2`)

### Requirement: Visual parity with orcai TUI
The Home screen and all site screens SHALL visually match the orcai app's ABS Dark default theme: same background color (`#0d1117` or equivalent near-black), same foreground, same accent colors (cyan, green, purple, yellow) from the Dracula-derived palette used in the app.

#### Scenario: Color tokens match app
- **WHEN** the site CSS is inspected
- **THEN** the CSS custom properties for `--bg`, `--fg`, `--cyan`, `--green`, `--purple`, `--yellow`, `--comment` SHALL match the hex values used in the orcai ABS Dark theme definition

#### Scenario: Border/panel style matches app panels
- **WHEN** content panels are rendered on the site
- **THEN** the panel headers SHALL use the same double-line box drawing characters (`╔═╗`, `╠═╣`, `╚═╝`) and color treatment as the orcai TUI panel borders shown in the screenshots

### Requirement: Screenshots on Home screen
The Home screen SHALL embed at least one TUI screenshot to immediately show new visitors what orcai looks like in action.

#### Scenario: Screenshot present on home
- **WHEN** the Home screen is rendered
- **THEN** at least one `<img>` from `public/screenshots/` SHALL be visible with a BBS frame and caption
