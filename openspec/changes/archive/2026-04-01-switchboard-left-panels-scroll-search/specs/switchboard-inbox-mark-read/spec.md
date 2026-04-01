## ADDED Requirements

### Requirement: Inbox items can be marked as read
While the Inbox panel is focused and an item is selected, pressing `x` SHALL mark the selected run as read. Marked items SHALL be hidden from the Inbox panel list immediately. The Inbox panel SHALL only display unread runs.

#### Scenario: Mark as read hides the item immediately
- **WHEN** the Inbox is focused, an item is selected, and the user presses `x`
- **THEN** the item is removed from the visible inbox list in the current session

#### Scenario: Next item is selected after mark-as-read
- **WHEN** the user marks an item as read and there are remaining items
- **THEN** the cursor advances to the next item (or the previous if the marked item was last)

#### Scenario: Empty inbox shows empty state after all items marked read
- **WHEN** the user marks the last unread item as read
- **THEN** the Inbox panel displays the "(empty)" placeholder

### Requirement: Read state persists across sessions
The Switchboard SHALL persist the set of read run IDs to `~/.config/orcai/inbox-read.json` on each mark-as-read action. On startup, the Switchboard SHALL load this file and exclude any run whose ID appears in the read set from the Inbox panel list.

#### Scenario: Marked items do not reappear after restart
- **WHEN** the user marks an item as read and then restarts the Switchboard
- **THEN** the previously marked item does not appear in the Inbox panel

#### Scenario: Missing read-state file is treated as empty
- **WHEN** `~/.config/orcai/inbox-read.json` does not exist
- **THEN** all runs are treated as unread and the inbox is populated normally; no error is emitted

#### Scenario: Corrupt read-state file is treated as empty
- **WHEN** `~/.config/orcai/inbox-read.json` contains invalid JSON
- **THEN** the file is ignored (treated as empty) and a warning is logged; the inbox is populated normally

### Requirement: Mark-as-read keybinding appears in the help bar
When the Inbox panel is focused, the Switchboard help bar SHALL display `x mark read` as an available action alongside existing panel keybindings.

#### Scenario: Help bar shows mark-read hint when inbox focused
- **WHEN** the Inbox panel has focus
- **THEN** the bottom help bar includes the text `x mark read`

#### Scenario: Help bar does not show mark-read hint when inbox not focused
- **WHEN** the Inbox panel does not have focus
- **THEN** the bottom help bar does not include `x mark read`
