## 1. FuzzyPickerModel — new component

- [x] 1.1 Create `internal/modal/fuzzypicker.go` with `FuzzyPickerModel` struct, `FuzzyPickerEvent` type (None/Confirmed/Cancelled), and `NewFuzzyPickerModel(maxVisible int)` constructor
- [x] 1.2 Extract `fuzzyScore` from `dirpicker.go` into a shared unexported helper in `internal/modal` (new `fuzzy.go` or top of `fuzzypicker.go`)
- [x] 1.3 Implement `Open(items []string)`, `Close()`, `IsOpen()`, `SelectedItem()`, `SetItems(items []string)` methods
- [x] 1.4 Implement `Update(msg tea.Msg) (FuzzyPickerModel, FuzzyPickerEvent, tea.Cmd)` with Up/Down/Enter/Esc/text-input handling
- [x] 1.5 Implement `ViewInline(w, maxVisible int, pal ANSIPalette) string` — bordered dropdown with filter row, item list (cursor highlighted), and hint line

## 2. DirPickerModel — inline render method

- [x] 2.1 Add `ViewInline(w int, pal ANSIPalette) string` to `DirPickerModel` that renders the filter input + directory list + hints without centering padding
- [x] 2.2 Update `dirpicker.go` to call shared `fuzzyScore` helper (removing local duplicate if extracted in 1.2)

## 3. Switchboard — Saved Prompt inline picker

- [x] 3.1 Add `savedPromptPicker modal.FuzzyPickerModel` field to switchboard `Model`
- [x] 3.2 Update `agentModalNextFocus` and `agentModalPrevFocus` to include slot 1 (0→1→2→3→4→5→0)
- [x] 3.3 On focus slot 1 + Enter: call `m.savedPromptPicker.Open(titles)` where titles includes "(none)" at index 0 followed by `agentPrompts` titles
- [x] 3.4 Route `tea.KeyMsg` to `savedPromptPicker.Update` when picker is open; on `FuzzyPickerConfirmed` set `agentPromptIdx` from selected index; on `FuzzyPickerCancelled` close
- [x] 3.5 Remove `[` / `]` key handlers from `handleAgentModal`
- [x] 3.6 Render `savedPromptPicker.ViewInline` rows inline in `buildAgentModalRows` immediately below the Saved Prompt header row when `savedPromptPicker.IsOpen()`

## 4. Switchboard — Working Directory inline picker

- [x] 4.1 In `buildAgentModalRows`, replace the "press enter to browse" hint row with inline `dirPicker.ViewInline` rows when `dirPickerOpen && dirPickerCtx == "agent"`
- [x] 4.2 In `View()`, guard the `OverlayCenter` dir picker render so it only fires when `dirPickerOpen && dirPickerCtx != "agent"`

## 5. Tests

- [x] 5.1 Unit tests for `FuzzyPickerModel`: Open/filter/confirm/cancel, empty list edge case
- [x] 5.2 Unit test for `DirPickerModel.ViewInline`: non-empty output, contains filter placeholder
- [x] 5.3 Switchboard model test: tab from slot 0 reaches slot 1; Enter on slot 1 opens savedPromptPicker
- [x] 5.4 Switchboard model test: `[`/`]` keys no longer change `agentPromptIdx`
- [x] 5.5 Switchboard model test: Enter on slot 4 sets `dirPickerOpen=true`, `dirPickerCtx="agent"` (no overlay in agent context)
