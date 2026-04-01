package console

import (
	"fmt"
	"time"

	"github.com/sahilm/fuzzy"

	"github.com/8op-org/gl1tch/internal/panelrender"
)

// SignalBoard tracks state for the SIGNAL BOARD panel.
type SignalBoard struct {
	selectedIdx  int
	activeFilter string // "all", "running", "done", "failed"
	blinkOn      bool
	query        string // fuzzy search query (active when searching == true)
	searching    bool   // true when / search mode is active
	scrollOffset int    // first visible row index into filtered+fuzzy results
}

var filterCycle = []string{"running", "all", "done", "failed", "archived"}

// cycleFilter advances the activeFilter to the next in the cycle.
func (sb *SignalBoard) cycleFilter() {
	for i, f := range filterCycle {
		if f == sb.activeFilter {
			sb.activeFilter = filterCycle[(i+1)%len(filterCycle)]
			return
		}
	}
	sb.activeFilter = "all"
}

// feedSource implements fuzzy.Source for feed entries, matching on title.
type feedSource []feedEntry

func (s feedSource) Len() int            { return len(s) }
func (s feedSource) String(i int) string { return s[i].title }

// fuzzyFeed applies fuzzy filtering by query over entries.
// Returns entries unchanged when query is empty.
func fuzzyFeed(query string, entries []feedEntry) []feedEntry {
	if query == "" {
		return entries
	}
	matches := fuzzy.FindFrom(query, feedSource(entries))
	out := make([]feedEntry, len(matches))
	for i, m := range matches {
		out[i] = entries[m.Index]
	}
	return out
}

// clearSearch resets the fuzzy search state.
func (sb *SignalBoard) clearSearch() {
	sb.searching = false
	sb.query = ""
	sb.selectedIdx = 0
	sb.scrollOffset = 0
}

// clampScroll adjusts scrollOffset so that selectedIdx stays within the
// visible window of visibleRows rows.
func (sb *SignalBoard) clampScroll(visibleRows int) {
	if visibleRows < 1 {
		visibleRows = 1
	}
	if sb.selectedIdx < sb.scrollOffset {
		sb.scrollOffset = sb.selectedIdx
	}
	if sb.selectedIdx >= sb.scrollOffset+visibleRows {
		sb.scrollOffset = sb.selectedIdx - visibleRows + 1
	}
}

// sbFixedBodyRows is the fixed number of visible body rows in the signal board.
// At 3 rows per entry, this fits 4 entries with no remainder.
const sbFixedBodyRows = 12

// humanDuration formats a duration as a human-readable string.
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

// signalBoardPanelHeight returns the total rendered height (in lines) of the
// signal board panel for the given contentH. Used consistently by View(),
// clampFeedScroll(), and signalBoardVisibleRows() to prevent height drift.
func (m Model) signalBoardPanelHeight(contentH int) int {
	// Count rendered header lines (same logic as buildSignalBoard).
	headerLines := 1 // boxTop
	if PanelHeader(m.activeBundle(), "signal_board", 80, "", "") != nil {
		headerLines = 3 // sprite
	}
	headerLines++ // filter line (always shown)
	if m.signalBoard.searching || m.signalBoard.query != "" {
		headerLines++ // search input line
	}
	// +2: hint footer row + boxBot row
	sbH := headerLines + sbFixedBodyRows + 2
	if sbH > contentH-3 {
		sbH = max(contentH-3, 8)
	}
	return sbH
}

// signalBoardVisibleRows computes the number of visible entries the signal board
// can show at the current terminal height. Each entry occupies 3 body rows.
func (m Model) signalBoardVisibleRows() int {
	h := m.height
	if h <= 0 {
		h = 40
	}
	contentH := max(h-m.headerHeight()-1, 5)
	// Mirror header line count from buildSignalBoard.
	headerRows := 1 // boxTop fallback
	if PanelHeader(m.activeBundle(), "signal_board", 80, "", "") != nil {
		headerRows = 3 // sprite(3)
	}
	headerRows++ // filter line (always shown)
	if m.signalBoard.searching || m.signalBoard.query != "" {
		headerRows++ // search input line
	}
	// -2: hint footer + boxBot
	bodyH := contentH - headerRows - 2
	if bodyH < 1 {
		bodyH = 1
	}
	visibleEntries := bodyH / 3
	if visibleEntries < 1 {
		visibleEntries = 1
	}
	return visibleEntries
}

