package promptmgr

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Dracula palette constants used throughout the prompt manager views.
const (
	draculaBg      = "#282a36"
	draculaCurrent = "#44475a"
	draculaFg      = "#f8f8f2"
	draculaCyan    = "#8be9fd"
	draculaGreen   = "#50fa7b"
	draculaComment = "#6272a4"
)

// View renders the full three-panel prompt manager layout.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "prompt manager — loading..."
	}

	leftWidth := m.width / 3
	rightWidth := m.width - leftWidth

	left := m.viewList(leftWidth, m.height)

	editorHeight := m.height / 2
	runnerHeight := m.height - editorHeight

	editor := m.viewEditor(rightWidth, editorHeight)
	runner := m.viewRunner(rightWidth, runnerHeight)

	right := lipgloss.JoinVertical(lipgloss.Left, editor, runner)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// viewList renders the left panel: filter input + prompt list.
func (m *Model) viewList(width, height int) string {
	panelStyle := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(lipgloss.Color(draculaBg)).
		Foreground(lipgloss.Color(draculaFg))

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(draculaCyan)).
		Bold(true)

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(draculaCurrent)).
		Foreground(lipgloss.Color(draculaFg))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(draculaComment))

	// Header
	header := headerStyle.Render("╔══ PROMPTS ══╗")

	// Filter input
	filterLine := "  " + m.filterInput.View()

	// List rows — compute visible window
	visibleRows := height - 4 // header + filter + border lines
	if visibleRows < 1 {
		visibleRows = 1
	}

	var rows []string
	if len(m.filtered) == 0 {
		rows = append(rows, dimStyle.Render("  no prompts — press n to create"))
	} else {
		end := m.scrollOffset + visibleRows
		if end > len(m.filtered) {
			end = len(m.filtered)
		}
		for i := m.scrollOffset; i < end; i++ {
			p := m.filtered[i]
			var row string
			if p.ModelSlug != "" {
				row = fmt.Sprintf("  %s  [%s]", p.Title, p.ModelSlug)
			} else {
				row = "  " + p.Title
			}
			// Truncate if too wide.
			if len(row) > width-2 {
				row = row[:width-5] + "..."
			}
			if i == m.selectedIdx {
				row = selectedStyle.Width(width - 2).Render(row)
			} else {
				row = lipgloss.NewStyle().
					Width(width - 2).
					Foreground(lipgloss.Color(draculaFg)).
					Render(row)
			}
			rows = append(rows, row)
		}
	}

	// Build content.
	content := header + "\n" + filterLine + "\n"
	for _, r := range rows {
		content += r + "\n"
	}

	// Delete confirmation overlay.
	if m.confirmDelete && len(m.filtered) > 0 {
		title := m.filtered[m.selectedIdx].Title
		overlay := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff5555")).
			Bold(true).
			Render(fmt.Sprintf("  delete '%s'? (y/n)", title))
		content += "\n" + overlay + "\n"
	}

	// Status message.
	if m.statusMsg != "" {
		content += dimStyle.Render("  "+m.statusMsg) + "\n"
	}

	return panelStyle.Render(content)
}

// viewEditor renders the right-top editor panel.
func (m *Model) viewEditor(width, height int) string {
	panelStyle := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(lipgloss.Color(draculaBg)).
		Foreground(lipgloss.Color(draculaFg))

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(draculaGreen)).
		Bold(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(draculaComment))

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(draculaComment))

	header := headerStyle.Render("╔══ EDITOR ══╗")

	titleLabel := labelStyle.Render("Title:")
	titleField := m.titleInput.View()

	bodyLabel := labelStyle.Render("Body:")
	bodyField := m.bodyInput.View()

	// Model selector.
	modelSlug := "(none)"
	if len(m.modelSlugs) > 0 && m.modelIdx < len(m.modelSlugs) {
		modelSlug = m.modelSlugs[m.modelIdx]
	}
	modelLine := labelStyle.Render("Model: ") + fmt.Sprintf("◀ %s ▶", modelSlug)

	// CWD field.
	cwdLabel := labelStyle.Render("CWD:")
	cwdField := m.cwdInput.View()

	footer := footerStyle.Render("ctrl+s save  ctrl+r run  [ ] model  tab panel")

	content := header + "\n" +
		titleLabel + "\n" + titleField + "\n" +
		bodyLabel + "\n" + bodyField + "\n" +
		modelLine + "\n" +
		cwdLabel + "\n" + cwdField + "\n" +
		footer

	return panelStyle.Render(content)
}

// viewRunner renders the right-bottom test runner panel.
func (m *Model) viewRunner(width, height int) string {
	panelStyle := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(lipgloss.Color(draculaBg)).
		Foreground(lipgloss.Color(draculaFg))

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(draculaCyan)).
		Bold(true)

	header := headerStyle.Render("── TEST RUNNER ──")

	output := m.runnerOutput
	if output == "" {
		output = "(ctrl+r to run)"
	}

	content := header + "\n" + output

	return panelStyle.Render(content)
}
