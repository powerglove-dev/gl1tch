## ADDED Requirements

### Requirement: Labs screen accessible via keyboard
The site SHALL expose a Labs screen reachable by pressing `8` or clicking `[8] LABS` in the nav bar.

#### Scenario: Key navigation to labs
- **WHEN** the user presses `8` on any screen
- **THEN** the Labs screen SHALL become visible and all other screens SHALL be hidden

#### Scenario: Nav item present
- **WHEN** the site loads
- **THEN** the nav bar SHALL include a `[8] LABS` button that switches to the labs screen on click

### Requirement: Labs display anonymized pipeline examples
The Labs screen SHALL present 4–6 annotated pipeline examples derived from real use cases, with all private repository references removed and replaced with generic placeholders.

#### Scenario: Each lab has required fields
- **WHEN** a lab entry is rendered
- **THEN** it SHALL display: a title, a one-line description, a prerequisites list, and a YAML snippet of the pipeline

#### Scenario: Prerequisites call out Ollama when needed
- **WHEN** a lab uses an Ollama model in its pipeline
- **THEN** the prerequisites list SHALL include "Ollama running with `<model>` pulled"

#### Scenario: Minimum lab count
- **WHEN** the Labs screen is rendered
- **THEN** it SHALL display at least 4 labs

### Requirement: Labs screen keyboard-navigable within screen
The Labs screen SHALL support `j`/`k` or arrow keys to scroll between labs, consistent with the orcai TUI UX.

#### Scenario: j/k navigation hint visible
- **WHEN** the Labs screen is active
- **THEN** a status line at the bottom SHALL show `j/k navigate · enter run · esc back`

### Requirement: Labs cover brain, Ollama, and cloud agent use cases
The lab selection SHALL include at least one lab for each of: brain context feedback loop (Ollama), local code review (Ollama), multi-step pipeline with cloud AI, and activity digest.

#### Scenario: Brain lab present
- **WHEN** the labs are rendered
- **THEN** at least one lab title SHALL reference "brain" or "memory"

#### Scenario: Ollama lab present
- **WHEN** the labs are rendered
- **THEN** at least one lab SHALL list Ollama as a prerequisite
