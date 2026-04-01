// Package jumpwindow implements the ^spc j window-jump popup.
// Lists the gl1tch main session and all other tmux windows with a live
// search filter. Enter switches to the selected window.
package jumpwindow

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/8op-org/gl1tch/internal/panelrender"
	"github.com/8op-org/gl1tch/internal/styles"
	"github.com/8op-org/gl1tch/internal/themes"
	"github.com/8op-org/gl1tch/internal/tuikit"

	"os"
)

// window is a single entry in the jump list.
type window struct {
	index string // tmux window index
	name  string // display name
	id    string // tmux window ID (@N)
	main  bool   // synthetic entry that navigates to gl1tch session window 0
}

// CloseMsg is posted by an EmbeddedModel when it wants the parent to dismiss it.
type CloseMsg struct{}

type model struct {
	windows    []window // full list (main entry + user windows)
	filtered   []window // after search filter
	selected   int
	input      textinput.Model
	width      int
	height     int
	themeState tuikit.ThemeState
	embedded   bool
}

func newModel() model {
	var bundle *themes.Bundle
	if gr := themes.GlobalRegistry(); gr != nil {
		bundle = gr.Active()
	} else {
		home, _ := os.UserHomeDir()
		if reg, err := themes.NewRegistry(filepath.Join(home, ".config", "glitch", "themes")); err == nil {
			bundle = reg.Active()
		}
	}

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
	ti.Placeholder = "jump to window..."
	ti.Focus()
	ti.PromptStyle = lipgloss.NewStyle().Foreground(accent)
	ti.TextStyle = lipgloss.NewStyle().Foreground(fg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim)
	ti.Prompt = "> "

	wins := []window{{name: "gl1tch", main: true}}
	wins = append(wins, listWindows()...)

	m := model{
		windows:    wins,
		filtered:   wins,
		input:      ti,
		themeState: tuikit.NewThemeState(bundle),
	}
	return m
}

// listWindows queries tmux for all windows except window 0.
func listWindows() []window {
	out, err := exec.Command("tmux", "list-windows",
		"-F", "#{window_index}:#{window_id}:#{window_name}:#{@glitch-label}").Output()
	if err != nil {
		return nil
	}
	var wins []window
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
			continue // skip gl1tch window
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
			if m.embedded {
				return m, func() tea.Msg { return CloseMsg{} }
			}
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
			if m.selected < len(m.filtered) {
				w := m.filtered[m.selected]
				if w.main {
					exec.Command("tmux", "switch-client", "-t", "glitch").Run()   //nolint:errcheck
					exec.Command("tmux", "select-window", "-t", "glitch:0").Run() //nolint:errcheck
				} else {
					target := w.id
					if target == "" {
						target = w.index
					}
					exec.Command("tmux", "select-window", "-t", target).Run() //nolint:errcheck
				}
			}
			if m.embedded {
				return m, func() tea.Msg { return CloseMsg{} }
			}
			return m, tea.Quit
		}
	}

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
		m.selected = 0
	} else if m.selected >= len(m.filtered) {
		m.selected = len(m.filtered) - 1
	}
}

func (m model) View() string {
	w := m.width
	if w <= 0 {
		w = 60
	}

	bundle := m.themeState.Bundle()

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

	// Header.
	if sprite := panelrender.PanelHeader(bundle, "jump_window", w, apal.Accent, apal.Accent); sprite != nil {
		rows = append(rows, sprite...)
	} else {
		rows = append(rows, panelrender.BoxTop(w, "GL1TCH  Jump to Window", apal.Border, apal.Accent))
	}

	// Search input.
	rows = append(rows, panelrender.BoxRow(" "+m.input.View(), w, apal.Border))
	rows = append(rows, panelrender.BoxRow("", w, apal.Border))

	// Window list.
	// Fixed overhead: header(1) + search(1) + blank(1) + hints(1) + bottom(1) = 5.
	listTarget := max(m.height-5, 1)

	if len(m.filtered) == 0 {
		rows = append(rows, panelrender.BoxRow(apal.Dim+"  no windows found"+panelrender.RST, w, apal.Border))
		listTarget--
	} else {
		for i, win := range m.filtered {
			if i >= listTarget {
				break
			}
			var row string
			if i == m.selected {
				row = fmt.Sprintf("%s  > %s%s", apal.Accent, win.name, panelrender.RST)
			} else {
				row = fmt.Sprintf("%s    %s%s", apal.FG, win.name, panelrender.RST)
			}
			rows = append(rows, panelrender.BoxRow(row, w, apal.Border))
		}
		listTarget -= min(len(m.filtered), listTarget)
	}

	// Pad remaining rows.
	for range listTarget {
		rows = append(rows, panelrender.BoxRow("", w, apal.Border))
	}

	// Hint bar.
	hints := panelrender.HintBar([]panelrender.Hint{
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

// Run launches the jump window as a standalone BubbleTea program.
func Run() {
	prog := tea.NewProgram(newModel(), tea.WithAltScreen())
	prog.Run() //nolint:errcheck
}

// padToWidth appends spaces to s until its lipgloss visible width equals visibleW.
func padToWidth(s string, visibleW int) string {
	cur := lipgloss.Width(s)
	if cur >= visibleW {
		return s
	}
	return s + strings.Repeat(" ", visibleW-cur)
}
