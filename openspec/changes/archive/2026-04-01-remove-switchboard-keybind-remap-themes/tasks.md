## 1. Bootstrap chord table

- [x] 1.1 Remove the `^spc t` chord line (`bind-key -T orcai-chord t ...`) from `internal/bootstrap/bootstrap.go`
- [x] 1.2 Rename the `^spc m` chord line to use `t` instead of `m` in `internal/bootstrap/bootstrap.go`

## 2. Tmux status bar hints

- [x] 2.1 Remove the `^spc t switchboard` segment from the status bar string in `internal/themes/tmux.go`
- [x] 2.2 Rename `^spc m themes` → `^spc t themes` in `internal/themes/tmux.go`

## 3. Switchboard help modal

- [x] 3.1 Remove the `bind("^spc t", "go to switchboard")` line from `internal/switchboard/help_modal.go`
- [x] 3.2 Rename `bind("^spc m", "theme picker")` → `bind("^spc t", "theme picker")` in `internal/switchboard/help_modal.go`

## 4. Inline hint text

- [x] 4.1 Update `internal/modal/modal.go` — remove `^spc t` reference and rename `^spc m` → `^spc t` in any hint strings

## 5. Verification

- [x] 5.1 Build the project (`go build ./...`) and confirm no compile errors
- [x] 5.2 Grep for remaining `spc t` switchboard references and confirm none exist
- [x] 5.3 Grep for remaining `spc m` theme references and confirm none exist
