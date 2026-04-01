## Why

The current switchboard uses a two-column layout where the activity feed is stacked below the signal board, competing for vertical space in a single right column. Separating the activity feed into its own 25% column gives it room to breathe, keeps the agents panel visually anchored in the center, and places the agent runner directly beneath it where it is contextually related. Renaming the signal board to "Agents" and giving it a navigable grid view makes active agent state more scannable.

## What Changes

- Add a third column (25% width) on the right side dedicated to the activity feed panel.
- The center panel (formerly "Signal Board") is renamed to **Agents** and rendered as a 2-D grid of agent cards; navigate with `h`/`j`/`k`/`l`.
- The agent runner panel moves from the left column to below the Agents grid in the center column.
- The left column retains the pipeline launcher, inbox, and cron sections (no agent runner).
- Activity feed entries and step timelines are horizontally centered within the right column.
- Pipeline step output within the feed uses ANSI box-drawing characters (`├─`, `└─`, `│`) to form a timeline, with timestamps displayed in 12-hour format (e.g., `2:34 pm`).

## Capabilities

### New Capabilities

- `switchboard-three-column-layout`: Three-column switchboard layout with left (launcher/inbox/cron), center (agents grid + agent runner), and right (activity feed) columns.
- `agents-grid-panel`: Center panel displays active agents as a navigable grid; cursor moves with `h`/`j`/`k`/`l` keys; panel header label is "agents".
- `activity-feed-timeline-style`: Activity feed renders as a Facebook-style social timeline — each agent event is a centered card with a timestamp, status badge, and a one-line summary. Raw step output is suppressed; only structured event metadata (agent name, pipeline name, status transition) is shown.

### Modified Capabilities

- `feed-step-output`: Step rendering is simplified to structured event cards. Raw output lines are hidden by default. Each step shows only its name and status badge on the timeline connector (`├─` / `└─`). Timestamp format changes from `15:04:05` (24hr) to `3:04 pm` (12hr with lowercase am/pm).

## Impact

- `internal/switchboard/switchboard.go`: `View()`, `leftColWidth()`, `viewLeftColumn()` — column width calculations and layout assembly change significantly.
- `buildSignalBoard()` / new `buildAgentsGrid()` — signal board logic replaced or wrapped with grid renderer and renamed panel header.
- New center column render function stacks agents grid + agent runner vertically.
- `feedRawLines()` / `viewActivityFeed()` — centering logic, ANSI timeline connectors, and timestamp format.
- `buildAgentSection()` — moves from left column to center column.
- Existing tests that assert column widths, panel labels, or feed line positions will need updating.
