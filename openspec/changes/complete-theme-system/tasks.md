## 1. Extend ANSIPalette with SelBG

- [x] 1.1 Add `SelBG string` field to `styles.ANSIPalette` struct in `internal/styles/styles.go`
- [x] 1.2 Add `toBG` helper in `BundleANSI` and populate `SelBG: toBG(p.Border)`
- [x] 1.3 Add `SelBG: "\x1b[44m"` to the fallback in `ansiPalette()` in `internal/switchboard/switchboard.go`
- [x] 1.4 Build `./...` and verify no errors

## 2. Add resolveModalColors helper

- [x] 2.1 Add `modalColors` struct and `resolveModalColors() modalColors` method to `internal/switchboard/switchboard.go` after `ansiPalette()`
- [x] 2.2 Build `./internal/switchboard/...` and verify no errors

## 3. Theme quit and delete modals

- [x] 3.1 Replace color resolution in `viewQuitModalBox` to use `m.resolveModalColors()` and replace all `styles.*` with `mc.*` fields
- [x] 3.2 Replace all hardcoded `styles.*` in `viewDeleteModalBox` with `m.resolveModalColors()` fields
- [x] 3.3 Build and verify no errors

## 4. Theme agent runner modal content

- [x] 4.1 Replace border-only resolution in `viewAgentModalBox` with full `pal := m.ansiPalette()` pull
- [x] 4.2 Replace `hint()` and `sep` with `pal.Accent` / `pal.Dim`
- [x] 4.3 Replace `sectionLabel()` with `pal.Accent` / `pal.Dim`
- [x] 4.4 Replace all `aDim`, `aBrC`, `aBC`, `aSelBg`, `aPnk` in modal row content with `pal.*` fields
- [x] 4.5 Build and verify no errors

## 5. Theme boxTop label color

- [x] 5.1 Add `labelColor string` parameter to `boxTop` in `internal/switchboard/switchboard.go`
- [x] 5.2 Update all `boxTop(...)` call sites to pass `pal.Accent` (or `modalBorderColor` for agent modal)
- [x] 5.3 Build and verify no errors

## 6. Theme pipelines list panel

- [x] 6.1 Add `pal := m.ansiPalette()` at top of `buildLauncherSection`
- [x] 6.2 Replace unfocused border `aBC` → `pal.Border`, focused border `aBrC` → `pal.Accent`
- [x] 6.3 Replace `aBrC` in `countLine`, `aSelBg+aWht` in selected row, `aBrC`/`aBC` in non-selected row, `aDim` in empty state with `pal.*` equivalents
- [x] 6.4 Build and verify no errors

## 7. Theme agent runner list panel

- [x] 7.1 Add `pal := m.ansiPalette()` at top of `buildAgentSection`
- [x] 7.2 Replace `aBC`/`aBrC` border colors, `aDim`/`aBrC`/`aBC`/`aSelBg` row colors with `pal.*` equivalents
- [x] 7.3 Build and verify no errors

## 8. Theme activity feed panel

- [x] 8.1 Add `pal := m.ansiPalette()` at top of `viewActivityFeed`
- [x] 8.2 Replace `aBC`/`aBrC` border colors with `pal.Border`/`pal.Accent`
- [x] 8.3 Replace `aDim`, `aBrC`, `aGrn`, `aRed` in entry/output rows with `pal.*`
- [x] 8.4 Change `statusIcon` signature to accept `pal styles.ANSIPalette` and update call sites
- [x] 8.5 Build and verify no errors

## 9. Theme signal board panel

- [x] 9.1 Add `pal := m.ansiPalette()` in `buildSignalBoard` in `internal/switchboard/signal_board.go`
- [x] 9.2 Replace LED colors (`aBrC`, `aDim`, `aGrn`, `aRed`) with `pal.Accent`, `pal.Dim`, `pal.Success`, `pal.Error`
- [x] 9.3 Replace status label colors with `pal.Accent`, `pal.Success`, `pal.Error`
- [x] 9.4 Build and verify no errors

## 10. Theme jump window

- [x] 10.1 Add `themes` import and define `jumpPalette` struct in `internal/jumpwindow/jumpwindow.go`
- [x] 10.2 Implement `loadPalette()` function that creates `themes.NewRegistry` and reads `reg.Active()`
- [x] 10.3 Add `pal jumpPalette` field to `model` struct and call `loadPalette()` in `newModel()`
- [x] 10.4 Update `newModel()` input styles to use `pal.*` instead of `styles.*`
- [x] 10.5 Update `View()` to use `m.pal.*` for all styles, remove `styles` import
- [x] 10.6 Build `./internal/jumpwindow/...` and verify no errors

## 11. Add new theme bundles

- [x] 11.1 Create `internal/assets/themes/catppuccin-mocha/theme.yaml` with Catppuccin Mocha palette
- [x] 11.2 Create `internal/assets/themes/tokyo-night/theme.yaml` with Tokyo Night palette
- [x] 11.3 Create `internal/assets/themes/rose-pine/theme.yaml` with Rose Pine palette
- [x] 11.4 Create `internal/assets/themes/solarized-dark/theme.yaml` with Solarized Dark palette
- [x] 11.5 Create `internal/assets/themes/kanagawa/theme.yaml` with Kanagawa palette
- [x] 11.6 Run `go test ./internal/themes/... -v -run TestLoad` and verify all 10 themes load

## 12. Full verification

- [x] 12.1 Run `go build ./...` — clean build
- [x] 12.2 Run `go test ./...` — all tests pass
- [x] 12.3 Fix any test failures (likely `statusIcon` call sites or `boxTop` signature changes in tests)
- [x] 12.4 Commit all changes with message `feat(theme): complete theme system — all panels, modals, jump window, 5 new themes`

## 13. Title visibility fix

- [x] 13.1 Add `hexToBGSeq` helper to `internal/switchboard/ansi_render.go`
- [x] 13.2 Update `DynamicHeader` to render title line with accent BG + text FG so titles are always readable
- [x] 13.3 Build and verify no errors
