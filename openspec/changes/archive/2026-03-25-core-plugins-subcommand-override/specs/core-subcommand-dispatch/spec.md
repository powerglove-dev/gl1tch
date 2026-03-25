## ADDED Requirements

### Requirement: Core widgets registered as cobra subcommands
The orcai binary SHALL register `sysop`, `picker`, and `welcome` as top-level `cobra` subcommands. Each subcommand SHALL start the corresponding BubbleTea component in a standalone terminal session and block until the component exits.

#### Scenario: Sysop subcommand runs standalone
- **WHEN** the user runs `orcai sysop`
- **THEN** the sysop panel launches in the current terminal and exits cleanly when dismissed

#### Scenario: Picker subcommand runs standalone
- **WHEN** the user runs `orcai picker`
- **THEN** the session picker launches in the current terminal and exits cleanly when a selection is made or the user cancels

#### Scenario: Welcome subcommand runs standalone
- **WHEN** the user runs `orcai welcome`
- **THEN** the welcome dashboard launches in the current terminal and exits cleanly when dismissed

### Requirement: Widget dispatch invokes core widgets via exec
The widget dispatch layer SHALL invoke core widgets by executing `orcai <name>` as a child process rather than calling their Go implementations directly. This ensures the override lookup path in `plugin-binary-override` is exercised uniformly for both built-in and external widgets.

#### Scenario: Dispatch calls orcai subcommand for built-in widget
- **WHEN** the dispatch layer is asked to launch widget `sysop` and no `orcai-sysop` binary is found in PATH
- **THEN** dispatch executes `orcai sysop` as a child process

#### Scenario: Subcommand exit propagates to dispatch layer
- **WHEN** the `orcai sysop` child process exits with a non-zero code
- **THEN** the dispatch layer surfaces the error to its caller with the exit code included

### Requirement: Core subcommands accept bus socket path via flag
Each core widget subcommand SHALL accept a `--bus-socket` flag specifying the orcai bus Unix socket path. When provided, the widget SHALL connect to the bus and subscribe to relevant events. When absent, the widget SHALL run in standalone mode without bus connectivity.

#### Scenario: Subcommand connects to bus when flag provided
- **WHEN** `orcai welcome --bus-socket /run/orcai/bus.sock` is executed
- **THEN** the welcome widget connects to the specified socket and subscribes to `theme.changed` events

#### Scenario: Subcommand runs without bus when flag absent
- **WHEN** `orcai welcome` is executed with no flags
- **THEN** the welcome widget starts successfully and does not attempt any socket connection
