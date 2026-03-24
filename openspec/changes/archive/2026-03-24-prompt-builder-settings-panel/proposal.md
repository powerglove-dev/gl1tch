## Why

The prompt builder sidebar currently only shows the steps list, leaving pipeline-level settings (name, description, tags) inaccessible from within the TUI — the pipeline name is display-only and cannot be edited after creation. Adding a settings panel to the sidebar gives users a single TUI surface to configure both step-level and pipeline-level properties without leaving the builder.

## What Changes

- Add a **Settings** section at the bottom of the left sidebar pane in the prompt builder TUI
- Make the pipeline **name** editable via a text input in the settings panel
- Add an optional **description** field to the pipeline model and settings panel
- Add optional **tags** (string slice) to the pipeline model and settings panel
- Introduce a new sidebar navigation mode that lets users tab between the steps list and the settings section
- Persist name, description, and tags to the saved `.pipeline.yaml` file

## Capabilities

### New Capabilities

- `prompt-builder-settings-panel`: A settings section within the prompt builder sidebar left pane that exposes pipeline-level fields (name, description, tags) as editable inputs, navigable via keyboard alongside the existing steps list.

### Modified Capabilities

<!-- No existing specs exist yet, so no delta specs are needed -->

## Impact

- `internal/promptbuilder/model.go` — add `description` and `tags` fields to `Model`; add setters
- `internal/promptbuilder/view.go` — add sidebar section rendering and settings panel navigation state
- `internal/promptbuilder/keys.go` — add any new keybindings for settings navigation
- `internal/pipeline/pipeline.go` — extend `Pipeline` struct with `Description` and `Tags` fields
- `internal/promptbuilder/save.go` — no structural change; fields will serialize automatically via yaml tags
