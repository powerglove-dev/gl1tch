# BBS-Style Sysop Panel Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Redesign the sysop panel View() to match a classic BBS aesthetic (thin cyan borders, three-column layout) and convert RunToggle() from a split-pane to a display-popup.

**Architecture:** All model/update logic in `internal/sidebar/sidebar.go` stays unchanged. Only the view layer (ANSI constants, box helpers, `View()`) and `RunToggle()` are replaced. The three-column layout renders nodes, selected node details, and activity log side-by-side using a `visLen`/`padToVis` approach to correctly pad ANSI-colored strings.

**Tech Stack:** Go, BubbleTea, raw ANSI escape codes (no lipgloss in view layer)

---

### Task 1: Update tests to match new design

**Files:**
- Modify: `internal/sidebar/sidebar_test.go`

The existing tests `TestViewContainsHeader` and `TestViewContainsActiveAccent` assert on the old double-line box style and old `NODE 01` label format. Update them to the new design before touching the implementation so they become the failing spec.

**Step 1: Update TestViewContainsHeader**

Replace the `╔` border check with `┌` and the `enter focus` footer check stays (but the footer is now inside a box row):

```go
func TestViewContainsHeader(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{})
	view := m.View()
	if !strings.Contains(view, "┌") {
		t.Errorf("View() missing outer border '┌':\n%s", view)
	}
	if !strings.Contains(view, "SYSOP MONITOR") {
		t.Errorf("View() missing 'SYSOP MONITOR' header:\n%s", view)
	}
	if !strings.Contains(view, "enter") {
		t.Errorf("View() does not contain footer hint 'enter':\n%s", view)
	}
}
```

**Step 2: Update TestViewContainsActiveAccent**

The new design uses `[1]` not `NODE 01`. Selection background stays `\x1b[48;5;235m` (updated from 236):

```go
func TestViewContainsActiveAccent(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{
		{Index: 1, Name: "claude-1"},
		{Index: 2, Name: "opencode-2"},
	})
	view := m.View()
	// cursor=0 → first node row should have the selection background escape.
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(line, "claude-1") && strings.Contains(line, "[1]") {
			if !strings.Contains(line, "\x1b[48;5;235m") {
				t.Errorf("active node line missing selection background: %q", line)
			}
			return
		}
	}
	t.Errorf("View() does not contain a '[1] claude-1' selected line:\n%s", view)
}
```

**Step 3: Run tests to confirm they now fail**

```bash
go test ./internal/sidebar/... -run TestViewContainsHeader -v
go test ./internal/sidebar/... -run TestViewContainsActiveAccent -v
```

Expected: both FAIL (old View() still uses `╔` and `NODE 01`).

**Step 4: Commit updated tests**

```bash
git add internal/sidebar/sidebar_test.go
git commit -m "test(sidebar): update view tests for BBS redesign"
```

---

### Task 2: Replace RunToggle — popup instead of split-pane

**Files:**
- Modify: `internal/sidebar/sidebar.go`

**Step 1: Delete the panel visibility helpers**

Remove these three functions entirely:
- `panelVisiblePath() (string, error)`
- `isPanelVisible() bool`
- `setPanelVisible(visible bool)`

