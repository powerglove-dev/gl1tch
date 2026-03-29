## ADDED Requirements

### Requirement: Run prompt against selected model inline
The prompt manager's right-bottom panel SHALL act as an inline test runner. Pressing `ctrl+r` in the editor panel SHALL invoke the selected model with the current prompt body and stream the response into the right-bottom panel. No new tmux window or agent session is created.

#### Scenario: Run triggered from editor
- **WHEN** the user presses `ctrl+r` in the editor panel
- **THEN** the right-bottom panel clears and begins displaying streamed output from the selected model

#### Scenario: Run uses current editor body
- **WHEN** the user triggers a run
- **THEN** the current (unsaved) body from the editor textarea is sent as the prompt, not the last-saved version

#### Scenario: Run uses selected model slug
- **WHEN** the user triggers a run
- **THEN** the model identified by the editor's model selector is used for the API call

### Requirement: Streamed response displayed in real time
The test runner panel SHALL display tokens as they are received from the model, using a scrollable viewport. A spinner SHALL be shown while streaming is in progress.

#### Scenario: Tokens appear as they stream
- **WHEN** the model is streaming a response
- **THEN** new tokens appear in the right-bottom panel without waiting for completion

#### Scenario: Spinner visible during streaming
- **WHEN** a run is in progress
- **THEN** a spinner indicator is rendered in the right-bottom panel header

#### Scenario: Spinner hidden after completion
- **WHEN** the model finishes streaming
- **THEN** the spinner is removed and the full response is shown

### Requirement: Cancel an in-progress run
Pressing `ctrl+c` or `esc` while a run is in progress SHALL cancel the streaming request and stop output.

#### Scenario: Cancel stops streaming
- **WHEN** the user presses `ctrl+c` during an active run
- **THEN** streaming stops and the panel shows the partial output received so far

### Requirement: Iterate on prompt after reviewing output
After a run completes, the editor panel SHALL remain editable. The user can modify the prompt body and trigger another run without navigating away.

#### Scenario: Editor stays editable after run
- **WHEN** a run completes
- **THEN** the editor panel is still active and the body text area accepts input

#### Scenario: Second run replaces previous output
- **WHEN** the user triggers a second run
- **THEN** the right-bottom panel clears and shows the new response

### Requirement: Completed run output is persisted with the prompt
When a test run completes successfully, the full response text SHALL be saved to the `last_response` column on the `prompts` table (added as an additive migration, `TEXT DEFAULT ''`). The store SHALL expose `SavePromptResponse(ctx, id int64, response string) error`. On next open of that prompt in the editor, the last response SHALL be pre-populated in the test runner panel.

#### Scenario: Response saved on run completion
- **WHEN** a test run finishes streaming
- **THEN** the complete response text is persisted to `prompts.last_response` for the current prompt ID

#### Scenario: Cancelled run does not overwrite saved response
- **WHEN** the user cancels an in-progress run
- **THEN** the previously saved `last_response` is not modified

#### Scenario: Last response shown on prompt open
- **WHEN** the user opens an existing prompt that has a saved response
- **THEN** the test runner panel pre-populates with the stored `last_response`

#### Scenario: Unsaved prompt run does not persist response
- **WHEN** the user runs a prompt that has not yet been saved (no ID)
- **THEN** the response is displayed in the panel but not persisted

### Requirement: Test runner output is scrollable
The right-bottom panel SHALL support scrolling through long responses using `↑`/`↓` or `j`/`k` when focused.

#### Scenario: Scroll down through output
- **WHEN** the response exceeds the panel height and the user presses `↓`
- **THEN** the viewport scrolls to reveal more output

#### Scenario: Scroll up through output
- **WHEN** the user has scrolled down and presses `↑`
- **THEN** the viewport scrolls back up
