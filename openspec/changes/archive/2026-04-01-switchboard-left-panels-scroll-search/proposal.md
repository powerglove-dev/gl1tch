## Why

The ABBS Switchboard's left column panels (Pipelines, Agent Runner, Inbox, Cron Jobs) currently render without height constraints, causing them to overflow the terminal when lists grow long—especially the Inbox, which accumulates every signal. Users cannot scroll through these panels or filter items, making the Switchboard unusable when there is significant activity.

## What Changes

- Left column panels are capped to a proportional share of terminal height, distributing available rows evenly across all visible panels
- Inbox and Cron Jobs panels gain scrollable viewports with cursor-driven navigation (up/down arrow, `j`/`k`)
- Inbox and Cron Jobs panels gain fuzzy text search (activate with `/`, dismiss with `Esc`)
- Inbox items gain a "mark as read" action (`x` or `Enter`) that hides the item from the unread list
- Read items are persisted across sessions so they do not reappear on restart

## Capabilities

### New Capabilities

- `switchboard-panel-height-fit`: Left column panels are distributed evenly across available terminal height; no panel overflows the terminal
- `switchboard-panel-scroll-search`: Inbox and Cron Jobs panels support scrollable viewports and fuzzy text search
- `switchboard-inbox-mark-read`: Inbox items can be marked as read, hiding them from the list, with persistence across sessions

### Modified Capabilities

<!-- No existing spec-level requirements are changing -->

## Impact

- `internal/switchboard/` — panel layout, viewport sizing, and height distribution logic
- `internal/switchboard/inbox.go` (or equivalent) — scroll state, fuzzy filter, mark-as-read state
- `internal/switchboard/cron.go` (or equivalent) — scroll state, fuzzy filter
- Persistent read-state storage (likely `~/.config/orcai/` or local state file)
- Keybinding help bar at the bottom of the Switchboard (new keys: `/` search, `x` mark read)
