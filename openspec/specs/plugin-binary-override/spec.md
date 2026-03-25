## Requirements

### Requirement: Override binary lookup before built-in subcommand
The widget dispatch layer SHALL call `exec.LookPath("orcai-" + name)` before invoking the built-in `orcai <name>` subcommand. If an override binary is found, dispatch SHALL execute it in place of the built-in. The override binary receives the same arguments and environment that the built-in subcommand would receive, including `--bus-socket` if applicable.

#### Scenario: Override binary found and executed
- **WHEN** `orcai-sysop` exists on PATH and the dispatch layer is asked to launch widget `sysop`
- **THEN** dispatch executes `orcai-sysop` instead of `orcai sysop`

#### Scenario: No override binary falls back to built-in
- **WHEN** no `orcai-sysop` binary exists on PATH and the dispatch layer is asked to launch widget `sysop`
- **THEN** dispatch executes `orcai sysop`

#### Scenario: Override binary receives bus socket flag
- **WHEN** `orcai-welcome` is found on PATH and the bus socket is active
- **THEN** dispatch passes `--bus-socket <path>` to `orcai-welcome` the same way it would to the built-in

### Requirement: Override lookup is PATH-only, no config required
The dispatch layer SHALL NOT require any configuration file entry to activate an override. Placing `orcai-<name>` anywhere in `$PATH` is sufficient. No manifest, no YAML entry, no restart of the bus daemon.

#### Scenario: Override activated without config changes
- **WHEN** a user copies `orcai-picker` to `/usr/local/bin/` (which is on PATH)
- **THEN** the next widget launch for `picker` uses the user's binary without any orcai config change

### Requirement: Self-referential override is detected and rejected
If the resolved override binary path is the same executable as the running orcai process, dispatch SHALL skip the override and fall back to the built-in subcommand to prevent infinite exec loops.

#### Scenario: Self-referential override skipped
- **WHEN** `orcai-sysop` on PATH resolves to the same inode as the running `orcai` binary
- **THEN** dispatch skips the override, logs a warning, and falls back to `orcai sysop`

### Requirement: Override binary exit code surfaced
If an override binary exits with a non-zero code, the dispatch layer SHALL surface the error to its caller with the exit code and the override binary's stderr output included in the error message.

#### Scenario: Override binary failure propagated
- **WHEN** `orcai-sysop` exits with code 1 and writes "config not found" to stderr
- **THEN** the dispatch layer returns an error containing exit code 1 and "config not found"
