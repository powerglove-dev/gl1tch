package switchboard

import (
	"fmt"

	"github.com/sahilm/fuzzy"
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

var filterCycle = []string{"all", "running", "done", "failed"}

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

// signalBoardVisibleRows computes the number of visible body rows using the
// same height formula as the View() layout and buildSignalBoard header count.
func (m Model) signalBoardVisibleRows() int {
	h := m.height
	if h <= 0 {
		h = 40
	}
	contentH := max(h-1, 5)
	maxSB := max(contentH*40/100, 8)
	sbHeight := min(len(m.feed)+6, maxSB)
	if sbHeight < 5 {
		sbHeight = 5
	}
	if sbHeight > contentH-3 {
		sbHeight = max(contentH-3, 5)
	}

	// Mirror header line count from buildSignalBoard:
	// sprite path: 3 sprite lines + 1 filter line = 4; else boxTop = 1 line.
	// Plus 1 search line when focused. Plus 1 for boxBot.
	headerRows := 1 // boxTop fallback
	if PanelHeader(m.activeBundle(), "signal_board", 80, "") != nil {
		headerRows = 4 // sprite(3) + filter(1)
	}
	if m.signalBoardFocused || m.signalBoard.query != "" {
		headerRows++ // search input line
	}
	headerRows++ // boxBot

	bodyH := sbHeight - headerRows
	if bodyH < 1 {
		bodyH = 1
	}
	return bodyH
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
	if sprite := PanelHeader(m.activeBundle(), "signal_board", width, borderColor); sprite != nil {
		lines = append(lines, sprite...)
		scrollIndicator := ""
		if total > 0 {
			scrollIndicator = fmt.Sprintf("  %s%d/%d%s", pal.Dim, sel, total, aRst)
		}
		filterLine := fmt.Sprintf("  filter: %s%s%s%s", pal.Accent, filter, aRst, scrollIndicator)
		lines = append(lines, boxRow(filterLine, width, borderColor))
	} else {
		header := fmt.Sprintf("%s [%s]", RenderHeader("signal_board"), filter)
		lines = append(lines, boxTop(width, header, borderColor, pal.Accent))
	}

	// Search input line (always visible when signal board focused, or when query non-empty).
	if m.signalBoardFocused || m.signalBoard.query != "" {
		cursor := ""
		if m.signalBoard.searching {
			cursor = pal.Accent + "█" + aRst
		}
		searchLine := fmt.Sprintf("  %s/ %s%s%s%s", pal.Dim, aRst, pal.FG, m.signalBoard.query, cursor+aRst)
		lines = append(lines, boxRow(searchLine, width, borderColor))
	}

	filtered := preFiltered

	// Cap to available body rows.
	bodyH := height - len(lines) - 1 // -1 for boxBot
	if bodyH <= 0 {
		bodyH = 1
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
		end := scrollOff + bodyH
		if end > len(filtered) {
			end = len(filtered)
		}
		shown := filtered[scrollOff:end]

		for i, entry := range shown {
			absIdx := scrollOff + i
			ts := entry.ts.Format("15:04:05")

			var led string
			switch entry.status {
			case FeedRunning:
				if m.signalBoard.blinkOn {
					led = pal.Accent + "●" + aRst
				} else {
					led = pal.Dim + "●" + aRst
				}
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
			case FeedDone:
				statusLabel = pal.Success + "done" + aRst
			case FeedFailed:
				statusLabel = pal.Error + "failed" + aRst
			}

			title := entry.title
			maxTitleLen := max(width-30, 8)
			if len([]rune(title)) > maxTitleLen {
				title = string([]rune(title)[:maxTitleLen-1]) + "…"
			}

			cursor := "  "
			if absIdx == m.signalBoard.selectedIdx && m.signalBoardFocused {
				cursor = pal.Accent + "> " + aRst
			}
			rowContent := fmt.Sprintf("%s[%s] %s  %-*s  %s",
				cursor, led, ts, maxTitleLen, title, statusLabel)
			lines = append(lines, boxRow(rowContent, width, borderColor))
		}
	}

	// Pad to fill height.
	for len(lines) < height-1 {
		lines = append(lines, boxRow("", width, borderColor))
	}
	lines = append(lines, boxBot(width, borderColor))

	// Clip to exact height.
	if len(lines) > height {
		lines = lines[:height]
	}

	return lines
}
