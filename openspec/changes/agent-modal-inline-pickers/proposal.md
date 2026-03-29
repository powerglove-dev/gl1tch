## Why

The agent modal's Saved Prompt and Working Directory fields use clunky interaction patterns — `[`/`]` key cycling for prompts and a full-screen overlay for directory browsing — that break focus and obscure context. Inline pickers that drop down within the modal give users immediate visual feedback and fuzzy search without leaving the modal surface.

## What Changes

- **New**: `FuzzyPickerModel` component in `internal/modal` — a reusable inline fuzzy picker for static item lists
- **New**: `ViewInline` method on `DirPickerModel` — renders the directory picker in-place within parent rows instead of as a centered overlay
- **Modified**: Saved Prompt field gains a proper focus slot (slot 1) and opens a fuzzy-filterable inline dropdown on Enter; `[`/`]` cycling removed
- **Modified**: Working Directory field (focus slot 4) renders the dir picker inline within the agent modal instead of launching a full-screen overlay

## Capabilities

### New Capabilities

- `agent-modal-inline-pickers`: Inline fuzzy-filter pickers for Saved Prompt and Working Directory fields within the agent modal

### Modified Capabilities

<!-- No existing spec-level requirements are changing -->

## Impact

- `internal/modal/` — new `fuzzypicker.go` file; `dirpicker.go` gains `ViewInline` method
- `internal/switchboard/switchboard.go` — agent modal state, focus cycling, key handling, and `buildAgentModalRows` view
- No API or external dependency changes
