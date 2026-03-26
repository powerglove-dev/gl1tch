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

	sbTitle := RenderHeader(m.activeBundle(), "signal_board", width)
	header := fmt.Sprintf("%s [%s]", sbTitle, filter)
	var lines []string
	lines = append(lines, boxTop(width, header, borderColor))

	filtered := m.filteredFeed()

	// Cap to available body rows.
	bodyH := height - 2
	if bodyH <= 0 {
		bodyH = 1
	}

	if len(filtered) == 0 {
		lines = append(lines, boxRow(aDim+"  no jobs"+aRst, width))
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
					led = aBrC + "●" + aRst
				} else {
					led = aDim + "●" + aRst
				}
			case FeedDone:
				led = aGrn + "●" + aRst
			case FeedFailed:
				led = aRed + "●" + aRst
			default:
				led = aDim + "●" + aRst
			}

			statusLabel := ""
			switch entry.status {
			case FeedRunning:
				statusLabel = aBrC + "running" + aRst
			case FeedDone:
				statusLabel = aGrn + "done" + aRst
			case FeedFailed:
				statusLabel = aRed + "failed" + aRst
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
				selRow := aBC + "│" + aSelBg + aWht + rowContent + strings.Repeat(" ", pad) + aRst + aBC + "│" + aRst
				lines = append(lines, selRow)
			} else {
				lines = append(lines, boxRow(rowContent, width))
			}
		}
	}

	// Pad to fill height.
	for len(lines) < height-1 {
		lines = append(lines, boxRow("", width))
	}
	lines = append(lines, boxBot(width))

	// Clip to exact height.
	if len(lines) > height {
		lines = lines[:height]
	}

	return lines
}
