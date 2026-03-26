# Complete Theme System Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make every visual element (panel content, all modals, jump window, borders, selection highlights) derive colors from the active theme bundle, and add 5 new themes.

**Architecture:** Extend `styles.ANSIPalette` with a `SelBG` field (bg ANSI sequence from `palette.border`). Add a shared `resolveModalColors` helper used by all three modals. Thread `ansiPalette()` through all panel builders, replacing hardcoded ANSI constants. The jump window (separate process) loads the persisted active theme via `themes.NewRegistry` at startup. Five new `theme.yaml` files are added under `internal/assets/themes/`.

**Tech Stack:** Go, BubbleTea, Lipgloss, ANSI escape sequences, YAML theme files.

---

### Task 1: Add `SelBG` to `ANSIPalette`

**Files:**
- Modify: `internal/styles/styles.go:126-153`

**Step 1: Add the field and populate it in `BundleANSI`**

In `ANSIPalette` struct add:
```go
SelBG string // 24-bit ANSI background sequence for selection highlight
```

In `BundleANSI`, add a `toBG` helper alongside `toFG` and populate the field:
```go
toBG := func(hex string) string {
    r, g, bv := hexToRGB(hex)
    return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, bv)
}
```
Then in the return:
```go
SelBG: toBG(p.Border), // border color as selection background
```

**Step 2: Update the fallback in `ansiPalette()` in switchboard.go**

In `internal/switchboard/switchboard.go:422-430`, add:
```go
SelBG: "\x1b[44m", // blue bg fallback
```

**Step 3: Build and verify**
```bash
go build ./...
```
Expected: clean build, no errors.

**Step 4: Commit**
```bash
git add internal/styles/styles.go internal/switchboard/switchboard.go
git commit -m "feat(theme): add SelBG to ANSIPalette for theme-driven selection highlights"
```

---

### Task 2: Add shared `resolveModalColors` helper

**Files:**
- Modify: `internal/switchboard/switchboard.go` (add helper near `ansiPalette()`)

**Step 1: Add the helper struct and function**

Add this block right after `ansiPalette()` (~line 433):

```go
// modalColors holds resolved lipgloss colors for modal overlays.
type modalColors struct {
    border  lipgloss.Color
    titleBG lipgloss.Color
    titleFG lipgloss.Color
    fg      lipgloss.Color
    accent  lipgloss.Color
    dim     lipgloss.Color
    error   lipgloss.Color
}

// resolveModalColors derives modal colors from the active bundle.
// Falls back to Dracula values when no bundle is active.
func (m Model) resolveModalColors() modalColors {
    // Dracula fallbacks
    c := modalColors{
        border:  lipgloss.Color("#bd93f9"),
        titleBG: lipgloss.Color("#bd93f9"),
        titleFG: lipgloss.Color("#282a36"),
        fg:      lipgloss.Color("#f8f8f2"),
        accent:  lipgloss.Color("#8be9fd"),
        dim:     lipgloss.Color("#6272a4"),
        error:   lipgloss.Color("#ff5555"),
    }
    b := m.activeBundle()
    if b == nil {
        return c
    }
    if v := b.ResolveRef(b.Modal.Border); v != "" {
        c.border = lipgloss.Color(v)
    }
    if v := b.ResolveRef(b.Modal.TitleBG); v != "" {
        c.titleBG = lipgloss.Color(v)
    }
    if v := b.ResolveRef(b.Modal.TitleFG); v != "" {
        c.titleFG = lipgloss.Color(v)
    }
    if v := b.Palette.FG; v != "" {
        c.fg = lipgloss.Color(v)
    }
    if v := b.Palette.Accent; v != "" {
        c.accent = lipgloss.Color(v)
    }
    if v := b.Palette.Dim; v != "" {
        c.dim = lipgloss.Color(v)
    }
    if v := b.Palette.Error; v != "" {
        c.error = lipgloss.Color(v)
    }
    return c
}
```

**Step 2: Build**
```bash
go build ./internal/switchboard/...
```
Expected: clean.

**Step 3: Commit**
```bash
git add internal/switchboard/switchboard.go
git commit -m "feat(theme): add resolveModalColors helper for shared modal color derivation"
```

---

