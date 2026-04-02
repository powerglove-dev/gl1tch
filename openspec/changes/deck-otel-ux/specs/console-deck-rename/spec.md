## ADDED Requirements

### Requirement: Main TUI entry point is named "deck"
All references to "switchboard" within `internal/console/` SHALL be renamed to "deck". The file `switchboard.go` SHALL be renamed to `deck.go`. The `console` package name SHALL remain unchanged. The `Model` type name SHALL remain unchanged.

#### Scenario: File renamed
- **WHEN** the rename is applied
- **THEN** `internal/console/deck.go` exists and `internal/console/switchboard.go` does not

#### Scenario: No exported symbol named Switchboard remains
- **WHEN** the rename is applied
- **THEN** `go grep -r "Switchboard\|switchboard" internal/console/` returns zero results for identifiers (comments excluded)

#### Scenario: Build passes after rename
- **WHEN** the rename is applied to all callers in `cmd/` and `internal/console/`
- **THEN** `go build ./...` exits 0 with no errors

### Requirement: All callers updated
Any file outside `internal/console/` that imports or references the renamed identifiers SHALL be updated in the same change.

#### Scenario: cmd/ callers compile
- **WHEN** `cmd/` files reference the console package
- **THEN** they reference only `deck`-named identifiers and compile without error
