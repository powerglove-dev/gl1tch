## MODIFIED Requirements

### Requirement: Status bar shows session control hints
The tmux status bar right section SHALL display chord-key hints for new-session and prompt-builder actions alongside the existing clock. The hints SHALL NOT include a `^spc t switchboard` segment. The theme picker hint SHALL use `^spc t themes` (not `^spc m themes`).

#### Scenario: Status bar contains new-session hint
- **WHEN** an orcai session is running
- **THEN** the tmux status bar right side contains a visible hint referencing the new-session chord

#### Scenario: Status bar contains prompt-builder hint
- **WHEN** an orcai session is running
- **THEN** the tmux status bar right side contains a visible hint referencing the prompt-builder chord

#### Scenario: Status bar does not reference switchboard chord
- **WHEN** an orcai session is running
- **THEN** the tmux status bar does NOT contain any hint referencing `^spc t switchboard`

#### Scenario: Status bar shows theme picker as ^spc t
- **WHEN** an orcai session is running
- **THEN** the tmux status bar contains a hint `^spc t` for the theme picker (not `^spc m`)

#### Scenario: Clock remains visible
- **WHEN** an orcai session is running
- **THEN** the tmux status bar still shows the current time

## REMOVED Requirements

### Requirement: ^spc t navigates to switchboard
**Reason**: The switchboard will be accessible via orcai-switchboard in the jump-to-window modal (`^spc j`). A dedicated top-level chord for it is no longer needed.
**Migration**: Use `^spc j` and select `orcai-switchboard` from the jump-to-window list.
