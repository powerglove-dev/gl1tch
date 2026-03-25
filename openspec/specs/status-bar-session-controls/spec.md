## ADDED Requirements

### Requirement: Status bar shows session control hints
The tmux status bar right section SHALL display chord-key hints for new-session and prompt-builder actions alongside the existing clock. The hints SHALL use the format `^spc n new  ^spc p build` to communicate the `ctrl+space` chord prefix.

#### Scenario: Status bar contains new-session hint
- **WHEN** an orcai session is running
- **THEN** the tmux status bar right side contains a visible hint referencing the new-session chord

#### Scenario: Status bar contains prompt-builder hint
- **WHEN** an orcai session is running
- **THEN** the tmux status bar right side contains a visible hint referencing the prompt-builder chord

#### Scenario: Clock remains visible
- **WHEN** an orcai session is running
- **THEN** the tmux status bar still shows the current time

### Requirement: New-session and prompt-builder removed from sidebar footer
The sidebar footer SHALL NOT show `n new` or `p build` hints. These actions are accessed exclusively via chord keys, advertised in the status bar.

#### Scenario: Sidebar footer shows only navigation hints
- **WHEN** the agent context panel sidebar is visible
- **THEN** the sidebar footer shows only navigation hints (focus, kill, nav) and does not mention new-session or prompt-builder
