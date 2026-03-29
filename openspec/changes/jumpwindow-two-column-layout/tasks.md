## 1. Model Refactor

- [x] 1.1 Replace `selected int` with `selectedSysop int`, `selectedJob int`, and `focusCol int` (0=left, 1=right) in the `model` struct
- [x] 1.2 Update `newModel()` to initialize the new fields (all zero)
- [x] 1.3 Update `applyFilter()` to clamp `selectedJob` instead of the old unified `selected`

## 2. Input Handling

- [x] 2.1 Add `tab` case in `Update()` to toggle `focusCol` between 0 and 1
- [x] 2.2 Update `up`/`k` and `down`/`j` cases to move `selectedSysop` or `selectedJob` based on `focusCol`
- [x] 2.3 Update `enter` case to act on `m.sysop[m.selectedSysop]` when `focusCol==0`, or `m.filtered[m.selectedJob]` when `focusCol==1`
- [x] 2.4 Update `e` case to act on `m.filtered[m.selectedJob]` only when `focusCol==1`

## 3. Two-Column View

- [x] 3.1 Compute `colW = (w - 3) / 2` for each column; add narrow-terminal fallback (single-column) when `w < 40`
- [x] 3.2 Build a helper that renders a single cell string padded to `colW` runes with ANSI cursor/dim styling
- [x] 3.3 Render each body row as `│ <leftCell> │ <rightCell> │` using `panelrender.BoxRow` or direct ANSI strings, iterating `rowCount = max(len(m.sysop), len(m.filtered))`
- [x] 3.4 Add column header labels (`— sysop —` left, `— active jobs —` right) as the first body row
- [x] 3.5 Highlight the active column's selected item with accent style; dim the inactive column's selected item

## 4. Hint Bar

- [x] 4.1 Add `hint("tab", "switch col")` to the hint bar between `j/k nav` and `enter select`

## 5. Verification

- [x] 5.1 Build the binary (`make build` or `go build ./...`) with no errors
- [ ] 5.2 Manually open the jump window and confirm two columns render side by side
- [ ] 5.3 Confirm tab switches focus and j/k moves only within the focused column
- [ ] 5.4 Confirm search narrows active jobs only and sysop list is unaffected
- [ ] 5.5 Confirm modal height equals `max(len(sysop), len(activeJobs))` content rows, not their sum
- [x] 5.6 Run existing tests (`go test ./internal/jumpwindow/...`) and confirm no regressions
