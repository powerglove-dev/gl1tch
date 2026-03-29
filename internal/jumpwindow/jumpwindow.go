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
	index         string // tmux window index (string for display)
	name          string // window name
	id            string // window ID (@N)
	switchboard   bool   // synthetic entry that navigates to orcai session window 0
	cronSession   bool   // synthetic entry that switches to the orcai-cron session
	promptsWindow bool   // synthetic entry that opens the prompt manager TUI
}

type model struct {
	windows       []window
	filtered      []window
	sysop         []window
	selectedSysop int // cursor within the sysop (left) column
	selectedJob   int // cursor within the active jobs (right) column
	focusCol      int // 0 = left (sysop), 1 = right (active jobs)
	input         textinput.Model
	width         int
	height        int
	err           string
	themeState    tuikit.ThemeState
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
	ti.Placeholder = "search active jobs..."
	ti.Focus()
	ti.PromptStyle = lipgloss.NewStyle().Foreground(accent)
	ti.TextStyle = lipgloss.NewStyle().Foreground(fg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim)
	ti.Prompt = "> "

	m := model{
		input:         ti,
		themeState:    tuikit.NewThemeState(bundle),
		selectedSysop: 0,
		selectedJob:   0,
		focusCol:      0,
	}
	m.windows = listWindows()
	m.filtered = m.windows
	sysop := []window{{name: "switchboard", switchboard: true}}
	cronWins := listSysopWindows()
	if cronWins != nil {
		sysop = append(sysop, window{name: "cron", cronSession: true})
		sysop = append(sysop, cronWins...)
	}
	sysop = append(sysop, window{name: "prompts", promptsWindow: true})
	m.sysop = sysop
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
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
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
// Returns a non-nil (possibly empty) slice when the session exists, so callers
// can distinguish "no session" from "session exists but only has window 0".
func listSysopWindows() []window {
	out, err := exec.Command("tmux", "list-windows",
		"-t", "orcai-cron",
		"-F", "#{window_index}:#{window_id}:#{window_name}:#{@orcai-label}").Output()
	if err != nil {
		return nil // session doesn't exist or tmux unavailable
	}
	wins := []window{} // non-nil: signals session exists even when no extra windows
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 4)
		if len(parts) != 4 {
			continue
		}
		idx, id, rawName, label := parts[0], parts[1], parts[2], parts[3]
		if idx == "0" {
			continue // skip session bootstrap window
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

		case "tab":
			m.focusCol = 1 - m.focusCol
			return m, nil

		case "up", "k":
			if m.focusCol == 0 {
				if m.selectedSysop > 0 {
					m.selectedSysop--
				}
			} else {
				if m.selectedJob > 0 {
					m.selectedJob--
				}
			}
			return m, nil

		case "down", "j":
			if m.focusCol == 0 {
				if m.selectedSysop < len(m.sysop)-1 {
					m.selectedSysop++
				}
			} else {
				if m.selectedJob < len(m.filtered)-1 {
					m.selectedJob++
				}
			}
			return m, nil

		case "enter":
			if m.focusCol == 0 {
				if m.selectedSysop < len(m.sysop) {
					w := m.sysop[m.selectedSysop]
					if w.switchboard {
						exec.Command("tmux", "switch-client", "-t", "orcai").Run()   //nolint:errcheck
						exec.Command("tmux", "select-window", "-t", "orcai:0").Run() //nolint:errcheck
					} else if w.cronSession {
						exec.Command("tmux", "switch-client", "-t", "orcai-cron").Run() //nolint:errcheck
					} else if w.promptsWindow {
						self, _ := os.Executable()
						self = filepath.Clean(self)
						exec.Command("tmux", "new-window", "-n", "orcai-prompts", self+" prompts tui").Run() //nolint:errcheck
					} else {
						target := w.id
						if target == "" {
							target = "orcai-cron:" + w.index
						}
						exec.Command("tmux", "switch-client", "-t", "orcai-cron").Run() //nolint:errcheck
						exec.Command("tmux", "select-window", "-t", target).Run()       //nolint:errcheck
					}
				}
			} else {
				if m.selectedJob < len(m.filtered) {
					w := m.filtered[m.selectedJob]
					target := w.id
					if target == "" {
						target = w.index
					}
					exec.Command("tmux", "select-window", "-t", target).Run() //nolint:errcheck
				}
			}
			return m, tea.Quit

		case "e":
			if m.focusCol == 1 && m.selectedJob < len(m.filtered) {
				openPipelineInEditor(m.filtered[m.selectedJob].name)
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
	} else {
		var out []window
		for _, w := range m.windows {
			if strings.Contains(strings.ToLower(w.name), q) {
				out = append(out, w)
			}
		}
		m.filtered = out
	}
	if len(m.filtered) == 0 {
		m.selectedJob = 0
	} else if m.selectedJob >= len(m.filtered) {
		m.selectedJob = len(m.filtered) - 1
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

	var rows []string

	// Header — sprite or dynamic panel header.
	if sprite := panelrender.PanelHeader(bundle, "jump_window", w, apal.Accent, apal.Accent); sprite != nil {
		rows = append(rows, sprite...)
	} else {
		rows = append(rows, panelrender.BoxTop(w, "ORCAI  Jump to Window", apal.Border, apal.Accent))
	}

	// Search input row.
	inputContent := " " + m.input.View()
	rows = append(rows, panelrender.BoxRow(inputContent, w, apal.Border))
	rows = append(rows, panelrender.BoxRow("", w, apal.Border))

	// Body: two-column layout or narrow fallback.
	// Fixed overhead: header(1)+search(1)+blank(1)+hints(1)+botborder(1) = 5.
	// Body rows must fill the remaining height so the box spans the full popup.
	bodyTarget := max(m.height-5, 2) // minimum: col-headers + 1 item
	if w < 40 {
		rows = append(rows, m.viewNarrow(w, bodyTarget, apal)...)
	} else {
		rows = append(rows, m.viewTwoCol(w, bodyTarget, apal)...)
	}

	// Hint bar (no "e edit" — feature still works via key, just not advertised).
	hints := panelrender.HintBar([]panelrender.Hint{
		{Key: "tab", Desc: "switch col"},
		{Key: "j/k", Desc: "nav"},
		{Key: "enter", Desc: "select"},
		{Key: "esc", Desc: "cancel"},
	}, w-2, apal)
	rows = append(rows,
		panelrender.BoxRow(hints, w, apal.Border),
		panelrender.BoxBot(w, apal.Border),
	)

	return strings.Join(rows, "\n")
}

// viewNarrow renders a single-column layout for terminals narrower than 40 cols.
// bodyTarget is the total number of rows this function should return to fill the popup.
func (m model) viewNarrow(w, bodyTarget int, apal styles.ANSIPalette) []string {
	var rows []string

	if len(m.sysop) > 0 {
		rows = append(rows, panelrender.BoxRow("   "+apal.Dim+"— sysop —"+panelrender.RST, w, apal.Border))
		for i, win := range m.sysop {
			label := win.name
			if m.focusCol == 0 && m.selectedSysop == i {
				rows = append(rows, panelrender.BoxRow(apal.Accent+"   > "+label+panelrender.RST, w, apal.Border))
			} else {
				rows = append(rows, panelrender.BoxRow(apal.Dim+"     "+label+panelrender.RST, w, apal.Border))
			}
		}
	}

	rows = append(rows, panelrender.BoxRow("   "+apal.Accent+"— active jobs —"+panelrender.RST, w, apal.Border))
	if len(m.filtered) == 0 {
		rows = append(rows, panelrender.BoxRow(apal.Dim+"     no windows found"+panelrender.RST, w, apal.Border))
	} else {
		for i, win := range m.filtered {
			label := win.name
			if m.focusCol == 1 && m.selectedJob == i {
				rows = append(rows, panelrender.BoxRow(apal.Accent+"   > "+label+panelrender.RST, w, apal.Border))
			} else {
				rows = append(rows, panelrender.BoxRow(apal.FG+"     "+label+panelrender.RST, w, apal.Border))
			}
		}
	}

	// Pad with blank rows to fill the popup height.
	for len(rows) < bodyTarget {
		rows = append(rows, panelrender.BoxRow("", w, apal.Border))
	}

	return rows
}

// viewTwoCol renders the two-column sysop/active-jobs layout.
// bodyTarget is the total number of rows this function should return to fill the popup.
func (m model) viewTwoCol(w, bodyTarget int, apal styles.ANSIPalette) []string {
	leftW := (w - 3) / 2
	rightW := w - 3 - leftW

	var rows []string

	// Column headers row (counts as 1 body row).
	leftHdr := padToWidth(apal.Dim+"  — sysop —"+panelrender.RST, leftW)
	rightHdr := padToWidth(apal.Accent+"  — active jobs —"+panelrender.RST, rightW)
	rows = append(rows, panelrender.BoxRow(leftHdr+apal.Border+"│"+panelrender.RST+rightHdr, w, apal.Border))

	// Item rows: render actual content, then pad to fill remaining body rows.
	itemTarget := max(bodyTarget-1, 1) // subtract the col-headers row

	rightCount := max(len(m.filtered), 1) // at least 1 for placeholder
	contentRows := max(len(m.sysop), rightCount)
	renderRows := max(contentRows, itemTarget)

	for i := range renderRows {
		var leftCell, rightCell string

		if i < len(m.sysop) {
			leftCell = buildCell(m.sysop[i].name, m.selectedSysop == i, m.focusCol == 0, leftW, apal)
		} else {
			leftCell = strings.Repeat(" ", leftW)
		}

		switch {
		case len(m.filtered) == 0 && i == 0:
			rightCell = padToWidth(apal.Dim+"  no windows found"+panelrender.RST, rightW)
		case i < len(m.filtered):
			rightCell = buildCell(m.filtered[i].name, m.selectedJob == i, m.focusCol == 1, rightW, apal)
		default:
			rightCell = strings.Repeat(" ", rightW)
		}

		rows = append(rows, panelrender.BoxRow(leftCell+apal.Border+"│"+panelrender.RST+rightCell, w, apal.Border))
	}

	return rows
}

// buildCell renders an item label for a column cell, padded to visibleW visible chars.
// selected: whether this item is under the cursor.
// focused: whether this column currently has keyboard focus.
func buildCell(label string, selected, focused bool, visibleW int, apal styles.ANSIPalette) string {
	const cursorPfx = "  > "
	const blankPfx = "    "
	var s string
	if selected {
		if focused {
			s = apal.Accent + cursorPfx + label + panelrender.RST
		} else {
			s = apal.Dim + cursorPfx + label + panelrender.RST
		}
	} else {
		s = apal.FG + blankPfx + label + panelrender.RST
	}
	return padToWidth(s, visibleW)
}

// padToWidth appends spaces to s until its lipgloss visible width equals visibleW.
func padToWidth(s string, visibleW int) string {
	cur := lipgloss.Width(s)
	if cur >= visibleW {
		return s
	}
	return s + strings.Repeat(" ", visibleW-cur)
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
