## MODIFIED Requirements

### Requirement: Sidecar YAML schema
A sidecar file at `~/.config/orcai/wrappers/<name>.yaml` SHALL declare the CLI tool's name, description, command, optional args, and optional input/output schemas. All fields except `command` SHALL be optional. The sidecar MUST include a comment block documenting the calling convention: `input` is passed as stdin to the subprocess; `vars` entries are passed as environment variables prefixed with `ORCAI_`.

#### Scenario: Minimal sidecar (command only)
- **WHEN** a sidecar file contains only `command: echo`
- **THEN** `NewCliAdapterFromSidecar` returns a valid CliAdapter that executes `echo`

#### Scenario: Full sidecar
- **WHEN** a sidecar file declares `name`, `description`, `command`, `args`, `input_schema`, `output_schema`
- **THEN** the resulting CliAdapter reflects all declared values

### Requirement: Load CliAdapter from sidecar file
`NewCliAdapterFromSidecar(path string)` SHALL parse the YAML sidecar at `path` and return a `*CliAdapter` populated with the sidecar's fields. It SHALL return an error if the file cannot be read or if `command` is empty.

#### Scenario: Valid sidecar returns adapter
- **WHEN** `NewCliAdapterFromSidecar` is called with a valid sidecar path
- **THEN** it returns a non-nil `*CliAdapter` and nil error

#### Scenario: Missing command returns error
- **WHEN** a sidecar file has no `command` field
- **THEN** `NewCliAdapterFromSidecar` returns a non-nil error

#### Scenario: Unreadable file returns error
- **WHEN** `NewCliAdapterFromSidecar` is called with a path that does not exist
- **THEN** it returns a non-nil error

### Requirement: Capabilities populated from sidecar schema
A `CliAdapter` loaded from a sidecar SHALL return a `[]Capability` from `Capabilities()` containing one entry with `InputSchema` and `OutputSchema` from the sidecar. A `CliAdapter` created via `NewCliAdapter` (no sidecar) SHALL continue to return nil from `Capabilities()`.

#### Scenario: Sidecar with schema returns capability
- **WHEN** a sidecar declares `input_schema: "string"` and `output_schema: "string"`
- **THEN** `adapter.Capabilities()` returns a slice with one entry containing those values

#### Scenario: Sidecar without schema returns capability with empty schemas
- **WHEN** a sidecar omits `input_schema` and `output_schema`
- **THEN** `adapter.Capabilities()` returns a slice with one entry where both schema fields are empty strings

#### Scenario: Non-sidecar adapter unchanged
- **WHEN** a `CliAdapter` is created via `NewCliAdapter`
- **THEN** `adapter.Capabilities()` returns nil

### Requirement: Plugin.Execute calling convention is documented
The `plugin.Plugin` interface documentation SHALL state: `input` is the primary data payload (rendered prompt or stdin content); `vars` is a string metadata map passed as environment variables or flags — it is NOT a general key-value store for structured data. This distinction SHALL be documented in the interface definition and in `CLIAdapter`.

#### Scenario: CLIAdapter passes input as stdin
- **WHEN** `CLIAdapter.Execute` is called with `input = "hello"`
- **THEN** the subprocess receives `"hello"` on its stdin

#### Scenario: CLIAdapter passes vars as env
- **WHEN** `CLIAdapter.Execute` is called with `vars = {"model": "sonnet"}`
- **THEN** the subprocess environment contains `ORCAI_MODEL=sonnet`
