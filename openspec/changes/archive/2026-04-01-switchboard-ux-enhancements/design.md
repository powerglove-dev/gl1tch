## Context

The Switchboard is a single BubbleTea `Model` in `internal/switchboard/switchboard.go`. It owns three left-column panels (Pipelines, Agent Runner, a future slot) and a right-column Activity Feed. All user input funnels through a single `Update` switch on `tea.KeyMsg`.

Current pain points:
- The Agent Runner form is a 3-step inline wizard (`formStep` 0/1/2) squeezed into a fixed-height box (~8 rows). There is no room to write a meaningful prompt.
- There are no keyboard shortcuts to create, edit, or delete pipeline YAML files without leaving the TUI entirely.
- No visual cue tells the user whether the Activity Feed has hidden content above or below the viewport.
- Focusing the Pipelines panel requires tabbing through all panels.

## Goals / Non-Goals

**Goals:**
- Add `n` / `e` / `d` keys on the pipelines panel for new/edit/delete pipeline actions
- Add `p` global shortcut to jump focus to the pipelines panel
- Replace the inline agent wizard with a centred overlay modal (`agentModal`)
- Show `↑` / `↓` scroll indicators in the activity feed header when off-screen content exists
- Remove trailing blank lines from the pipelines panel body

**Non-Goals:**
- Persisting pipeline edits back to the running session (reload handles that)
- In-TUI YAML editor; `$EDITOR` is sufficient
- Any changes to the signal board or tmux window management beyond what's needed for editor launch

## Decisions

### Decision: Overlay modal, not a new panel

**Choice**: Add an `agentModal` overlay rendered on top of all panels via a Z-order conditional in `View()`.

**Rationale**: BubbleTea renders a single string; overlays are implemented by writing over the base view. A centred box drawn with `lipgloss` (or raw ANSI) gives full terminal width/height for the prompt textarea and selection lists without restructuring the existing layout. An alternative — expanding the Agent Runner panel on focus — would require dynamic height negotiation across all panels.

**Alternative considered**: Dedicated "full-screen agent" mode swapping out the entire view. Rejected because it breaks the user's spatial model of the switchboard.

### Decision: Editor launch via `tmux new-window`

**Choice**: For `n` and `e`, execute `tmux new-window -d -n orcai-edit "$EDITOR <path>"`. The TUI continues running; focus shifts to the new window automatically via tmux. The pipeline reload on next `r` or on window close picks up changes.

**Rationale**: Consistent with the existing `tmux new-window` pattern used by the jump window and pipeline runner. Keeps the TUI process alive so state (feed, scroll, selection) is preserved.

**Alternative considered**: `tea.ExecProcess` to suspend the TUI and launch the editor in-place. Rejected because it clears the screen and loses feed context.

### Decision: Delete confirmation as an inline modal

**Choice**: A narrow centred overlay showing the pipeline name and `[y]es / [n]o` prompt. `y` deletes the file and refreshes the pipeline list; any other key dismisses without action.

**Rationale**: Lightweight; no new state machine needed beyond a `confirmDelete` bool + `pendingDelete string` on the model.

### Decision: Scroll indicators via header annotation

**Choice**: The activity feed box title changes from `ACTIVITY FEED` to `ACTIVITY FEED ↑` / `ACTIVITY FEED ↓` / `ACTIVITY FEED ↕` depending on scroll state, using the existing `visLen`-aware header rendering.

**Rationale**: No additional layout rows needed; uses the box title string which is already constructed dynamically. Consistent with BBS aesthetic of inline status.

### Decision: `p` as a global focus shortcut

**Choice**: `p` is handled at the top of the key switch (before panel-local handling) and sets `launcher.focused = true`, unfocusing all other panels.

**Rationale**: Mirrors the existing `a` (agent), `f` (feed), `s` (signals) shortcuts in the status bar. Simple single-character binding with no conflicts in the current key map.

## Risks / Trade-offs

- **Editor path** — `$EDITOR` may be unset on some systems → Mitigation: fall back to `vi` if `$EDITOR` is empty, same pattern used elsewhere in orcai.
- **File deletion is irreversible** — the confirmation modal reduces accidental deletion but there is no recycle bin → Mitigation: display full file path in confirmation prompt so the user can see what will be deleted.
- **Modal focus stealing** — when the agent modal is open, all key events must be consumed by the modal before reaching panel handlers → Mitigation: guard at the top of `Update`; if `agentModalOpen == true`, route all keys to modal handler and return early.
- **Overlay rendering alignment** — ANSI-rendered overlays must account for terminal width/height stored on the model; if the terminal is very narrow the overlay may wrap → Mitigation: enforce a minimum overlay width of 60 cols; if `m.width < 62` degrade gracefully to the inline form.

## Open Questions

- Should the agent modal remember the last-used provider/model between opens (persist on the model) or always reset? Likely persist — TBD during implementation.
- Should `n` for new pipeline pre-populate a template YAML, or open an empty file? Suggest template with sensible defaults.
