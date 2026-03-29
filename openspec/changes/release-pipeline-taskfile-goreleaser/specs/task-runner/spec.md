## ADDED Requirements

### Requirement: Taskfile replaces Makefile as primary task runner
The project SHALL use `Taskfile.yml` (Task v3) as the sole task runner. The `Makefile` SHALL be removed. All contributors SHALL use `task <name>` to run project operations.

#### Scenario: Developer builds the binary
- **WHEN** a developer runs `task build`
- **THEN** the `orcai` binary is compiled to `bin/orcai` using `go build`

#### Scenario: Developer installs locally
- **WHEN** a developer runs `task install`
- **THEN** the binary is built and installed via `go install` with a symlink to `~/.local/bin/orcai`

#### Scenario: Developer runs tests
- **WHEN** a developer runs `task test`
- **THEN** `go test ./...` executes and results are printed

#### Scenario: Developer starts the app (clean run)
- **WHEN** a developer runs `task run:clean`
- **THEN** existing tmux sessions are killed, config/db files are removed, and orcai is started fresh

#### Scenario: Developer starts the app (normal run)
- **WHEN** a developer runs `task run`
- **THEN** orcai is started without wiping the database or config

#### Scenario: Developer starts a debug session
- **WHEN** a developer runs `task debug`
- **THEN** the binary is compiled with debug symbols and Delve listens on `:2345`

#### Scenario: Developer connects to an existing debug session
- **WHEN** a developer runs `task debug:connect`
- **THEN** `dlv connect :2345` is executed

#### Scenario: Developer starts debug in tmux
- **WHEN** a developer runs `task debug:tmux`
- **THEN** the `scripts/debug-tmux.sh` script is executed

#### Scenario: Task list is self-documenting
- **WHEN** a developer runs `task --list`
- **THEN** all tasks with descriptions are printed in a readable format

### Requirement: Taskfile provides release helper tasks
The Taskfile SHALL include tasks for creating snapshot builds and tagging releases.

#### Scenario: Developer creates a snapshot build
- **WHEN** a developer runs `task release:snapshot`
- **THEN** `goreleaser release --snapshot --clean` executes and artifacts appear in `dist/`

#### Scenario: Developer tags a new release
- **WHEN** a developer runs `task release:tag VERSION=v1.2.3`
- **THEN** a signed annotated git tag `v1.2.3` is created and the developer is prompted to push it

### Requirement: Taskfile supports cross-platform execution
The Taskfile SHALL work on Linux, macOS, and (for CI) Windows without requiring bash-specific syntax in task bodies.

#### Scenario: CI runs Taskfile on ubuntu-latest
- **WHEN** a GitHub Actions job on `ubuntu-latest` runs `task build`
- **THEN** the build completes successfully without shell compatibility errors
