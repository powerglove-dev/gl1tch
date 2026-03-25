# Prompt Builder Sidebar Keybinding — Design

## Goal

Add `p` as a sidebar keybinding that opens the prompt builder TUI in a new tmux window, consistent with how other session windows are launched.

## Changes

### 1. `internal/promptbuilder/run.go` (new file)

Add a `Run()` function that bootstraps the prompt builder BubbleTea program — extracting the setup currently duplicated in `cmd/pipeline.go`.

### 2. `main.go`

Add `case "_promptbuilder":` to the internal dispatch switch, calling `promptbuilder.Run()`.

### 3. `internal/sidebar/sidebar.go`

- Add `case "p":` in `Update()` that executes:
  `tmux new-window -t orcai -n prompt-builder <self> _promptbuilder`
- Update footer constant from `n new  x kill  ↑↓ nav` to `n new  p build  x kill  ↑↓ nav`