// buildSignalBoard renders the SIGNAL BOARD panel.
// Returns a slice of rendered lines (including box borders).
func (m Model) buildSignalBoard(height, width int) []string {
	filter := m.signalBoard.activeFilter
	if filter == "" {
		filter = "all"
	}

	pal := m.ansiPalette()
	borderColor := pal.Border
	if m.signalBoardFocused {
		borderColor = pal.Accent
	}

	// Pre-compute filtered results so we can show count in the header.
	preFiltered := fuzzyFeed(m.signalBoard.query, m.filteredFeed())
	total := len(preFiltered)
	sel := m.signalBoard.selectedIdx + 1
	if total == 0 {
		sel = 0
	}

	var lines []string
	if sprite := PanelHeader(m.activeBundle(), "signal_board", width, borderColor, pal.Accent); sprite != nil {
		lines = append(lines, sprite...)
	} else {
		lines = append(lines, boxTop(width, RenderHeader("signal_board"), borderColor, pal.Accent))
	}
	// Always show the current filter with scroll indicator.
	scrollIndicator := ""
	if total > 0 {
		scrollIndicator = fmt.Sprintf("  %s%d/%d%s", pal.Dim, sel, total, aRst)
	}
	filterLine := fmt.Sprintf("  filter: %s%s%s%s", pal.Accent, filter, aRst, scrollIndicator)
	lines = append(lines, boxRow(filterLine, width, borderColor))

	// Search input line (only visible when / search mode is active, or query is non-empty).
	if m.signalBoard.searching || m.signalBoard.query != "" {
		cursor := ""
		if m.signalBoard.searching {
			cursor = pal.Accent + "█" + aRst
		}
		searchLine := fmt.Sprintf("  %s/ %s%s%s%s", pal.Dim, aRst, pal.FG, m.signalBoard.query, cursor+aRst)
		lines = append(lines, boxRow(searchLine, width, borderColor))
	}

	filtered := preFiltered

	// Cap to available body rows (-1 for boxBot, -1 for always-present hint footer).
	bodyH := height - len(lines) - 2 // -1 for boxBot, -1 for hint footer
	if bodyH <= 0 {
		bodyH = 1
	}
	// Each entry occupies 3 rows.
	visibleEntries := bodyH / 3
	if visibleEntries < 1 {
		visibleEntries = 1
	}

	if len(filtered) == 0 {
		msg := pal.Dim + "  no jobs" + aRst
		if m.signalBoard.query != "" {
			msg = pal.Dim + "  no matches" + aRst
		}
		lines = append(lines, boxRow(msg, width, borderColor))
	} else {
		// Clamp scroll offset.
		scrollOff := m.signalBoard.scrollOffset
		if scrollOff > len(filtered)-1 {
			scrollOff = len(filtered) - 1
		}
		if scrollOff < 0 {
			scrollOff = 0
		}

		// Visible window.
		end := scrollOff + visibleEntries
		if end > len(filtered) {
			end = len(filtered)
		}
		shown := filtered[scrollOff:end]

		maxTitleLen := max(width-6, 8)

		for i, entry := range shown {
			absIdx := scrollOff + i

			var led string
			switch entry.status {
			case FeedRunning:
				if m.signalBoard.blinkOn {
					led = pal.Accent + "●" + aRst
				} else {
					led = pal.Dim + "●" + aRst
				}
			case FeedPaused:
				led = pal.Warn + "?" + aRst
			case FeedDone:
				led = pal.Success + "●" + aRst
			case FeedFailed:
				led = pal.Error + "●" + aRst
			default:
				led = pal.Dim + "●" + aRst
			}

			statusLabel := ""
			switch entry.status {
			case FeedRunning:
				statusLabel = pal.Accent + "running" + aRst
			case FeedPaused:
				statusLabel = pal.Warn + "reply?" + aRst
			case FeedDone:
				statusLabel = pal.Success + "done" + aRst
			case FeedFailed:
				statusLabel = pal.Error + "failed" + aRst
			}

			title := entry.title
			if len([]rune(title)) > maxTitleLen {
				title = string([]rune(title)[:maxTitleLen-1]) + "…"
			}

			cursor := "  "
			if absIdx == m.signalBoard.selectedIdx && m.signalBoardFocused {
				cursor = pal.Accent + "> " + aRst
			}

			startTime := entry.ts.Format("3:04 PM")
			duration := humanDuration(time.Since(entry.ts))

			// Row 1: name
			lines = append(lines, boxRow(fmt.Sprintf("%s%s", cursor, title), width, borderColor))
			// Row 2: start time | duration
			lines = append(lines, boxRow(fmt.Sprintf("  %s%s%s  %s|%s  %s%s%s",
				pal.Dim, startTime, aRst,
				pal.Dim, aRst,
				pal.Dim, duration, aRst), width, borderColor))
			// Row 3: status indicator
			lines = append(lines, boxRow(fmt.Sprintf("  [%s]  %s", led, statusLabel), width, borderColor))
		}
	}

	// Pad to fill height, leaving room for the always-present hint footer row.
	for len(lines) < height-2 {
		lines = append(lines, boxRow("", width, borderColor))
	}
	// Hint footer row — always present; shows hints when focused, blank when not.
	var sbHints []panelrender.Hint
	if m.signalBoardFocused {
		if m.signalBoard.searching {
			sbHints = []panelrender.Hint{
				{Key: "type", Desc: "search"},
				{Key: "j/k", Desc: "nav"},
				{Key: "enter", Desc: "confirm"},
				{Key: "esc", Desc: "clear"},
			}
		} else {
			sbHints = []panelrender.Hint{
				{Key: "j/k", Desc: "nav"},
				{Key: "/", Desc: "search"},
				{Key: "f", Desc: "filter"},
				{Key: "enter", Desc: "go to window"},
			}
			if len(filtered) > 0 {
				sbHints = append(sbHints, panelrender.Hint{Key: "d", Desc: "archive"})
				if m.signalBoard.selectedIdx < len(filtered) && filtered[m.signalBoard.selectedIdx].status == FeedRunning {
					sbHints = append(sbHints, panelrender.Hint{Key: "x", Desc: "kill"})
				}
			}
		}
	}
	lines = append(lines, boxRow(panelrender.HintBar(sbHints, width-2, pal), width, borderColor))
	lines = append(lines, boxBot(width, borderColor))

	// Clip to exact height.
	if len(lines) > height {
		lines = lines[:height]
	}

	return lines
}
