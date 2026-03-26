// Package jumpwindow implements the ^spc j window-jump popup.
// Lists all tmux windows in the current session (excluding window 0)
// with a live search filter. Enter switches to the selected window.
package jumpwindow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/themes"
)

// window is a single tmux window entry.
type window struct {
	index string // tmux window index (string for display)
	name  string // window name
	id    string // window ID (@N)
}

type jumpPalette struct {
	titleBG lipgloss.Color
	titleFG lipgloss.Color
	fg      lipgloss.Color
	accent  lipgloss.Color
	selBG   lipgloss.Color
	selFG   lipgloss.Color
	dim     lipgloss.Color
}

func loadPalette() jumpPalette {
	p := jumpPalette{
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
		p.titleBG = lipgloss.Color(v)
	}
	if v := b.ResolveRef(b.Modal.TitleBG); v != "" {
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

// listWindows queries tmux for all windows except window 0.
// Uses @orcai-label (set at window creation) for the display name when available,
// falling back to the raw window name. Appends " #<index>" to distinguish
// multiple windows with the same label.
func listWindows() []window {
	out, err := exec.Command("tmux", "list-windows",
		"-F", "#{window_index}:#{window_id}:#{window_name}:#{@orcai-label}").Output()
	if err != nil {
		return nil
	}
	var wins []window
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Format: index:id:name:label (label may be empty)
		parts := strings.SplitN(line, ":", 4)
		if len(parts) != 4 {
			continue
		}
		idx, id, rawName, label := parts[0], parts[1], parts[2], parts[3]
		if idx == "0" {
			continue // skip switchboard window
		}
		display := label
		if display == "" {
			display = rawName
		}
		display += "  #" + idx
		wins = append(wins, window{index: idx, name: display, id: id})
	}
	return wins
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
			return m, nil

		case "down", "j":
			if m.selected < len(m.filtered)-1 {
				m.selected++
			}
			return m, nil

		case "enter":
			if len(m.filtered) > 0 && m.selected < len(m.filtered) {
				w := m.filtered[m.selected]
				target := w.id
				if target == "" {
					target = w.index
				}
				exec.Command("tmux", "select-window", "-t", target).Run() //nolint:errcheck
			}
			return m, tea.Quit

		case "e":
			if len(m.filtered) > 0 && m.selected < len(m.filtered) {
				openPipelineInEditor(m.filtered[m.selected].name)
			}
			return m, tea.Quit
		}
	}

	// Update search input and refilter.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.applyFilter()
	return m, cmd
}

func (m *model) applyFilter() {
	q := strings.ToLower(m.input.Value())
	if q == "" {
		m.filtered = m.windows
		if m.selected >= len(m.filtered) {
			m.selected = max(len(m.filtered)-1, 0)
		}
		return
	}
	var out []window
	for _, w := range m.windows {
		if strings.Contains(strings.ToLower(w.name), q) {
			out = append(out, w)
		}
	}
	m.filtered = out
	if m.selected >= len(m.filtered) {
		m.selected = max(len(m.filtered)-1, 0)
	}
}

func (m model) View() string {
	w := m.width
	if w <= 0 {
		w = 70
	}

	headerStyle := lipgloss.NewStyle().
		Background(m.pal.titleBG).
		Foreground(m.pal.titleFG).
		Bold(true).
		Width(w).
		Padding(0, 1)

	inputRow := lipgloss.NewStyle().
		Width(w).
		Padding(0, 1).
		Render(m.input.View())

	sectionStyle := lipgloss.NewStyle().
		Foreground(m.pal.accent).
		Width(w).
		Padding(0, 1)

	selectedStyle := lipgloss.NewStyle().
		Background(m.pal.selBG).
		Foreground(m.pal.accent).
		Width(w)

	normalStyle := lipgloss.NewStyle().
		Foreground(m.pal.fg).
		Width(w).
		Padding(0, 2)

	dimStyle := lipgloss.NewStyle().Foreground(m.pal.dim)

	rows := []string{
		headerStyle.Render("ORCAI  Jump to Window"),
		inputRow,
	}

	if len(m.filtered) == 0 {
		rows = append(rows,
			sectionStyle.Render("— active jobs —"),
			lipgloss.NewStyle().
				Foreground(m.pal.dim).
				Width(w).
				Padding(0, 2).
				Render("no windows found"),
		)
	} else {
		rows = append(rows, sectionStyle.Render("— active jobs —"))
		for i, win := range m.filtered {
			label := win.name
			if i == m.selected {
				rows = append(rows, selectedStyle.Render("  "+label))
			} else {
				rows = append(rows, normalStyle.Render(label))
			}
		}
	}

	hintRow := lipgloss.NewStyle().
		Foreground(m.pal.dim).
		Width(w).
		Padding(0, 1).
		Render(fmt.Sprintf("%s nav  %s select  %s cancel",
			dimStyle.Render("↑↓"),
			dimStyle.Render("enter"),
			dimStyle.Render("esc"),
		))

	rows = append(rows, "", hintRow)
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// openPipelineInEditor derives the pipeline YAML path from a display name like
// "http-json-analyze #5" and opens it in $EDITOR (or vi) via a new tmux window.
func openPipelineInEditor(displayName string) {
	// Strip " #N" suffix added by listWindows.
	label := displayName
	if idx := strings.LastIndex(label, "  #"); idx >= 0 {
		label = label[:idx]
	}
	if label == "" {
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	pipelineDir := filepath.Join(home, ".config", "orcai", "pipelines")
	path := filepath.Join(pipelineDir, label+".pipeline.yaml")
	if _, err := os.Stat(path); err != nil {
		return // file doesn't exist — not a pipeline entry
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	out, err := exec.Command("tmux", "display-message", "-p", "#S").Output()
	if err != nil {
		return
	}
	session := strings.TrimSpace(string(out))
	if session == "" {
		return
	}
	out2, err2 := exec.Command("tmux", "new-window", "-d", "-P", "-F", "#{window_id}", "-t", session+":", "-n", "orcai-edit", editor+" "+path).Output()
	if err2 != nil {
		return
	}
	winID := strings.TrimSpace(string(out2))
	if winID != "" {
		exec.Command("tmux", "select-window", "-t", session+":"+winID).Run() //nolint:errcheck
	}
}

// Run starts the jump-window popup as a bubbletea program.
func Run() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("jumpwindow error: %v\n", err)
	}
}
