## ADDED Requirements

### Requirement: Docs screen accessible via keyboard
The site SHALL expose a Docs screen reachable by pressing `9` or clicking `[9] DOCS` in the nav bar.

#### Scenario: Key navigation to docs
- **WHEN** the user presses `9` on any screen
- **THEN** the Docs screen SHALL become visible and all other screens SHALL be hidden

#### Scenario: Nav item present
- **WHEN** the site loads
- **THEN** the nav bar SHALL include a `[9] DOCS` button

### Requirement: use_brain reference documentation
The Docs screen SHALL contain a dedicated section explaining `use_brain` and `write_brain` pipeline fields: what they do, when to use each, and how they interact.

#### Scenario: use_brain definition present
- **WHEN** the Docs screen is rendered
- **THEN** it SHALL display a definition of `use_brain: true` explaining that it injects accumulated brain context into the step prompt preamble

#### Scenario: write_brain definition present
- **WHEN** the Docs screen is rendered
- **THEN** it SHALL display a definition of `write_brain: true` explaining that the step's output is appended to the brain context store for the current run

#### Scenario: Step-level override documented
- **WHEN** the use_brain section is rendered
- **THEN** it SHALL document that `use_brain: false` on an individual step suppresses brain injection for that step even when the pipeline-level flag is `true`

### Requirement: Annotated use_brain YAML examples
The Docs screen SHALL show at minimum 2 annotated YAML examples demonstrating `use_brain`/`write_brain` usage.

#### Scenario: Brain feedback loop example
- **WHEN** the use_brain examples are rendered
- **THEN** one example SHALL show a two-step pipeline where step 1 has `write_brain: true` and step 2 has `use_brain: true`, demonstrating context carry-forward

#### Scenario: Mixed brain usage example
- **WHEN** the use_brain examples are rendered
- **THEN** one example SHALL show a pipeline with `use_brain: true` at the top level and one step with `use_brain: false` to suppress injection for a data-fetch step

### Requirement: Brain workspace tips
The Docs screen SHALL include a "tips" section with practical advice on making an AI workspace smarter using brain context.

#### Scenario: At least 4 tips present
- **WHEN** the tips section is rendered
- **THEN** it SHALL contain at least 4 distinct tips

#### Scenario: Tips cover write strategy
- **WHEN** the tips are rendered
- **THEN** at least one tip SHALL address when to write brain notes vs. when to suppress injection

### Requirement: Pipeline YAML quick-reference
The Docs screen SHALL include a concise pipeline YAML field reference table covering the most commonly used fields.

#### Scenario: Core fields present in reference
- **WHEN** the YAML reference is rendered
- **THEN** it SHALL document at minimum: `name`, `version`, `use_brain`, `write_brain`, `steps[].id`, `steps[].executor`/`plugin`, `steps[].model`, `steps[].prompt`
