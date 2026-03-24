package sidebar

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Window represents a tmux window (excluding window 0).
type Window struct {
	Index  int
	Name   string
	Active bool
}

// ParseWindows parses output of:
//
//	tmux list-windows -t orcai -F "#{window_index} #{window_name} #{window_active}"
//
// Skips window 0 (the ORCAI home window).
func ParseWindows(output string) []Window {
	var windows []Window
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil || idx == 0 {
			continue
		}
		windows = append(windows, Window{
			Index:  idx,
			Name:   parts[1],
			Active: parts[2] == "1",
		})
	}
	return windows
}

func listWindows() []Window {
	out, err := exec.Command("tmux", "list-windows", "-t", "orcai",
		"-F", "#{window_index} #{window_name} #{window_active}").Output()
	if err != nil {
		return nil
	}
	return ParseWindows(string(out))
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// windowPaneCount returns how many panes window idx has, or 2 on error (safe default).
func windowPaneCount(windowIdx int) int {
	out, err := exec.Command("tmux", "list-panes", "-t",
		fmt.Sprintf("orcai:%d", windowIdx), "-F", "x").Output()
	if err != nil {
		return 2
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// currentWindowIndex returns the tmux window index this process is running in.
func currentWindowIndex() int {
	out, err := exec.Command("tmux", "display-message", "-p", "#{window_index}").Output()
	if err != nil {
		return -1
	}
	idx, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return idx
}

// Model is the bubbletea sidebar model.
type Model struct {
	windows []Window
	cursor  int
	width   int
	height  int
	self    string // path to orcai binary, used to spawn sidebars in new windows
	manager bool   // only window 0 sidebar spawns sidebars in other windows
}

// NewWithWindows creates a Model with a fixed window list — used in tests.
func NewWithWindows(windows []Window) Model {
	return Model{windows: windows}
}

// Cursor returns the current cursor position — used in tests.
func (m Model) Cursor() int { return m.cursor }

// New creates the sidebar model by querying tmux.
func New() Model {
	self, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	return Model{
		windows: listWindows(),
		self:    self,
		manager: currentWindowIndex() == 0,
	}
}

// spawnSidebar creates a sidebar pane in the given tmux window index.
func (m Model) spawnSidebar(windowIdx int) {
	if m.self == "" {
		return
	}
	exec.Command("tmux", "split-window",
		"-d", "-h", "-b", "-f", "-l", "25%",
		"-t", fmt.Sprintf("orcai:%d", windowIdx),
		m.self, "_sidebar").Run() //nolint:errcheck
}

// ensureSidebars spawns a sidebar in any window that only has one pane.
func (m Model) ensureSidebars() {
	for _, w := range m.windows {
		if windowPaneCount(w.Index) == 1 {
			m.spawnSidebar(w.Index)
		}
	}
}

func (m Model) Init() tea.Cmd {
	if !m.manager {
		return nil
	}
	return tickCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if m.manager {
			m.windows = listWindows()
			m.ensureSidebars()
		}
		return m, tickCmd()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Self-resize: maintain 25% of the total window width on every resize.
		if pane := os.Getenv("TMUX_PANE"); pane != "" {
			out, err := exec.Command("tmux", "display-message", "-p", "#{window_width}").Output()
			if err == nil {
				if totalWidth, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && totalWidth > 0 {
					target := totalWidth / 4
					if target > 0 && m.width != target {
						exec.Command("tmux", "resize-pane", "-t", pane,
							"-x", strconv.Itoa(target)).Run() //nolint:errcheck
					}
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		// ── Normal bindings ──────────────────────────────────────────────────
		switch msg.String() {
		case "ctrl+c":
			exec.Command("tmux", "kill-session", "-t", "orcai").Run() //nolint:errcheck
			return m, tea.Quit

		case "j", "down":
			if m.cursor < len(m.windows)-1 {
				m.cursor++
			}

		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}

		case "enter":
			if len(m.windows) > 0 {
				w := m.windows[m.cursor]
				target := fmt.Sprintf("orcai:%d", w.Index)
				exec.Command("tmux", "select-window", "-t", target).Run()          //nolint:errcheck
				exec.Command("tmux", "select-pane", "-t", target+".1").Run()       //nolint:errcheck
			}

		case "x":
			if len(m.windows) > 0 {
				w := m.windows[m.cursor]
				exec.Command("tmux", "kill-window", "-t",
					fmt.Sprintf("orcai:%d", w.Index)).Run() //nolint:errcheck
				m.windows = listWindows()
				if m.cursor >= len(m.windows) && m.cursor > 0 {
					m.cursor = len(m.windows) - 1
				}
			}

		case "n":
			if m.self != "" {
				exec.Command("tmux", "display-popup", "-E",
					"-w", "80%", "-h", "80%", m.self, "_picker").Run() //nolint:errcheck
			}
			m.windows = listWindows()
			m.ensureSidebars()
			if m.cursor >= len(m.windows) && m.cursor > 0 {
				m.cursor = len(m.windows) - 1
			}
		}
	}

	return m, nil
}

// ANSI colour constants (Dracula palette + teal accent, 256-colour).
const (
	aTeal  = "\x1b[38;5;87m"   // bright teal — banner lines & connectors
	aDimT  = "\x1b[38;5;66m"   // dim teal — dots row & dividers
	aPink  = "\x1b[38;5;212m"  // pink — block chars & active name
	aBold  = "\x1b[1;38;5;212m" // bold pink — ORCAI logo text
	aBlue  = "\x1b[38;5;61m"   // muted blue — inactive names & footer
	aSelBg = "\x1b[48;5;236m"  // dark selection background
	aReset = "\x1b[0m"
)

// buildSidebarView renders the sidebar using an open BBS-style layout:
//
//	  ▪                      ▪   ← dim teal dots above T-junctions
//	══╢  ░▒▓ ORCAI ▓▒░  ╟══     ← teal banner with pink logo
//	──────────────────────────   ← dim divider
//	▎ session-name               ← active: pink on dark bg
//	  session-name               ← inactive: muted blue
//	──────────────────────────   ← dim divider
//	n new  x kill  ↑↓ nav        ← footer
func buildSidebarView(width int, windows []Window, cursor int) string {
	if width < 10 {
		width = 22
	}

	pad := func(n int) string {
		if n <= 0 {
			return ""
		}
		return strings.Repeat(" ", n)
	}
	truncate := func(name string, maxCols int) string {
		runes := []rune(name)
		if len(runes) <= maxCols {
			return name
		}
		if maxCols <= 1 {
			return "…"
		}
		return string(runes[:maxCols-1]) + "…"
	}

	// ── Banner ────────────────────────────────────────────────────────────────
	// Center section between T-junctions: "╢ ░▒▓ ORCAI ▓▒░ ╟" = 18 visible chars.
	const centerLen = 18
	sideLen := max((width-centerLen)/2, 0)
	rightLen := max(width-centerLen-sideLen, 0)

	// Dots row: ▪ above each T-junction.
	var dotsRow strings.Builder
	for i := 0; i < width; i++ {
		switch i {
		case sideLen, sideLen + centerLen - 1:
			dotsRow.WriteString(aDimT + "▪" + aReset)
		default:
			dotsRow.WriteByte(' ')
		}
	}

	bannerRow := aTeal + strings.Repeat("═", sideLen) + "╢" +
		" " + aPink + "░▒▓ " + aBold + "ORCAI" + aReset + aPink + " ▓▒░" + aReset +
		aTeal + " ╟" + strings.Repeat("═", rightLen) + aReset

	divider := aDimT + strings.Repeat("─", width) + aReset

	rows := []string{dotsRow.String(), bannerRow, divider}

	// ── Session list ──────────────────────────────────────────────────────────
	if len(windows) == 0 {
		rows = append(rows, aBlue+"  no sessions yet"+aReset)
	} else {
		maxName := max(width-3, 1) // reserve 2 for "▎ " / "  " prefix + 1 spare
		for i, w := range windows {
			name := truncate(w.Name, maxName)
			nameLen := len([]rune(name))
			trailing := max(width-2-nameLen, 0)
			var line string
			if i == cursor {
				// Extend selection background to full width.
				line = aSelBg + aPink + "▎ " + name + pad(trailing) + aReset
			} else {
				line = aBlue + "  " + name + aReset
			}
			rows = append(rows, line)
		}
	}

	// ── Footer ────────────────────────────────────────────────────────────────
	const footerText = "n new  x kill  ↑↓ nav"
	rows = append(rows, divider, aBlue+footerText+aReset)

	return strings.Join(rows, "\n")
}

// View renders the vertical tab manager as a full ANSI box.
func (m Model) View() string {
	w := m.width
	if w <= 0 {
		w = 22
	}
	return buildSidebarView(w, m.windows, m.cursor)
}

// Run starts the sidebar as a bubbletea program.
func Run() {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("sidebar error: %v\n", err)
	}
}
