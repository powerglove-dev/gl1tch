## Requirements

### Requirement: Activity Feed step badges use theme palette colors
The Activity Feed SHALL render per-step status badges using colors from the active theme palette rather than hardcoded ANSI constants. The mapping SHALL be:

| Step status | Color source          |
|-------------|-----------------------|
| `running`   | `ANSIPalette.Warn`    |
| `done`      | `ANSIPalette.Success` |
| `failed`    | `ANSIPalette.Error`   |
| `pending`   | `ANSIPalette.Dim`     |

`ANSIPalette.Warn` SHALL be populated from the active theme bundle when one is loaded, and SHALL fall back to hardcoded ANSI yellow (`\x1b[33m`) when no bundle is active.

#### Scenario: Running step uses theme warn color
- **WHEN** a step is in "running" state and a theme bundle is active
- **THEN** the step badge is rendered using `ANSIPalette.Warn` derived from the bundle

#### Scenario: Fallback to ANSI yellow when no theme is loaded
- **WHEN** no theme bundle is active and a step is in "running" state
- **THEN** the step badge is rendered with the hardcoded ANSI yellow escape sequence

#### Scenario: Done and failed steps already use palette colors
- **WHEN** a step transitions to "done" or "failed"
- **THEN** `ANSIPalette.Success` or `ANSIPalette.Error` is used (no change from current behavior)
