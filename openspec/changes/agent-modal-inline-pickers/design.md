## Context

The agent modal in `internal/switchboard/switchboard.go` presents a multi-field form with distinct focus slots (0=provider/model, [1 skipped], 2=prompt, 3=use_brain, 4=cwd, 5=schedule). Two fields have awkward UX:

- **Saved Prompt**: uses `[`/`]` keys to cycle through prompts with no search — not accessible via tab focus
- **Working Directory**: Enter on focus slot 4 sets `dirPickerOpen = true`, which renders `DirPickerModel` via `panelrender.OverlayCenter`, covering the entire terminal

The existing modal package already provides `DirPickerModel` (async walk + fuzzy filter) and `AgentPickerModel` (inline list navigation). The `promptbuilder` package has a `Dropdown` component for inline list selection but lacks fuzzy search and uses hardcoded colors.

## Goals / Non-Goals

**Goals:**
- Add a `FuzzyPickerModel` to `internal/modal` for static-list inline fuzzy picking (Saved Prompt)
- Add `ViewInline` to `DirPickerModel` so it can render within parent rows (Working Directory)
- Give Saved Prompt a proper tab-stop (focus slot 1)
- Remove `[`/`]` cycling
- Replace the `OverlayCenter` dir picker for the agent context with an inline render

**Non-Goals:**
- Changing the dir picker overlay used in pipeline launch context (only the agent context changes)
- Modifying the `promptbuilder` Dropdown component
- Adding fuzzy search to any other modal field

## Decisions

### 1. New `FuzzyPickerModel` vs extending `Dropdown`

**Decision**: New `FuzzyPickerModel` in `internal/modal`.

`Dropdown` in `promptbuilder` uses hardcoded Dracula palette strings and lacks a filter input. Extending it risks breaking promptbuilder and the color model diverges from theme-aware ANSI palette used in the modal package. A new `FuzzyPickerModel` follows the same `Update`/`ViewInline` contract as `DirPickerModel` and `AgentPickerModel`.

### 2. `ViewInline` on `DirPickerModel` vs new component for CWD

**Decision**: Add `ViewInline` to `DirPickerModel`.

`DirPickerModel` already owns the async walk, fuzzy scoring, cursor state, and filter input for directories. Duplicating that into `FuzzyPickerModel` would require a separate async-feed mechanism. Adding `ViewInline` to `DirPickerModel` keeps the logic in one place and the change is additive.

### 3. Saved Prompt focus slot

**Decision**: Fill the currently-skipped slot 1.

The comment in switchboard already reserves slot 1. Inserting Saved Prompt at slot 1 makes the tab cycle natural: provider/model → saved prompt → prompt textarea → use brain → cwd → schedule → wrap.

### 4. `dirPickerOpen` flag scope

**Decision**: Keep `dirPickerOpen` + `dirPickerCtx` flags; change only the render path for `dirPickerCtx == "agent"`.

The pipeline launch flow still uses `OverlayCenter`. Reusing the same flag with a context check minimises diff and avoids introducing a separate `agentDirPickerOpen` field.

## Risks / Trade-offs

- **Fixed overhead constant**: `buildAgentModalRows` has a `fixedOverhead = 32` row budget constant. Adding inline picker rows when open expands the modal. The picker is capped at `maxVisible` rows (8 for saved prompts, 10 for dirs) so the overflow is bounded, but the constant will need updating or dynamic calculation.
  → Mitigation: make the overhead dynamic (count actual rows) rather than updating the constant.

- **Walk latency for inline CWD picker**: If the user opens CWD before the dir walk completes, `DirPickerModel` shows "scanning…". This is the same behavior as the overlay — no regression.

## Migration Plan

No data migration needed. The `[`/`]` key shortcuts are removed; users learn the new Enter-to-open behavior via the hint bar. No external API changes.