### Task 3: Theme the quit confirmation modal

**Files:**
- Modify: `internal/switchboard/switchboard.go:1508-1566` (`viewQuitModalBox`)

**Step 1: Replace the existing color resolution block**

Replace the whole section starting `// Resolve modal colors from active bundle.` through the three `if` blocks, and the `rowStyle` func definition, with:

```go
mc := m.resolveModalColors()

headerStyle := lipgloss.NewStyle().
    Background(mc.titleBG).
    Foreground(mc.titleFG).
    Bold(true).
    Width(innerW).
    Padding(0, 1)

rowStyle := func(fg lipgloss.Color) lipgloss.Style {
    return lipgloss.NewStyle().Foreground(fg).Width(innerW).Padding(0, 1)
}
```

Replace hardcoded color args in `rows`:
- `styles.Fg` → `mc.fg`
- `styles.Cyan` → `mc.accent`
- `styles.Comment` → `mc.dim`

Replace `borderColor` in the final `BorderForeground(borderColor)` with `mc.border`.

**Step 2: Remove now-unused import of `styles` if nothing else uses it**

Check: `grep -n "styles\." internal/switchboard/switchboard.go` — remove import only if zero remaining uses.

**Step 3: Build**
```bash
go build ./internal/switchboard/...
```

**Step 4: Commit**
```bash
git add internal/switchboard/switchboard.go
git commit -m "feat(theme): quit modal uses active theme colors"
```

---

### Task 4: Theme the delete confirmation modal

**Files:**
- Modify: `internal/switchboard/switchboard.go:1570-1619` (`viewDeleteModalBox`)

**Step 1: Replace all hardcoded `styles.*` colors**

Replace the `headerStyle` block:
- `styles.Purple` → `mc.titleBG`
- `styles.Bg` → `mc.titleFG`

Replace `rowStyle` color args:
- `styles.Pink` → `mc.accent` (pipeline name highlight)
- `styles.Comment` → `mc.dim` (path display)
- `styles.Fg` → `mc.fg` (confirm line)
- `styles.Cyan` → `mc.accent` (yes key)
- `styles.Comment` → `mc.dim` (no key)

Replace `BorderForeground(styles.Purple)` → `BorderForeground(mc.border)`.

Add `mc := m.resolveModalColors()` at the top of the function.

**Step 2: Build**
```bash
go build ./internal/switchboard/...
```

**Step 3: Commit**
```bash
git add internal/switchboard/switchboard.go
git commit -m "feat(theme): delete modal uses active theme colors"
```

---

### Task 5: Theme the agent runner modal content

**Files:**
- Modify: `internal/switchboard/switchboard.go:1624-1780` (`viewAgentModalBox`)

The agent modal uses raw ANSI strings (not lipgloss) to be compatible with `boxRow`. We need to derive ANSI sequences from the palette.

**Step 1: Replace the border color resolution block with a full palette pull**

Replace lines 1630-1638 (just the border block) with:
```go
pal := m.ansiPalette()
// Derive border color ANSI sequence from active bundle.
modalBorderColor := aPur // fallback
if b := m.activeBundle(); b != nil {
    if border := b.ResolveRef(b.Modal.Border); border != "" {
        r, g, bv := hexToRGBFromStyles(border)
        modalBorderColor = fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, bv)
    }
}
```

**Step 2: Replace hardcoded constants in `hint()` and `sep`**

```go
hint := func(key, desc string) string {
    return pal.Accent + key + pal.Dim + " " + desc + aRst
}
sep := pal.Dim + " · " + aRst
```

**Step 3: Replace hardcoded constants in `sectionLabel()`** (local func at ~line 1765)

```go
sectionLabel := func(title string, active bool) string {
    if active {
        return pal.Accent + aBld + title + aRst
    }
    return pal.Dim + title + aRst
}
```

**Step 4: Replace row content colors throughout the modal**

