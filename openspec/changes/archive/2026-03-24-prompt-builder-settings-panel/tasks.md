## 1. Pipeline Model

- [ ] 1.1 Add `Description string` (yaml:"description,omitempty") and `Tags []string` (yaml:"tags,omitempty") fields to `pipeline.Pipeline` in `internal/pipeline/pipeline.go`
- [ ] 1.2 Add tests for round-trip YAML serialization of description and tags (empty fields omitted, non-empty fields preserved)

## 2. Promptbuilder Model

- [ ] 2.1 Add `description string`, `tags []string`, and `sidebarMode int` fields to `Model` in `internal/promptbuilder/model.go`
- [ ] 2.2 Add `Description()`/`SetDescription()` and `Tags()`/`SetTags()` accessors
- [ ] 2.3 Update `ToPipeline()` to include `description` and `tags` in the returned `Pipeline`
- [ ] 2.4 Add `SidebarMode()` accessor and constants `SidebarModeSteps = 0`, `SidebarModeSettings = 1`
- [ ] 2.5 Add `ToggleSidebarMode()` method that switches between steps and settings modes
- [ ] 2.6 Add tests for new Model fields, accessors, and `ToPipeline()` propagation

## 3. Settings Inputs

- [ ] 3.1 Add three `textinput.Model` fields to `Model`: `nameInput`, `descriptionInput`, `tagsInput` (import `github.com/charmbracelet/bubbles/textinput`)
- [ ] 3.2 Initialize the inputs in `New()` with appropriate placeholders (`"pipeline name"`, `"description"`, `"tags (comma-separated)"`)
- [ ] 3.3 Add `settingsFocus int` field to track which settings input is focused (0=name, 1=description, 2=tags)
- [ ] 3.4 Add `AdvanceSettingsFocus(forward bool)` method that cycles focus through the three inputs (wrapping)

## 4. Keys

- [ ] 4.1 Add `Settings key.Binding` to `keyMap` in `internal/promptbuilder/keys.go` bound to `","` with help text `", settings"`

## 5. Update Logic

- [ ] 5.1 In `internal/promptbuilder/model.go` (or wherever `Update` lives), handle the `,` key: call `ToggleSidebarMode()`
- [ ] 5.2 When `sidebarMode == SidebarModeSettings`, route `Tab` and `Shift+Tab` to `AdvanceSettingsFocus()` instead of right-pane field cycling
- [ ] 5.3 Propagate text input `Msg` events to the focused settings input when in settings mode
- [ ] 5.4 On each settings input change, sync the value back to `model.description` / `model.tags` (split+trim tags on comma)
- [ ] 5.5 When switching to settings mode, pre-populate `nameInput` with `model.name`

## 6. View

- [ ] 6.1 In `internal/promptbuilder/view.go`, add a `renderSettingsPanel()` function that renders the three labeled text inputs
- [ ] 6.2 In the left sidebar render path, switch on `sidebarMode`: render steps list for `SidebarModeSteps`, render settings panel for `SidebarModeSettings`
- [ ] 6.3 Add mode indicator to the sidebar header (e.g., `STEPS` / `SETTINGS` label)

## 7. Save

- [ ] 7.1 Verify `save.go` requires no changes — `ToPipeline()` now includes description/tags and yaml marshaling handles omitempty automatically
- [ ] 7.2 Add save round-trip test: build a model with description+tags, save to temp file, load pipeline, assert fields match

## 8. Integration

- [ ] 8.1 Run all existing promptbuilder and pipeline tests: `go test ./internal/promptbuilder/... ./internal/pipeline/...`
- [ ] 8.2 Fix any test failures caused by new fields or changed behavior
- [ ] 8.3 Commit with message: `feat(promptbuilder): add settings panel with description and tags`
