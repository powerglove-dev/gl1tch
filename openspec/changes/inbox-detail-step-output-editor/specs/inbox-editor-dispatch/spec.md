## ADDED Requirements

### Requirement: Inbox detail provides an open-in-editor keybinding
The inbox detail view SHALL expose an `e` keybinding that opens the current run's full content as a plain-text temp file in `$EDITOR` (falling back to `vi` if `$EDITOR` is unset). The TUI SHALL suspend via `tea.ExecProcess` while the editor is running and resume when the editor exits.

#### Scenario: e opens editor with run content
- **WHEN** the user presses `e` while the inbox detail is open
- **THEN** the TUI suspends and `$EDITOR` opens a temp file containing the run's content (ANSI stripped)

#### Scenario: EDITOR env var respected
- **WHEN** `$EDITOR` is set to a valid executable
- **THEN** that executable is launched with the temp file as its argument

#### Scenario: Fallback to vi when EDITOR unset
- **WHEN** `$EDITOR` is not set
- **THEN** `vi` is used as the editor

#### Scenario: Flash message when no editor available
- **WHEN** neither `$EDITOR` nor `vi` can be found on PATH
- **THEN** the TUI shows a flash message "set $EDITOR to use this feature" and does not suspend

### Requirement: Clipboard content is injected into agent runner on editor exit
When the editor exits, the system clipboard SHALL be read. If the clipboard content differs from the pre-launch snapshot AND is non-empty, that content SHALL be injected into the agent runner prompt and the agent modal SHALL be opened with the prompt focused.

#### Scenario: Yanked text injected into agent runner
- **WHEN** the user yanks text to the system clipboard in the editor and then quits
- **THEN** on return to orcai the agent modal opens with the yanked text pre-filled in the prompt

#### Scenario: Clipboard snapshot prevents stale injection
- **WHEN** the clipboard contains text from before the editor was launched and the user does not yank anything new
- **THEN** the pre-launch clipboard text is NOT injected into the agent runner

#### Scenario: Temp file fallback when clipboard unchanged
- **WHEN** the editor exits and the clipboard is unchanged or empty
- **THEN** the temp file content (as saved by the user) is injected into the agent runner prompt if it differs from the original content written to the file

#### Scenario: No injection when nothing changed
- **WHEN** the editor exits and neither the clipboard nor the temp file was changed
- **THEN** the agent modal is NOT opened and the inbox detail is restored as-is

### Requirement: Temp file is cleaned up after editor exit
The temp file written for the editor session SHALL be deleted after the clipboard/file content is read on editor exit, regardless of whether injection occurred.

#### Scenario: Temp file deleted on editor exit
- **WHEN** the editor exits (normally or via forced quit)
- **THEN** the temp file is deleted from the filesystem

### Requirement: e keybinding appears in inbox detail hint bar
The `e` keybinding SHALL appear in the inbox detail hint bar with the description `editor`.

#### Scenario: Hint bar shows editor key
- **WHEN** the inbox detail view is rendered
- **THEN** the hint bar includes `e  editor`
