## 1. Action Map

- [x] 1.1 Add `resize-pane-left` → `{"resize-pane", "-L", "5"}` to `actionMap` in `internal/keybindings/keybindings.go`
- [x] 1.2 Add `resize-pane-right` → `{"resize-pane", "-R", "5"}` to `actionMap`
- [x] 1.3 Add `resize-pane-up` → `{"resize-pane", "-U", "5"}` to `actionMap`
- [x] 1.4 Add `resize-pane-down` → `{"resize-pane", "-D", "5"}` to `actionMap`

## 2. Tests

- [x] 2.1 Add test cases in `internal/keybindings/keybindings_test.go` asserting each resize action resolves to the correct tmux args

## 3. Config Init Defaults

- [x] 3.1 Locate where `orcai config init` writes the default `keybindings.yaml` content
- [x] 3.2 Add four example resize bindings (e.g. `M-Left`, `M-Right`, `M-Up`, `M-Down`) to the default template
