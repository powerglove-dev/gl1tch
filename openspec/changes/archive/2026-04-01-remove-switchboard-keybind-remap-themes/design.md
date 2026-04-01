## Context

ORCAI's chord system (`^spc` prefix) is configured in two places: the tmux bootstrap (chord table bindings and status bar hints) and the in-app TUI help modals. Both must be kept in sync manually. Currently `^spc t` launches the switchboard and `^spc m` opens the theme picker — a layout that will become incorrect once the switchboard is reachable via the jump-to-window modal.

## Goals / Non-Goals

**Goals:**
- Remove all traces of `^spc t` as a switchboard shortcut (tmux binding, status bar hint, help modal, inline hint text)
- Remap `^spc m` → `^spc t` for the theme picker in every location where `^spc m` currently appears

**Non-Goals:**
- Adding the switchboard to the jump-to-window modal (separate change)
- Changing how the theme picker itself works
- Modifying the `T` in-app key that triggers the picker inside the switchboard/crontui

## Decisions

**Single-pass search-and-replace across four files.**
All affected strings are literals with no shared constants. The safest approach is a targeted edit per file rather than introducing a new shared constant — the chord strings appear in mixed contexts (tmux shell strings, Go string literals, rendered ANSI text) that don't share a common abstraction layer.

Files to change:
| File | Change |
|------|--------|
| `internal/bootstrap/bootstrap.go` | Remove chord `t` line; rename chord `m` → `t` |
| `internal/themes/tmux.go` | Remove `^spc t switchboard` segment; rename `^spc m` → `^spc t` |
| `internal/switchboard/help_modal.go` | Remove `bind("^spc t", ...)` line; rename `^spc m` → `^spc t` |
| `internal/modal/modal.go` | Update any inline hint text referencing these chords |

## Risks / Trade-offs

- **Stale worktrees**: Worktrees in `.worktrees/` contain copies of these files that won't be updated. Those are ephemeral agent branches and are not a concern.
- **No runtime validation**: Tmux keybindings are only applied at `orcai _reload` or session start. After this change, users must reload their config for the new binding to take effect. → No mitigation needed; this is normal orcai behavior.
