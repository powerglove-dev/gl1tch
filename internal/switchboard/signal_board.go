package switchboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SignalBoard tracks state for the SIGNAL BOARD panel.
type SignalBoard struct {
	selectedIdx  int
	activeFilter string // "all", "running", "done", "failed"
	blinkOn      bool
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

// buildSignalBoard renders the SIGNAL BOARD panel.
// Returns a slice of rendered lines (including box borders).
func (m Model) buildSignalBoard(height, width int) []string {
	filter := m.signalBoard.activeFilter
	if filter == "" {
		filter = "all"
	}

	borderColor := aBC
	if m.signalBoardFocused {
		borderColor = aBrC
	}
	pal := m.ansiPalette()

	var lines []string
	if sprite := PanelHeader(m.activeBundle(), "signal_board", width); sprite != nil {
		lines = append(lines, sprite...)
		filterLine := fmt.Sprintf("  filter: %s%s%s", pal.Accent, filter, aRst)
		lines = append(lines, boxRow(filterLine, width, borderColor))
	} else {
		header := fmt.Sprintf("%s [%s]", RenderHeader("signal_board"), filter)
		lines = append(lines, boxTop(width, header, borderColor, pal.Accent))
	}

	filtered := m.filteredFeed()

	// Cap to available body rows (header may be 1 line or a multi-line sprite).
	bodyH := height - len(lines) - 1 // -1 for boxBot
	if bodyH <= 0 {
		bodyH = 1
	}

	if len(filtered) == 0 {
		lines = append(lines, boxRow(pal.Dim+"  no jobs"+aRst, width, borderColor))
	} else {
		shown := filtered
		if len(shown) > bodyH {
			shown = shown[:bodyH]
		}
		for i, entry := range shown {
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
			if len(title) > maxTitleLen {
				title = title[:maxTitleLen-1] + "…"
			}

			rowContent := fmt.Sprintf("  [%s] %s  %-*s  %s",
				led, ts, maxTitleLen, title, statusLabel)
			rowVis := lipgloss.Width(rowContent)

			if i == m.signalBoard.selectedIdx && m.signalBoardFocused {
				// Highlight selected row.
				inner := width - 2
				pad := max(inner-rowVis, 0)
				selRow := borderColor + "│" + aSelBg + aWht + rowContent + strings.Repeat(" ", pad) + aRst + borderColor + "│" + aRst
				lines = append(lines, selRow)
			} else {
				lines = append(lines, boxRow(rowContent, width, borderColor))
			}
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
