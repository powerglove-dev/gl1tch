## Context

ORCAI's theme system is already well-architected: `themes.Bundle` (YAML + optional ANSI splash), a file-based registry, and `internal/tuikit/theme_picker.go` with `ViewThemePicker` / `HandleThemePicker` as a reusable overlay. The current picker is a flat single-column list — all bundles, no grouping, no light themes in the library.

The jump window modal (`internal/jumpwindow/jumpwindow.go`) already demonstrates a 2-column layout with `focusCol`, `selectedSysop`, and `selectedJob` cursors using `lipgloss.JoinHorizontal`. We follow that pattern.

terminalcolors.com catalogs ~40 dark and ~15 light themes with per-terminal YAML/config exports. We source color values from there to populate new YAML bundles.

## Goals / Non-Goals

**Goals:**
- Add ~8 light theme bundles and ~8 additional dark theme bundles as YAML assets
- Redesign `ThemePicker` struct and its `View`/`Handle` functions to support dark/light tabs with a 2-column item grid
- Tab key switches between Dark / Light tabs; column focus mirrors jump window (h/l or left/right arrows)
- No changes to `themes.Bundle`, registry, bus events, or persistence — the theme name stored in `~/.config/orcai/active_theme` is stable

**Non-Goals:**
- Automatic theme mode sync with macOS appearance (system dark/light switching)
- Per-panel theme overrides
- Importing themes directly from terminalcolors.com at runtime (YAML bundles are compiled into the binary via `go:embed`)
- Gradient border or splash art for newly added themes in v1

## Decisions

### D1 — Extend `ThemePicker` struct, preserve function signatures

**Decision:** Add `Tab int` (0=dark, 1=light), `DarkCursor int`, `LightCursor int` to `ThemePicker`. Update `ViewThemePicker` and `HandleThemePicker` to accept a `tab` argument alongside the bundle slice, which is pre-split by caller into `darkBundles` / `lightBundles`.

**Why over alternatives:**
- Keeping existing function names means call sites in `switchboard/theme_picker.go` and `crontui` need only minor updates, no interface changes.
- Splitting bundle slices at the call site (not inside the picker) keeps the picker stateless about how bundles are classified — any bundle tagged `dark` or `light` works automatically.

### D2 — Dark/Light classification via bundle metadata field

**Decision:** Add an optional `mode` field to `theme.yaml` (`dark` | `light`, default `dark`). The registry exposes `BundlesByMode(mode string) []Bundle`. Untagged bundles are treated as `dark`.

**Why:** Simple string field, backward-compatible (missing = dark), no schema migration needed for existing themes.

### D3 — 2-column layout using lipgloss.JoinHorizontal, mirroring jump window

**Decision:** Each column renders a vertical list of themed swatches. Left column = odd-indexed themes, right = even-indexed (or split by half). Active column is highlighted. `h`/`l` or left/right arrows move column focus; `j`/`k` move within the active column.

**Why over a wider single column:** The jump window layout is proven in this codebase, familiar to users, and fits a ~80-char terminal well.

### D4 — Tab themes sourced from terminalcolors.com, hand-authored YAML

**Decision:** Color values are manually transcribed into YAML bundles. No runtime HTTP fetch. Bundles embedded with `go:embed`.

**Why:** Runtime fetching adds complexity, network dependency, and potential startup latency. The set of canonical themes is stable enough to ship as static assets.

## Risks / Trade-offs

- **ANSI width accounting in 2-column layout** → Mitigation: Use `panelrender.VisibleWidth()` (or equivalent) for swatch lines; test at 80 and 120 column widths.
- **Light theme palette rendering in a dark terminal** → Mitigation: Light themes still use the active bundle's UI chrome (borders, picker background) — only the swatch preview uses the light bundle's colors. No forced terminal background change.
- **`mode` field additions to existing YAMLs** → Mitigation: Backward-compatible default; existing themes work without the field.

## Migration Plan

1. Add `mode` field to all existing `theme.yaml` files (all `dark`).
2. Add new light and dark bundle YAML files under `internal/assets/themes/`.
3. Update `themes.Bundle` struct and YAML loader to read `Mode`.
4. Add `BundlesByMode` to registry.
5. Rewrite `ViewThemePicker` and `HandleThemePicker` for tabbed 2-column layout.
6. Update call sites in `switchboard/theme_picker.go` and `crontui/`.
7. No rollback needed — the theme name file format is unchanged.

### D5 — Real-time theme preview on cursor move via busd

**Decision:** When the cursor moves to a new theme entry (j/k/h/l navigation), the picker SHALL immediately publish a `theme.changed` busd event and apply tmux colors for that bundle — identical to what `ApplyThemeSelection` does today. On `esc` or cancel, the picker publishes a `theme.changed` event for the previously active bundle to restore it.

**Why:** The busd socket and tmux apply are already present in `ApplyThemeSelection`. Splitting it into `PreviewTheme(bundle)` (no persistence) and `ApplyThemeSelection(bundle)` (with persistence) keeps the architecture clean. Real-time preview is a significant UX improvement with minimal extra code: one extra bus publish per navigation key.

**Why not defer:** The infrastructure (busd socket, tmux apply) is already in place. The only missing piece is not persisting the active_theme file on cursor moves, which is a one-line change.

## Open Questions

- Should the tab label show a count, e.g. `Dark (12)` vs just `Dark`? (cosmetic, can decide during implementation)
- Do we include any splash ANSI art for light themes, or defer to v2?
