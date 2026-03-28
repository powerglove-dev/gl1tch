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

	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/styles"
	"github.com/adam-stokes/orcai/internal/themes"
	"github.com/adam-stokes/orcai/internal/tuikit"
)

// window is a single tmux window entry.
type window struct {
	index string // tmux window index (string for display)
	name  string // window name
	id    string // window ID (@N)
}

type model struct {
	windows    []window
	filtered   []window
	sysop      []window
	selected   int
	input      textinput.Model
	width      int
	height     int
	err        string
	themeState tuikit.ThemeState
}

func newModel() model {
	// Seed themeState from GlobalRegistry (set by the caller) or user themes dir.
	var bundle *themes.Bundle
	if gr := themes.GlobalRegistry(); gr != nil {
		bundle = gr.Active()
	} else {
		home, _ := os.UserHomeDir()
		if reg, err := themes.NewRegistry(filepath.Join(home, ".config", "orcai", "themes")); err == nil {
			bundle = reg.Active()
		}
	}

	// Default Dracula accent/fg/dim colors; override from bundle if available.
	accent := lipgloss.Color("#8be9fd")
	fg := lipgloss.Color("#f8f8f2")
	dim := lipgloss.Color("#6272a4")
	if bundle != nil {
		if v := bundle.Palette.Accent; v != "" {
			accent = lipgloss.Color(v)
		}
		if v := bundle.Palette.FG; v != "" {
			fg = lipgloss.Color(v)
		}
		if v := bundle.Palette.Dim; v != "" {
			dim = lipgloss.Color(v)
		}
	}

	ti := textinput.New()
	ti.Placeholder = "search windows..."
	ti.Focus()
	ti.PromptStyle = lipgloss.NewStyle().Foreground(accent)
	ti.TextStyle = lipgloss.NewStyle().Foreground(fg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim)
	ti.Prompt = "> "

	m := model{
		input:      ti,
		themeState: tuikit.NewThemeState(bundle),
	}
	m.windows = listWindows()
	m.filtered = m.windows
	m.sysop = listSysopWindows()
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

// listSysopWindows queries the orcai-cron tmux session for its windows.
// Returns nil if the session does not exist or tmux is unavailable.
func listSysopWindows() []window {
	out, err := exec.Command("tmux", "list-windows",
		"-t", "orcai-cron",
		"-F", "#{window_index}:#{window_id}:#{window_name}:#{@orcai-label}").Output()
	if err != nil {
		return nil // session doesn't exist or tmux unavailable
	}
	var wins []window
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 4)
		if len(parts) != 4 {
			continue
		}
		idx, id, rawName, label := parts[0], parts[1], parts[2], parts[3]
		display := label
		if display == "" {
			display = rawName
		}
		display += "  #" + idx
		wins = append(wins, window{index: idx, name: display, id: id})
	}
	return wins
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.themeState.Init())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ts, cmd, ok := m.themeState.Handle(msg); ok {
		m.themeState = ts
		return m, cmd
	}

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
			total := len(m.filtered) + len(m.sysop)
			if m.selected < total-1 {
				m.selected++
			}
			return m, nil

		case "enter":
			totalFiltered := len(m.filtered)
			if m.selected < totalFiltered {
				if totalFiltered > 0 {
					w := m.filtered[m.selected]
					target := w.id
					if target == "" {
						target = w.index
					}
					exec.Command("tmux", "select-window", "-t", target).Run() //nolint:errcheck
				}
			} else {
				sysopIdx := m.selected - totalFiltered
				if sysopIdx < len(m.sysop) {
					w := m.sysop[sysopIdx]
					target := w.id
					if target == "" {
						target = "orcai-cron:" + w.index
					}
					exec.Command("tmux", "switch-client", "-t", "orcai-cron").Run() //nolint:errcheck
					exec.Command("tmux", "select-window", "-t", target).Run()       //nolint:errcheck
				}
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
		total := len(m.filtered) + len(m.sysop)
		if total == 0 {
			m.selected = 0
		} else if m.selected >= total {
			m.selected = total - 1
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
	total := len(m.filtered) + len(m.sysop)
	if total == 0 {
		m.selected = 0
	} else if m.selected >= total {
		m.selected = total - 1
	}
}

func (m model) View() string {
	w := m.width
	if w <= 0 {
		w = 70
	}

	bundle := m.themeState.Bundle()

	// ANSI palette for box-drawing rows.
	var apal styles.ANSIPalette
	if bundle != nil {
		apal = styles.BundleANSI(bundle)
	} else {
		apal = styles.ANSIPalette{
			Accent: "\x1b[35m",
			Dim:    "\x1b[2m",
			FG:     "\x1b[97m",
			BG:     "\x1b[40m",
			Border: "\x1b[36m",
			SelBG:  "\x1b[44m",
		}
	}

	// Build rows using ANSI box drawing to match switchboard panels.
	var rows []string

	// Header — sprite or dynamic panel header.
	if sprite := panelrender.PanelHeader(bundle, "jump_window", w, apal.Accent); sprite != nil {
		rows = append(rows, sprite...)
	} else {
		rows = append(rows, panelrender.BoxTop(w, "ORCAI  Jump to Window", apal.Border, apal.Accent))
	}

	// Search input row.
	inputContent := "  " + m.input.View()
	rows = append(rows, panelrender.BoxRow(inputContent, w, apal.Border))

	// Section: active jobs.
	rows = append(rows, panelrender.BoxRow(apal.Accent+"— active jobs —"+panelrender.RST, w, apal.Border))

	if len(m.filtered) == 0 {
		rows = append(rows, panelrender.BoxRow(apal.Dim+"  no windows found"+panelrender.RST, w, apal.Border))
	} else {
		for i, win := range m.filtered {
			label := win.name
			if i == m.selected {
				content := apal.SelBG + apal.Accent + "  " + label + panelrender.RST
				rows = append(rows, panelrender.BoxRow(content, w, apal.Border))
			} else {
				rows = append(rows, panelrender.BoxRow(apal.FG+"  "+label+panelrender.RST, w, apal.Border))
			}
		}
	}

	// Section: sysop (orcai-cron session windows).
	if len(m.sysop) > 0 {
		rows = append(rows, panelrender.BoxRow(apal.Dim+"— sysop —"+panelrender.RST, w, apal.Border))
		for i, win := range m.sysop {
			label := win.name
			sysopIdx := len(m.filtered) + i
			if m.selected == sysopIdx {
				content := apal.SelBG + apal.Accent + "  " + label + panelrender.RST
				rows = append(rows, panelrender.BoxRow(content, w, apal.Border))
			} else {
				rows = append(rows, panelrender.BoxRow(apal.Dim+"  "+label+panelrender.RST, w, apal.Border))
			}
		}
	}

	// Hint bar row (accent keys, dim descriptions).
	hint := func(key, desc string) string {
		return apal.Accent + key + apal.Dim + " " + desc + panelrender.RST
	}
	sep := apal.Dim + " · " + panelrender.RST
	hintContent := strings.Join([]string{
		hint("j/k", "nav"),
		hint("enter", "select"),
		hint("e", "edit"),
		hint("esc", "cancel"),
	}, sep)
	rows = append(rows,
		panelrender.BoxRow("", w, apal.Border),
		panelrender.BoxRow(hintContent, w, apal.Border),
		panelrender.BoxBot(w, apal.Border),
	)

	return strings.Join(rows, "\n")
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
