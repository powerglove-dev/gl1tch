## 1. Dropdown Component

- [x] 1.1 Create `internal/promptbuilder/dropdown.go` — define `Dropdown` struct with `items []string`, `separators map[int]bool`, `selected int`, `open bool`, `scrollOffset int`
- [x] 1.2 Implement `Dropdown.Open()`, `Dropdown.Close()`, `Dropdown.Toggle()` methods
- [x] 1.3 Implement `Dropdown.Update(msg tea.KeyMsg) (changed bool)` — handle Up/Down (skip separators), Enter (close+apply), Escape (close+revert)
- [x] 1.4 Implement `Dropdown.View(label string, width int, p palette) string` — renders closed state as `label: [value]` and open state with bordered overlay using Dracula palette and box-drawing chars
- [x] 1.5 Write unit tests for dropdown cursor navigation (separator skipping, scroll clamping, open/close/revert)

## 2. Multi-Select Dropdown

- [x] 2.1 Create `internal/promptbuilder/multiselect.go` — `MultiSelect` struct extending Dropdown with `checked map[int]bool`
- [x] 2.2 Implement Space to toggle checked state, Enter to confirm, Escape to revert
- [x] 2.3 Implement `MultiSelect.Selected() []string` returning checked item values
- [x] 2.4 Render checked items with `✓` prefix in pink, unchecked in dim

## 3. BubbleModel Rewrite — Core Group

- [x] 3.1 Replace `activeField int` + `pluginIndex`/`modelIndex` integers in `BubbleModel` with `activeGroup int` (0=Core,1=Exec,2=Advanced), `activeFieldInGroup int`, and three `Dropdown` fields: `executorDD`, `modelDD Dropdown`
- [x] 3.2 Populate `executorDD` from `picker.BuildProviders()` providers + separator + builtin list at init time
- [x] 3.3 On executor selection change: if builtin selected → disable and clear `modelDD`; if provider → repopulate `modelDD` with provider's models
- [x] 3.4 Update `Update()` to route key events to the focused dropdown when open, otherwise to group/field navigation
- [x] 3.5 Update `View()` Core group rendering: executor dropdown, model dropdown (disabled when builtin), prompt text input

## 4. BubbleModel Rewrite — Execution Group

- [x] 4.1 Add `needsDD MultiSelect` to `BubbleModel`; populate from sibling step IDs on step selection change
- [x] 4.2 Add text inputs for `retryMaxAttempts`, `retryInterval`, `retryOn` (dropdown: always/on_failure), `forEachInput`, `onFailureDD Dropdown` (sibling step IDs)
- [x] 4.3 Wire Execution group fields into `Update()` and `View()`
- [x] 4.4 On save, write non-zero Execution fields to the step's `Retry`, `Needs`, `ForEach`, `OnFailure` fields

## 5. BubbleModel Rewrite — Advanced Group

- [x] 5.1 Add `conditionIfInput`, `conditionThenDD`, `conditionElseDD` (sibling step dropdowns) to `BubbleModel`
- [x] 5.2 Add `publishToInput textinput.Model`
- [x] 5.3 Implement generic args key/value list: `argsRows []argsRow`, `argsSelected int`; `+` adds row, `d` deletes, `Enter` edits inline
- [x] 5.4 Implement builtin-specific arg field pre-population: switch on executor name, pre-key rows for assert/log/sleep/http_get
- [x] 5.5 Wire Advanced group fields into `Update()` and `View()`
- [x] 5.6 On save, write `Condition`, `PublishTo`, and `Args` map to step

## 6. Left Pane Step List

- [x] 6.1 Update step list rendering to show `◆ [N] id (executor)` for provider steps and `⚙ [N] id (executor)` for builtin steps
- [x] 6.2 Highlight selected step in pink; dim all others

## 7. Save Integration

- [x] 7.1 Audit `save.go` — confirm it writes `executor` + `args` (not `plugin` + `vars`) and update if needed
- [x] 7.2 Ensure builtin steps save `executor: builtin.<type>` and `args:` map with no `plugin:` or `vars:` keys

## 8. Keys & Help

- [x] 8.1 Update `keys.go` — add group Tab/Shift+Tab bindings, remove old Left/Right cycling bindings for plugin/model
- [x] 8.2 Update the help overlay (`?`) to reflect new group navigation and dropdown interactions