Also remove the `path/filepath` import if it becomes unused after this step (it's used by `resolveSysopBin` so it stays).

**Step 2: Replace RunToggle**

```go
// RunToggle opens the sysop panel as a tmux popup.
func RunToggle() {
	bin := resolveSysopBin()
	exec.Command("tmux", "display-popup", "-E", "-w", "120", "-h", "40", bin).Run() //nolint:errcheck
}
```

**Step 3: Remove now-unused imports**

`os` may still be needed by `resolveSysopBin` (uses `os.Executable`). Keep it. Remove any import that becomes unused.

**Step 4: Build to confirm no compile errors**

```bash
go build ./internal/sidebar/...
```

Expected: compiles cleanly.

**Step 5: Commit**

```bash
git add internal/sidebar/sidebar.go
git commit -m "feat(sysop): RunToggle uses display-popup, remove split-pane visibility state"
```

---

### Task 3: Replace ANSI color constants and add view helpers

**Files:**
- Modify: `internal/sidebar/sidebar.go`

**Step 1: Replace the ANSI constant block**

Find and replace the existing `const (aTeal ... aReset)` block:

```go
// ── ANSI palette — ABS/Dracula BBS aesthetic ───────────────────────────────
const (
	aBC    = "\x1b[36m"       // cyan — borders and general text
	aBrC   = "\x1b[96m"       // bright cyan — section headers and key labels
	aDim   = "\x1b[38;5;66m"  // dim teal — secondary text, [IDLE] badge
	aWht   = "\x1b[97m"       // bright white — selected row text
	aSelBg = "\x1b[48;5;235m" // dark background — cursor row highlight
	aGrn   = "\x1b[92m"       // bright green — [BUSY] badge
	aYlw   = "\x1b[93m"       // bright yellow — [WAIT] badge
	aReset = "\x1b[0m"
)
```

**Step 2: Add the visLen and padToVis helpers**

These allow correct padding of ANSI-coloured strings for the column layout.

```go
// visLen returns the number of visible (non-ANSI-escape) runes in s.
func visLen(s string) int {
	n, esc := 0, false
	for _, r := range s {
		if r == '\x1b' {
			esc = true
			continue
		}
		if esc {
			if r == 'm' {
				esc = false
			}
			continue
		}
		n++
	}
	return n
}

// padToVis right-pads s with spaces until its visible length equals w.
func padToVis(s string, w int) string {
	vl := visLen(s)
	if vl >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vl)
}
```

**Step 3: Add the box-drawing helpers**

Replace the old `borderTop`, `borderMid`, `borderThin`, `borderBot`, `borderRow`, `innerPad` helpers with:

```go
// boxTop renders the top edge of a box. If title is non-empty it is inset
// into the border: ┌─── Title ───┐
func boxTop(w int, title string) string {
	if title == "" {
		return aBC + "┌" + strings.Repeat("─", w-2) + "┐" + aReset
	}
	label := " " + title + " "
	dashes := w - 2 - visLen(label)
	if dashes < 0 {
		dashes = 0
	}
	left := dashes / 2
	right := dashes - left
	return aBC + "┌" + strings.Repeat("─", left) + aBrC + label + aBC + strings.Repeat("─", right) + "┐" + aReset
}

// boxMid renders a horizontal rule inside a box (├─┤), with optional title.
func boxMid(w int, title string) string {
	if title == "" {
		return aBC + "├" + strings.Repeat("─", w-2) + "┤" + aReset
	}
	label := " " + title + " "
	dashes := w - 2 - visLen(label)
	if dashes < 0 {
		dashes = 0
	}
	left := dashes / 2
	right := dashes - left
	return aBC + "├" + strings.Repeat("─", left) + aBrC + label + aBC + strings.Repeat("─", right) + "┤" + aReset
}

// boxBot renders the bottom edge of a box.
func boxBot(w int) string {
	return aBC + "└" + strings.Repeat("─", w-2) + "┘" + aReset
}

// boxRow renders one content row inside a box, padded to w.
// content is an ANSI string; contentVis is its visible length.
func boxRow(content string, contentVis, w int) string {
	inner := w - 2
	pad := inner - contentVis
	if pad < 0 {
		pad = 0
	}
	return aBC + "│" + aReset + content + strings.Repeat(" ", pad) + aBC + "│" + aReset
}

// boxEmpty renders a blank row inside a box of width w.
func boxEmpty(w int) string {
	return aBC + "│" + strings.Repeat(" ", w-2) + "│" + aReset
}

// sideBySide merges three equal-height column slices into a single slice,
// padding each column to colW visible characters with gap spaces between.
func sideBySide(left, mid, right []string, colW, gap int) []string {
	h := max(max(len(left), len(mid)), len(right))
	sp := strings.Repeat(" ", gap)
	rows := make([]string, h)
	for i := range h {
		l, m, r := "", "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(mid) {
			m = mid[i]
		}
		if i < len(right) {
			r = right[i]
		}
		rows[i] = padToVis(l, colW) + sp + padToVis(m, colW) + sp + r
	}
	return rows
}
```

**Step 4: Build**

```bash
go build ./internal/sidebar/...
```

Expected: clean (old `View()` still uses the old helpers — they're now replaced, so you may get compile errors until View() is updated in Task 4. If needed, leave the old `View()` body temporarily and fix in Task 4).

**Step 5: Commit**

```bash
git add internal/sidebar/sidebar.go
git commit -m "refactor(sidebar): BBS ANSI palette, visLen/padToVis, new box helpers"
```

---

### Task 4: Implement the three-column BBS View()

**Files:**
- Modify: `internal/sidebar/sidebar.go`

This is the main task. Replace the entire `View()` function and add the three column-builder methods.

**Step 1: Add buildNodesColumn**

```go
// buildNodesColumn renders the "Active Nodes" column as a slice of lines of width w.
func (m Model) buildNodesColumn(w int) []string {
	inner := w - 2
	rows := []string{boxTop(w, "Active Nodes")}

	if len(m.windows) == 0 {
		nodesLine := aDim + "  no active nodes" + aReset
		rows = append(rows, boxRow(nodesLine, 18, w))
	}

	for i, win := range m.windows {
		_, hasTel := m.sessions[win.Name]
		st := m.sessions[win.Name]

		var badge, badgeCol string
		switch {
		case !hasTel:
			badge, badgeCol = "[WAIT]", aYlw
		case st.Status == "streaming":
			badge, badgeCol = "[BUSY]", aGrn
		default:
			badge, badgeCol = "[IDLE]", aDim
		}

		keyLabel := fmt.Sprintf("[%d]", i+1)
		// Truncate window name to fit: inner - len(keyLabel) - 1 space - 1 space - len(badge)
		maxName := inner - len(keyLabel) - 2 - len(badge)
		if maxName < 1 {
			maxName = 1
		}
		name := win.Name
		if len(name) > maxName {
			name = name[:maxName-1] + "…"
		}
		dotCount := inner - len(keyLabel) - 1 - len(name) - 1 - len(badge)
		if dotCount < 1 {
			dotCount = 1
		}

		var content string
		contentVis := len(keyLabel) + 1 + len(name) + dotCount + len(badge)
		if i == m.cursor {
			content = aSelBg + aBrC + keyLabel + " " + aWht + name +
				aDim + strings.Repeat(".", dotCount) +
				badgeCol + badge + aReset
			rows = append(rows, aBC+"│"+content+strings.Repeat(" ", max(inner-contentVis, 0))+aBC+"│"+aReset)
		} else {
			content = aBrC + keyLabel + " " + aBC + name +
				aDim + strings.Repeat(".", dotCount) +
				badgeCol + badge + aReset
			rows = append(rows, boxRow(content, contentVis, w))
		}
	}

	rows = append(rows, boxBot(w))
	return rows
}
```

**Step 2: Add buildDetailsColumn**

```go
// buildDetailsColumn renders the "Node Details" column for the cursor node.
func (m Model) buildDetailsColumn(w int) []string {
	rows := []string{boxTop(w, "Node Details")}

	if len(m.windows) == 0 || m.cursor >= len(m.windows) {
		rows = append(rows, boxRow(aDim+"  no node selected"+aReset, 18, w))
		rows = append(rows, boxBot(w))
		return rows
	}

	win := m.windows[m.cursor]
	st, hasTel := m.sessions[win.Name]

	field := func(label, value string) string {
		dots := 12 - len(label)
		if dots < 1 {
			dots = 1
		}
		line := "  " + aBrC + label + " " + aDim + strings.Repeat(".", dots) + " " + aBC + value + aReset
		return boxRow(line, 2+len(label)+1+dots+1+len(value), w)
	}

	rows = append(rows, field("Window", win.Name))

	if hasTel {
		rows = append(rows, field("Provider", st.Provider))
		var statusStr string
		switch st.Status {
		case "streaming":
			statusStr = aGrn + "streaming" + aReset
		default:
			statusStr = aDim + st.Status + aReset
		}
		statusLine := "  " + aBrC + "Status" + " " + aDim + strings.Repeat(".", 7) + " " + statusStr + aReset
		rows = append(rows, boxRow(statusLine, 2+6+1+7+1+len(st.Status), w))

		if st.InputTokens > 0 {
			tokens := fmt.Sprintf("%dk↑ / %d↓", st.InputTokens/1000, st.OutputTokens)
			rows = append(rows, field("Tokens", tokens))
			cost := fmt.Sprintf("$%.4f", st.CostUSD)
			rows = append(rows, field("Cost", cost))
		}
	} else {
		rows = append(rows, boxRow(aDim+"  no telemetry yet"+aReset, 18, w))
	}

	rows = append(rows, boxBot(w))
	return rows
}
```

**Step 3: Add buildLogColumn**

```go
// buildLogColumn renders the "Activity Log" column.
func (m Model) buildLogColumn(w int) []string {
	rows := []string{boxTop(w, "Activity Log")}

	if len(m.log) == 0 {
		rows = append(rows, boxRow(aDim+"  no activity"+aReset, 13, w))
	}

	for _, entry := range m.log {
		var line string
		nodeLabel := fmt.Sprintf("NODE%02d", entry.Node)
		if entry.Event == "done" && entry.CostUSD > 0 {
			line = fmt.Sprintf("  %s  %s  done  $%.3f",
				entry.At.Format("15:04"), nodeLabel, entry.CostUSD)
		} else {
			line = fmt.Sprintf("  %s  %s  %s",
				entry.At.Format("15:04"), nodeLabel, entry.Event)
		}
		rows = append(rows, boxRow(aDim+line+aReset, len(line), w))
	}

	rows = append(rows, boxBot(w))
	return rows
}
```

**Step 4: Replace View()**

```go
func (m Model) View() string {
	w := m.width
	if w <= 0 {
		w = 120
	}

	gap := 2
	colW := (w - gap*2) / 3

	var lines []string

	// ── Title bar ─────────────────────────────────────────────────────────────
	title := "ABS · SYSOP MONITOR"
	titleVis := len(title)
	pad := (w - 2 - titleVis) / 2
	centred := strings.Repeat(" ", pad) + aBrC + title + aReset
	lines = append(lines,
		boxTop(w, ""),
		boxRow(centred, pad+titleVis, w),
		boxBot(w),
		"",
	)

	// ── Three columns ──────────────────────────────────────────────────────────
	left := m.buildNodesColumn(colW)
	mid := m.buildDetailsColumn(colW)
	right := m.buildLogColumn(colW)
	lines = append(lines, sideBySide(left, mid, right, colW, gap)...)
	lines = append(lines, "")

	// ── Actions bar ────────────────────────────────────────────────────────────
	actionsContent := "  " + aBrC + "[enter]" + aBC + " focus    " +
		aBrC + "[x]" + aBC + " kill    " +
		aBrC + "[↑↓]" + aBC + " navigate    " +
		aBrC + "[ctrl+c]" + aBC + " quit" + aReset
	actionsVis := 2 + 7 + 10 + 3 + 9 + 4 + 12 + 8 + 5
	lines = append(lines,
		boxTop(w, ""),
		boxRow(actionsContent, actionsVis, w),
		boxBot(w),
	)

	// ── Status strip ───────────────────────────────────────────────────────────
	nodeCount := len(m.windows)
	var totalCost float64
	var activeProvider string
	var anyStreaming bool
	for _, st := range m.sessions {
		totalCost += st.CostUSD
		if st.Provider != "" && activeProvider == "" {
			activeProvider = st.Provider
		}
		if st.Status == "streaming" {
			anyStreaming = true
		}
	}
	sep := aDim + "  │  " + aReset
	statusParts := []string{
		fmt.Sprintf("%sNODES: %d ACTIVE%s", aBrC, nodeCount, aReset),
	}
	if anyStreaming {
		statusParts = append(statusParts, aGrn+"STREAMING"+aReset)
	}
	if activeProvider != "" {
		statusParts = append(statusParts, aBC+strings.ToUpper(activeProvider)+aReset)
	}
	if totalCost > 0 {
		statusParts = append(statusParts, aBC+fmt.Sprintf("$%.3f TOTAL", totalCost)+aReset)
	}
	statusParts = append(statusParts, aDim+time.Now().Format("15:04")+aReset)
	lines = append(lines, strings.Join(statusParts, sep))

	return strings.Join(lines, "\n")
}
```

**Step 5: Build**

```bash
go build ./internal/sidebar/...
```

Expected: clean.

**Step 6: Run all sidebar tests**

```bash
go test ./internal/sidebar/... -v
```

Expected: all PASS. If `TestViewContainsActiveAccent` fails, check that `[1]` + window name appear on the same line with `\x1b[48;5;235m`.

**Step 7: Commit**

```bash
git add internal/sidebar/sidebar.go
git commit -m "feat(sidebar): BBS three-column View() — nodes, details, activity log"
```

---

### Task 5: Full build, all tests, install

**Step 1: Build everything**

```bash
go build ./...
```

Expected: clean.

**Step 2: Run all tests**

```bash
go test ./...
```

Expected: all PASS.

**Step 3: Install binaries**

```bash
go install ./cmd/orcai-sysop/... && echo "ok"
```

**Step 4: Commit and push**

```bash
git push
```

---

## Notes

- `visLen` strips ANSI by scanning for `\x1b` … `m` sequences — this is sufficient for our palette (no multi-param sequences with non-`m` terminators).
- Column widths are computed as `(totalWidth - 2*gap) / 3`; at 120 cols that gives 38 per column.
- The activity log column height is driven by `m.log` (max 12 entries). All three columns are padded to the same height by `sideBySide`.
- `RunToggle` no longer manages state — every `^spc t` opens a fresh popup. This matches the picker pattern exactly.