For each occurrence replace:
- `aDim + "  no providers"` → `pal.Dim + "  no providers"`
- `aDim + "  select a provider first"` → `pal.Dim + "  select a provider first"`
- `aDim + "  no models"` → `pal.Dim + "  no models"`
- `sel := aBrC` (unfocused selection) → `sel := pal.Accent`
- `sel = aSelBg + aWht` (focused selection) → `sel = pal.SelBG + aWht`
- `content := aDim + "    " + aBC + label` → `content := pal.Dim + "    " + pal.Accent + label`
- `warn := aPnk + fmt.Sprintf("  ⚠ ...)` → `warn := pal.Error + fmt.Sprintf("  ⚠ ...)`

**Step 5: Build**
```bash
go build ./internal/switchboard/...
```

**Step 6: Commit**
```bash
git add internal/switchboard/switchboard.go
git commit -m "feat(theme): agent runner modal content uses active theme colors"
```

---

### Task 6: Theme panel content — pipelines list

**Files:**
- Modify: `internal/switchboard/switchboard.go:1819-1865` (`buildLauncherSection`)

**Step 1: Pull palette at top of function**

Add at the start of `buildLauncherSection`:
```go
pal := m.ansiPalette()
```

**Step 2: Replace hardcoded colors in content rows**

- `borderColor = aBrC` (focused) → keep as-is for border (it uses `pal.Border` already via `aBC`). Actually, update:
  - unfocused: `borderColor = pal.Border`
  - focused: `borderColor = pal.Accent`
- `countLine` with `aBrC` → `pal.Accent`
- `boxRow(aDim+"  no pipelines..."` → `boxRow(pal.Dim+"  no pipelines..."`
- Selected row: `aSelBg + aWht + "  " + displayName` → `pal.SelBG + aWht + "  " + displayName`
- Non-selected running: `aBrC + "  " + aBC + displayName` → `pal.Accent + "  " + pal.Accent + displayName`

**Step 3: Also fix `boxTop` label color** (line ~2138)

