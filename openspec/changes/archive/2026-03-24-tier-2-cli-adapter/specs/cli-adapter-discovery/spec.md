## ADDED Requirements

### Requirement: Load wrappers from directory
`LoadWrappers(dir string)` SHALL scan `dir` for `*.yaml` files, attempt to load each as a sidecar, and return the successfully loaded plugins. Files that fail to parse SHALL be skipped and their errors collected. The function SHALL return an empty slice (not an error) when `dir` does not exist.

#### Scenario: Directory with valid sidecars
- **WHEN** `LoadWrappers` is called on a directory containing two valid sidecar YAML files
- **THEN** it returns a slice of two plugins with no error

#### Scenario: Directory does not exist
- **WHEN** `LoadWrappers` is called with a path that does not exist
- **THEN** it returns an empty slice and nil error

#### Scenario: Mixed valid and invalid sidecars
- **WHEN** a directory contains one valid and one malformed sidecar YAML
- **THEN** `LoadWrappers` returns the one valid plugin and a non-nil error slice describing the failure

#### Scenario: Non-YAML files ignored
- **WHEN** a directory contains `.yaml` and `.txt` files
- **THEN** only the `.yaml` files are processed

### Requirement: Manager loads and registers wrappers from directory
`Manager.LoadWrappersFromDir(dir string)` SHALL call `LoadWrappers(dir)` and register all returned plugins. It SHALL return a non-nil error only if `LoadWrappers` itself encounters a non-skippable failure (e.g., permission denied on the directory itself). Individual sidecar parse failures SHALL be logged but SHALL NOT prevent other wrappers from being registered.

#### Scenario: All valid sidecars registered
- **WHEN** `LoadWrappersFromDir` is called on a directory with two valid sidecars
- **THEN** both plugins are available via `Manager.Get` by their sidecar-declared names

#### Scenario: Empty directory registers nothing
- **WHEN** `LoadWrappersFromDir` is called on an empty directory
- **THEN** no plugins are registered and no error is returned
