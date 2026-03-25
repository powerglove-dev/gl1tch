## ADDED Requirements

### Requirement: Resize-pane actions are available in the action map
The keybindings package SHALL expose four resize-pane action names — `resize-pane-left`, `resize-pane-right`, `resize-pane-up`, and `resize-pane-down` — each mapped to the corresponding `tmux resize-pane` direction flag with a fixed step of 5 cells.

#### Scenario: resize-pane-left bound to a key
- **WHEN** `keybindings.yaml` contains a binding with `action: resize-pane-left`
- **THEN** orcai runs `tmux bind-key <key> resize-pane -L 5` at startup

#### Scenario: resize-pane-right bound to a key
- **WHEN** `keybindings.yaml` contains a binding with `action: resize-pane-right`
- **THEN** orcai runs `tmux bind-key <key> resize-pane -R 5` at startup

#### Scenario: resize-pane-up bound to a key
- **WHEN** `keybindings.yaml` contains a binding with `action: resize-pane-up`
- **THEN** orcai runs `tmux bind-key <key> resize-pane -U 5` at startup

#### Scenario: resize-pane-down bound to a key
- **WHEN** `keybindings.yaml` contains a binding with `action: resize-pane-down`
- **THEN** orcai runs `tmux bind-key <key> resize-pane -D 5` at startup

### Requirement: config init includes example resize bindings
When `orcai config init` writes the default `keybindings.yaml`, it SHALL include example bindings for all four resize-pane actions alongside the existing split and navigation defaults.

#### Scenario: Default keybindings file contains resize entries
- **WHEN** `orcai config init` is run and no `keybindings.yaml` exists
- **THEN** the generated file contains bindings for `resize-pane-left`, `resize-pane-right`, `resize-pane-up`, and `resize-pane-down`