The `boxTop` helper has `aBrC` hardcoded for the title label. This is called from all panels. Change to accept an optional label color, or — simpler — use the border color itself (it's already themed). Actually just pass the label color in.

Change `boxTop` signature to:
```go
func boxTop(w int, title, borderColor, labelColor string) string {
```
And use `labelColor` instead of `aBrC`. Update all call sites to pass `pal.Accent` or `modalBorderColor` as appropriate.

**Step 4: Build**
```bash
go build ./internal/switchboard/...
```

**Step 5: Commit**
```bash
git add internal/switchboard/switchboard.go
git commit -m "feat(theme): pipelines list and boxTop label use active theme colors"
```

---

### Task 7: Theme panel content — agent runner list

**Files:**
- Modify: `internal/switchboard/switchboard.go:1866-1930` (`buildAgentSection`)

**Step 1: Pull palette at top of function**

```go
pal := m.ansiPalette()
```

**Step 2: Replace hardcoded colors**

- unfocused border: `aBC` → `pal.Border`
- focused border: `aBrC` → `pal.Accent`
- `aDim + "  no providers available"` → `pal.Dim + "  no providers available"`
- selected unfocused: `sel := aBrC` → `sel := pal.Accent`
- selected focused: `sel = aSelBg + aWht` → `sel = pal.SelBG + aWht`
- non-selected: `aDim + "    " + aBC + label` → `pal.Dim + "    " + pal.Accent + label`

**Step 3: Build**
```bash
go build ./internal/switchboard/...
```

**Step 4: Commit**
```bash
git add internal/switchboard/switchboard.go
git commit -m "feat(theme): agent runner list uses active theme colors"
```

---

### Task 8: Theme panel content — activity feed

**Files:**
- Modify: `internal/switchboard/switchboard.go:1931-2030` (`viewActivityFeed`)

**Step 1: Pull palette at top of function**

```go
pal := m.ansiPalette()
```

**Step 2: Replace hardcoded colors**

- unfocused border: `aBC` → `pal.Border`
- focused border: `aBrC` → `pal.Accent`
- `aDim + "  no activity yet"` → `pal.Dim + "  no activity yet"`
- Timestamp: `aDim, ts, aRst` → `pal.Dim, ts, aRst`
- Title: `aBrC+entry.title+aRst` → `pal.Accent+entry.title+aRst`
- Step status colors:
  - `col = aGrn` → `col = pal.Success`
  - `col = aRed` → `col = pal.Error`
  - `col = aDim` → `col = pal.Dim`
- Skip message: `aDim+skipMsg` → `pal.Dim+skipMsg`
- Output lines: `aDim+"    "+outLine` → `pal.Dim+"    "+outLine`

**Step 3: Also fix `statusIcon()` helper** (line ~2181) which returns `aPnk`/`aGrn`/`aRed`.

Since this is a standalone func without access to `pal`, change it to accept a `pal styles.ANSIPalette` parameter:
```go
func statusIcon(status FeedStatus, pal styles.ANSIPalette) (string, string) {
    switch status {
    case FeedRunning:
        return "▶ running", pal.Accent
    case FeedDone:
        return "✓ done", pal.Success
    case FeedFailed:
        return "✗ failed", pal.Error
    }
    return "? unknown", pal.Dim
}
```
Update call sites to pass `pal`.

**Step 4: Build**
```bash
go build ./internal/switchboard/...
```

**Step 5: Commit**
```bash
git add internal/switchboard/switchboard.go
git commit -m "feat(theme): activity feed uses active theme colors"
```

---

### Task 9: Theme panel content — signal board list

**Files:**
- Modify: `internal/switchboard/signal_board.go`

Signal board already partially themed (border via `borderColor`). Content rows still use hardcoded `aBrC`, `aGrn`, `aRed`, `aDim`.

**Step 1: Pull palette in `buildSignalBoard`**

Add after the border color block:
```go
pal := m.ansiPalette()
```

**Step 2: Replace hardcoded colors in row rendering**

LED indicator (lines ~74-86):
- `aBrC + "●"` → `pal.Accent + "●"`
- `aDim + "●"` → `pal.Dim + "●"`
- `aGrn + "●"` → `pal.Success + "●"`
- `aRed + "●"` → `pal.Error + "●"`

Status label (lines ~88-96):
- `aBrC + "running"` → `pal.Accent + "running"`
- `aGrn + "done"` → `pal.Success + "done"`
- `aRed + "failed"` → `pal.Error + "failed"`

**Step 3: Build**
```bash
go build ./internal/switchboard/...
```

**Step 4: Commit**
```bash
git add internal/switchboard/signal_board.go
git commit -m "feat(theme): signal board list uses active theme colors"
```

---

### Task 10: Theme the jump window

**Files:**
- Modify: `internal/jumpwindow/jumpwindow.go`

**Step 1: Add themes import**

Add to imports:
```go
"os"
"path/filepath"
"github.com/adam-stokes/orcai/internal/themes"
```

(`os` and `filepath` may already exist — check first.)

**Step 2: Add a `palette` field to the model**

```go
type model struct {
    windows  []window
    filtered []window
    selected int
    input    textinput.Model
    width    int
    height   int
    err      string
    pal      jumpPalette
}

type jumpPalette struct {
    border  lipgloss.Color
    titleBG lipgloss.Color
    titleFG lipgloss.Color
    fg      lipgloss.Color
    accent  lipgloss.Color
    selBG   lipgloss.Color
    selFG   lipgloss.Color
    dim     lipgloss.Color
}
```

**Step 3: Add `loadPalette()` function**

```go
func loadPalette() jumpPalette {
    // Dracula fallbacks
    p := jumpPalette{
        border:  lipgloss.Color("#bd93f9"),
        titleBG: lipgloss.Color("#bd93f9"),
        titleFG: lipgloss.Color("#282a36"),
        fg:      lipgloss.Color("#f8f8f2"),
        accent:  lipgloss.Color("#8be9fd"),
        selBG:   lipgloss.Color("#44475a"),
        selFG:   lipgloss.Color("#f8f8f2"),
        dim:     lipgloss.Color("#6272a4"),
    }
    home, err := os.UserHomeDir()
    if err != nil {
        return p
    }
    userThemesDir := filepath.Join(home, ".config", "orcai", "themes")
    reg, err := themes.NewRegistry(userThemesDir)
    if err != nil {
        return p
    }
    b := reg.Active()
    if b == nil {
        return p
    }
    if v := b.ResolveRef(b.Modal.Border); v != "" {
        p.border = lipgloss.Color(v)
        p.titleBG = lipgloss.Color(v)
    }
    if v := b.ResolveRef(b.Modal.TitleFG); v != "" {
        p.titleFG = lipgloss.Color(v)
    }
    if v := b.Palette.FG; v != "" {
        p.fg = lipgloss.Color(v)
        p.selFG = lipgloss.Color(v)
    }
    if v := b.Palette.Accent; v != "" {
        p.accent = lipgloss.Color(v)
    }
    if v := b.Palette.Border; v != "" {
        p.selBG = lipgloss.Color(v)
    }
    if v := b.Palette.Dim; v != "" {
        p.dim = lipgloss.Color(v)
    }
    return p
}
```

**Step 4: Update `newModel()` to load palette**

```go
func newModel() model {
    pal := loadPalette()
    ti := textinput.New()
    ti.Placeholder = "search windows..."
    ti.Focus()
    ti.PromptStyle = lipgloss.NewStyle().Foreground(pal.accent)
    ti.TextStyle = lipgloss.NewStyle().Foreground(pal.fg)
    ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(pal.dim)
    ti.Prompt = "> "
    m := model{input: ti, pal: pal}
    m.windows = listWindows()
    m.filtered = m.windows
    return m
}
```

**Step 5: Update `View()` to use `m.pal`**

Replace all `styles.*` with `m.pal.*`:
- `styles.Purple` → `m.pal.titleBG`
- `styles.Bg` → `m.pal.titleFG`
- `styles.Cyan` → `m.pal.accent`
- `styles.SelBg` → `m.pal.selBG`
- `styles.Pink` → `m.pal.accent`
- `styles.Fg` → `m.pal.fg`
- `styles.Comment` → `m.pal.dim`

After all replacements, remove the `styles` import if nothing else uses it.

**Step 6: Build**
```bash
go build ./internal/jumpwindow/...
```

**Step 7: Commit**
```bash
git add internal/jumpwindow/jumpwindow.go
git commit -m "feat(theme): jump window loads active theme from persisted registry"
```

---

### Task 11: Add 5 new themes

**Files:**
- Create: `internal/assets/themes/catppuccin-mocha/theme.yaml`
- Create: `internal/assets/themes/tokyo-night/theme.yaml`
- Create: `internal/assets/themes/rose-pine/theme.yaml`
- Create: `internal/assets/themes/solarized-dark/theme.yaml`
- Create: `internal/assets/themes/kanagawa/theme.yaml`

**Step 1: Create Catppuccin Mocha**

```yaml
name: catppuccin-mocha
display_name: "Catppuccin Mocha"
palette:
  bg:      "#1e1e2e"
  fg:      "#cdd6f4"
  accent:  "#cba6f7"
  dim:     "#6c7086"
  border:  "#313244"
  error:   "#f38ba8"
  success: "#a6e3a1"
borders:
  style: light
statusbar:
  format: " {session} · {provider} · {model} "
  bg: palette.bg
  fg: palette.accent
header_style:
  panels:
    pipelines:
      accent: "#cba6f7"
      text:   "#1e1e2e"
    agent_runner:
      accent: "#cba6f7"
      text:   "#1e1e2e"
    signal_board:
      accent: "#cba6f7"
      text:   "#1e1e2e"
    activity_feed:
      accent: "#cba6f7"
      text:   "#1e1e2e"
    modal:
      accent: "#cba6f7"
      text:   "#1e1e2e"
modal:
  bg:       palette.bg
  border:   palette.accent
  title_bg: palette.accent
  title_fg: palette.bg
```

**Step 2: Create Tokyo Night**

```yaml
name: tokyo-night
display_name: "Tokyo Night"
palette:
  bg:      "#1a1b26"
  fg:      "#c0caf5"
  accent:  "#7aa2f7"
  dim:     "#565f89"
  border:  "#292e42"
  error:   "#f7768e"
  success: "#9ece6a"
borders:
  style: light
statusbar:
  format: " {session} · {provider} · {model} "
  bg: palette.bg
  fg: palette.accent
header_style:
  panels:
    pipelines:
      accent: "#7aa2f7"
      text:   "#1a1b26"
    agent_runner:
      accent: "#7aa2f7"
      text:   "#1a1b26"
    signal_board:
      accent: "#7aa2f7"
      text:   "#1a1b26"
    activity_feed:
      accent: "#7aa2f7"
      text:   "#1a1b26"
    modal:
      accent: "#7aa2f7"
      text:   "#1a1b26"
modal:
  bg:       palette.bg
  border:   palette.accent
  title_bg: palette.accent
  title_fg: palette.bg
```

**Step 3: Create Rose Pine**

```yaml
name: rose-pine
display_name: "Rose Pine"
palette:
  bg:      "#191724"
  fg:      "#e0def4"
  accent:  "#c4a7e7"
  dim:     "#6e6a86"
  border:  "#26233a"
  error:   "#eb6f92"
  success: "#31748f"
borders:
  style: light
statusbar:
  format: " {session} · {provider} · {model} "
  bg: palette.bg
  fg: palette.accent
header_style:
  panels:
    pipelines:
      accent: "#c4a7e7"
      text:   "#191724"
    agent_runner:
      accent: "#c4a7e7"
      text:   "#191724"
    signal_board:
      accent: "#c4a7e7"
      text:   "#191724"
    activity_feed:
      accent: "#c4a7e7"
      text:   "#191724"
    modal:
      accent: "#c4a7e7"
      text:   "#191724"
modal:
  bg:       palette.bg
  border:   palette.accent
  title_bg: palette.accent
  title_fg: palette.bg
```

**Step 4: Create Solarized Dark**

```yaml
name: solarized-dark
display_name: "Solarized Dark"
palette:
  bg:      "#002b36"
  fg:      "#839496"
  accent:  "#268bd2"
  dim:     "#657b83"
  border:  "#073642"
  error:   "#dc322f"
  success: "#859900"
borders:
  style: light
statusbar:
  format: " {session} · {provider} · {model} "
  bg: palette.bg
  fg: palette.accent
header_style:
  panels:
    pipelines:
      accent: "#268bd2"
      text:   "#002b36"
    agent_runner:
      accent: "#268bd2"
      text:   "#002b36"
    signal_board:
      accent: "#268bd2"
      text:   "#002b36"
    activity_feed:
      accent: "#268bd2"
      text:   "#002b36"
    modal:
      accent: "#268bd2"
      text:   "#002b36"
modal:
  bg:       palette.bg
  border:   palette.accent
  title_bg: palette.accent
  title_fg: palette.bg
```

**Step 5: Create Kanagawa**

```yaml
name: kanagawa
display_name: "Kanagawa"
palette:
  bg:      "#1f1f28"
  fg:      "#dcd7ba"
  accent:  "#7e9cd8"
  dim:     "#727169"
  border:  "#2a2a37"
  error:   "#c34043"
  success: "#76946a"
borders:
  style: light
statusbar:
  format: " {session} · {provider} · {model} "
  bg: palette.bg
  fg: palette.accent
header_style:
  panels:
    pipelines:
      accent: "#7e9cd8"
      text:   "#1f1f28"
    agent_runner:
      accent: "#7e9cd8"
      text:   "#1f1f28"
    signal_board:
      accent: "#7e9cd8"
      text:   "#1f1f28"
    activity_feed:
      accent: "#7e9cd8"
      text:   "#1f1f28"
    modal:
      accent: "#7e9cd8"
      text:   "#1f1f28"
modal:
  bg:       palette.bg
  border:   palette.accent
  title_bg: palette.accent
  title_fg: palette.bg
```

**Step 6: Verify themes load by running tests**
```bash
go test ./internal/themes/... -v -run TestLoad
```
Expected: all bundled themes load including new ones.

**Step 7: Commit**
```bash
git add internal/assets/themes/
git commit -m "feat(theme): add Catppuccin Mocha, Tokyo Night, Rose Pine, Solarized Dark, Kanagawa themes"
```

---

### Task 12: Full build and test

**Step 1: Full build**
```bash
go build ./...
```
Expected: clean.

**Step 2: Run all tests**
```bash
go test ./... 2>&1
```
Expected: all pass.

**Step 3: Fix any test failures**

Common expected failure: `switchboard_test.go` may reference `statusIcon` with old signature. Update call sites.

**Step 4: Final commit if any test fixes needed**
```bash
git add -p
git commit -m "fix(theme): update tests for new theme-aware signatures"
```
