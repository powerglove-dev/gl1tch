## ADDED Requirements

### Requirement: Layout config is optional and no-op when absent
Orcai SHALL read `~/.config/orcai/layout.yaml` at session attach time. If the file does not exist, orcai SHALL skip all layout initialization and leave the tmux environment untouched. No warning or error SHALL be emitted for a missing layout file.

#### Scenario: No layout file means no pane changes
- **WHEN** `orcai attach` is run and `~/.config/orcai/layout.yaml` does not exist
- **THEN** orcai does not create any tmux panes and does not launch any widgets automatically

#### Scenario: Empty layout file is valid and no-op
- **WHEN** `layout.yaml` exists but contains an empty `panes` list
- **THEN** orcai performs no layout operations and emits no errors

### Requirement: Layout config declares panes with widget and position
The layout config SHALL support a top-level `panes` list. Each entry SHALL specify at minimum: `name` (string, unique), `widget` (string, matches a known widget name), `position` (one of: `left`, `right`, `top`, `bottom`), and `size` (string, percentage or absolute columns/rows, e.g. `40%` or `80`).

#### Scenario: Single pane declared and created
- **WHEN** `layout.yaml` declares one pane with `widget: welcome`, `position: left`, `size: 40%`
- **THEN** orcai creates a left split 40% wide in the current tmux window and launches the welcome widget in it

#### Scenario: Multiple panes created in declaration order
- **WHEN** `layout.yaml` declares two panes — sysop on the right at 60% and welcome on the left at 40%
- **THEN** orcai creates both panes and launches the respective widgets in each

#### Scenario: Invalid position value rejected
- **WHEN** a pane declares `position: diagonal`
- **THEN** orcai logs an error for that pane, skips it, and continues processing remaining panes

### Requirement: Existing panes are not duplicated on re-attach
Before creating a pane, orcai SHALL check whether a tmux pane with the same `name` already exists in the current window. If it does, orcai SHALL skip creation for that pane without error.

#### Scenario: Re-attach to session with existing widget panes
- **WHEN** `orcai attach` is run on a session where the sysop pane already exists
- **THEN** orcai does not create a second sysop pane

#### Scenario: New pane created when named pane absent
- **WHEN** `orcai attach` is run on a session where the welcome pane does not exist
- **THEN** orcai creates the welcome pane as configured

### Requirement: Keybindings config controls which tmux keys orcai binds
Orcai SHALL read `~/.config/orcai/keybindings.yaml` and bind only the keys listed there. Keys absent from the file SHALL be left untouched in tmux. If the file does not exist, orcai SHALL bind no keys.

#### Scenario: Key listed in config is bound
- **WHEN** `keybindings.yaml` contains `key: "M-s"` with `action: launch-session-picker`
- **THEN** orcai runs `tmux bind-key M-s ...` to register that binding at startup

#### Scenario: Key absent from config is not bound
- **WHEN** `keybindings.yaml` exists but does not list `M-p`
- **THEN** orcai does not run any `tmux bind-key` for `M-p` and any existing user binding for that key is preserved

#### Scenario: No keybindings file means no bindings
- **WHEN** `~/.config/orcai/keybindings.yaml` does not exist
- **THEN** orcai binds no tmux keys at startup

### Requirement: config init writes default layout and keybindings files
`orcai config init` SHALL write default `layout.yaml` and `keybindings.yaml` files to `~/.config/orcai/` containing the classic orcai layout and keybinding defaults. If a file already exists, orcai SHALL prompt the user before overwriting.

#### Scenario: Config init creates files when absent
- **WHEN** `orcai config init` is run and neither config file exists
- **THEN** both `layout.yaml` and `keybindings.yaml` are created with opinionated defaults and orcai prints their paths

#### Scenario: Config init prompts before overwrite
- **WHEN** `orcai config init` is run and `layout.yaml` already exists
- **THEN** orcai asks the user to confirm before overwriting the existing file
