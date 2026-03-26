## Context

The ORCAI switchboard has a working theme registry (`internal/themes`) that persists the active theme to `~/.config/orcai/active_theme`. Five themes exist (Nord, ABS Dark, Gruvbox Dark, Dracula, Borland). However, the theme system only affects headers and borders — all panel body content and modals use hardcoded ANSI escape constants (`aBrC`, `aDim`, `aGrn`, `aRed`, `aSelBg`, `styles.Purple`, etc.) that are fixed to the Dracula palette.

The `styles.ANSIPalette` struct and `ansiPalette()` method exist but lack a `SelBG` (selection background) field. The three modal functions (`viewQuitModalBox`, `viewDeleteModalBox`, `viewAgentModalBox`) each resolve colors independently with duplicated logic. The jump window is a separate process with no theme awareness.

## Goals / Non-Goals

**Goals:**
- Every pixel of the switchboard UI responds to the active theme
- All three modals share a single color-resolution helper
- Jump window reads persisted theme at startup (no IPC needed)
- Five new YAML theme files added and auto-discovered by the registry
- `SelBG` field added to `ANSIPalette` so selection highlights are theme-driven

**Non-Goals:**
- Dynamic theme reload without restart (jump window reads theme once at startup)
- Per-panel theme overrides (all panels use the same palette)
- Light theme support (all new themes are dark)
- Tmux status bar dynamic recoloring (set at session creation time)

## Decisions

**D1: Add `SelBG` to `ANSIPalette` as a BG ANSI sequence derived from `palette.border`**

Rationale: The border color is always a slightly-elevated surface color in well-designed dark themes (e.g., Dracula `#44475a`, Nord `#3b4252`). Using it as the selection background gives a visually coherent highlight without needing a new palette field in every theme YAML. Alternative considered: add a `selection` field to `Palette` — rejected as over-engineering for 10 theme files.

**D2: Single `resolveModalColors()` helper returns a `modalColors` struct**

Rationale: The three modal functions currently duplicate the bundle-resolution pattern. Centralizing it ensures all modals stay in sync when themes change. Alternative: leave per-modal resolution — rejected because it's a maintenance liability.

**D3: Agent runner modal stays ANSI-based (not converted to lipgloss)**

Rationale: The agent modal uses `boxRow`/`boxBot` which produce raw ANSI strings for precise column-aligned layout. Converting to lipgloss would require rewriting the layout engine. Instead, we derive ANSI sequences from `ansiPalette()` and pass them into the existing string-building functions.

**D4: Jump window uses `themes.NewRegistry` at startup — Option A**

Rationale: The registry already persists the active theme name to `~/.config/orcai/active_theme` on every `SetActive()` call. The jump window can read this without any IPC. Alternative: pass `--theme` CLI flag from bootstrap — rejected because it adds bootstrap coupling and the file-based approach already works.

**D5: `boxTop` label color uses palette accent via an added parameter**

Rationale: The title label in panel headers (`boxTop`) hardcodes `aBrC` (bright cyan), overriding the theme. Since `boxTop` is a package-level function already accepting `borderColor`, adding a `labelColor string` parameter is minimal and keeps the function signature consistent. All call sites already have `pal.Accent` available.

## Risks / Trade-offs

- [Risk] Jump window startup slightly slower (loads theme registry) → Mitigation: registry load is a single file read + embed FS scan, sub-millisecond in practice
- [Risk] `boxTop` signature change breaks call sites → Mitigation: Go compiler catches all mismatches; plan includes updating all call sites in the same commit
- [Risk] New theme colors look bad at certain terminal color depths → Mitigation: all themes use 24-bit hex; terminals without 24-bit support will approximate
- [Trade-off] `SelBG` derived from `border` rather than a dedicated field — means themes with very dark borders might have low-contrast selection. Acceptable for v1; can add `selection` palette field later.

## Migration Plan

No database or config migrations. All changes are additive:
- New theme YAMLs are auto-discovered by the registry embed
- `SelBG` field is zero-value safe (existing serialized data unaffected)
- Rollback: revert commits, rebuild
