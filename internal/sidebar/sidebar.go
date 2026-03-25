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

// SessionTelemetry holds live telemetry for one session window.
type SessionTelemetry struct {
	WindowName   string
	Provider     string
	Status       string // "streaming" | "done"
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// TelemetryMsg carries a parsed telemetry event from the bus.
type TelemetryMsg struct {
	SessionID    string
	WindowName   string
	Provider     string
	Status       string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// logEntry records a single telemetry event for the activity log.
type logEntry struct {
	At         time.Time
	Node       int // 1-based node number at time of event
	WindowName string
	Event      string // "streaming" | "done"
	CostUSD    float64
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Model is the bubbletea BBS sysop panel model.
type Model struct {
	windows  []Window
	cursor   int
	width    int
	height   int
	sessions map[string]SessionTelemetry // keyed by session_id
	log      []logEntry                  // activity log, newest-first, capped at 12
}

// NewWithWindows creates a Model with a fixed window list — used in tests.
func NewWithWindows(windows []Window) Model {
	return Model{
		windows:  windows,
		sessions: make(map[string]SessionTelemetry),
		log:      []logEntry{},
	}
}

// Cursor returns the current cursor position — used in tests.
func (m Model) Cursor() int { return m.cursor }

// nodeIndexFor returns the 1-based node number for a window name, or 0 if not found.
func (m Model) nodeIndexFor(windowName string) int {
	for i, w := range m.windows {
		if w.Name == windowName {
			return i + 1
		}
	}
	return 0
}

// New creates the sidebar model.
func New() Model {
	return Model{
		windows:  listWindows(),
		sessions: make(map[string]SessionTelemetry),
		log:      []logEntry{},
	}
}

func (m Model) Init() tea.Cmd {
	return tickCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TelemetryMsg:
		m.sessions[msg.SessionID] = SessionTelemetry{
			WindowName:   msg.WindowName,
			Provider:     msg.Provider,
			Status:       msg.Status,
			InputTokens:  msg.InputTokens,
			OutputTokens: msg.OutputTokens,
			CostUSD:      msg.CostUSD,
		}
		// Prepend to activity log and cap at 12.
		node := m.nodeIndexFor(msg.WindowName)
		entry := logEntry{
			At:         time.Now(),
			Node:       node,
			WindowName: msg.WindowName,
			Event:      msg.Status,
			CostUSD:    msg.CostUSD,
		}
		m.log = append([]logEntry{entry}, m.log...)
		if len(m.log) > 12 {
			m.log = m.log[:12]
		}
		return m, nil

	case tickMsg:
		m.windows = listWindows()
		return m, tickCmd()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if pane := os.Getenv("TMUX_PANE"); pane != "" {
			out, err := exec.Command("tmux", "display-message", "-p", "#{window_width}").Output()
			if err == nil {
				if totalWidth, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && totalWidth > 0 {
					target := totalWidth * 3 / 10 // 30%
					if target > 0 && m.width != target {
						exec.Command("tmux", "resize-pane", "-t", pane,
							"-x", strconv.Itoa(target)).Run() //nolint:errcheck
					}
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
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
				exec.Command("tmux", "select-window", "-t", target).Run()    //nolint:errcheck
				exec.Command("tmux", "select-pane", "-t", target+".1").Run() //nolint:errcheck
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
		}
	}

	return m, nil
}

// ── ANSI palette ───────────────────────────────────────────────────────────────

const (
	aTeal   = "\x1b[38;5;87m"
	aDimT   = "\x1b[38;5;66m"
	aPink   = "\x1b[38;5;212m"
	aBold   = "\x1b[1;38;5;212m"
	aBlue   = "\x1b[38;5;61m"
	aGreen  = "\x1b[38;5;84m"
	aYellow = "\x1b[38;5;228m"
	aSelBg  = "\x1b[48;5;236m"
	aReset  = "\x1b[0m"
)

// ── View helpers ───────────────────────────────────────────────────────────────

func innerPad(s string, visibleLen, inner int) string {
	pad := inner - visibleLen
	if pad < 0 {
		pad = 0
	}
	return s + strings.Repeat(" ", pad)
}

func borderTop(w int) string {
	return aTeal + "╔" + strings.Repeat("═", w-2) + "╗" + aReset
}

func borderMid(w int) string {
	return aTeal + "╠" + strings.Repeat("═", w-2) + "╣" + aReset
}

func borderThin(w int) string {
	return aDimT + "╠" + strings.Repeat("─", w-2) + "╣" + aReset
}

func borderBot(w int) string {
	return aTeal + "╚" + strings.Repeat("═", w-2) + "╝" + aReset
}

func borderRow(content, colour string, inner int, visLen int) string {
	return aTeal + "║" + colour + innerPad(content, visLen, inner) + aReset + aTeal + "║" + aReset
}

// View renders the sidebar using an ABS-style (Agentic Bulletin System) layout.
func (m Model) View() string {
	w := m.width
	if w <= 0 {
		w = 32
	}
	inner := w - 2

	// ── Header ────────────────────────────────────────────────────────────────
	title := " ▒▒▒ ORCAI SYSOP MONITOR ▒▒▒"
	titleVis := len([]rune(title))
	subLine := fmt.Sprintf(" NODES: %d ACTIVE  %s", len(m.windows), time.Now().Format("15:04"))
	subVis := len([]rune(subLine))

	rows := []string{
		borderTop(w),
		borderRow(title, aBold, inner, titleVis),
		borderRow(subLine, aBlue, inner, subVis),
		borderMid(w),
	}

	// ── Node sections ─────────────────────────────────────────────────────────
	byName := make(map[string]SessionTelemetry)
	for _, st := range m.sessions {
		byName[st.WindowName] = st
	}

	if len(m.windows) == 0 {
		rows = append(rows, borderRow("   no nodes active", aDimT, inner, len("   no nodes active")))
	} else {
		for i, win := range m.windows {
			nodeNum := i + 1
			nodeLabel := fmt.Sprintf("NODE %02d", nodeNum)

			st, hasTel := byName[win.Name]

			// Status badge
			var badge, badgeColour string
			if !hasTel {
				badge = "[WAIT]"
				badgeColour = aYellow
			} else if st.Status == "streaming" {
				badge = "[BUSY]"
				badgeColour = aGreen
			} else {
				badge = "[IDLE]"
				badgeColour = aDimT
			}

			// Node header line — highlighted when cursor is here
			headerContent := " " + nodeLabel + " " + badge
			headerVis := 1 + len(nodeLabel) + 1 + len(badge)
			if i == m.cursor {
				rows = append(rows, aTeal+"║"+aSelBg+aPink+innerPad(headerContent, headerVis, inner)+aReset+aTeal+"║"+aReset)
			} else {
				rows = append(rows,
					aTeal+"║"+aPink+nodeLabel+" "+badgeColour+badge+
						strings.Repeat(" ", max(inner-headerVis, 0))+
						aReset+aTeal+"║"+aReset)
			}

			// Name + provider line
			provLine := "   " + win.Name
			if hasTel && st.Provider != "" {
				provLine = "   " + win.Name + "  " + st.Provider
			}
			rows = append(rows, borderRow(provLine, aTeal, inner, len([]rune(provLine))))

			// Metrics line
			var metricsLine string
			var metricsVis int
			if hasTel && st.InputTokens > 0 {
				metricsLine = fmt.Sprintf("   %dk↑ %d↓  $%.3f", st.InputTokens/1000, st.OutputTokens, st.CostUSD)
				metricsVis = len([]rune(metricsLine))
				rows = append(rows, borderRow(metricsLine, aYellow, inner, metricsVis))
			} else {
				metricsLine = "   no data"
				rows = append(rows, borderRow(metricsLine, aDimT, inner, len(metricsLine)))
			}

			// Divider between nodes (not after last)
			if i < len(m.windows)-1 {
				rows = append(rows, borderMid(w))
			}
		}
	}

	// ── Activity log ──────────────────────────────────────────────────────────
	rows = append(rows, borderThin(w))

	logHeader := " ── ACTIVITY LOG ──"
	rows = append(rows, borderRow(logHeader, aDimT, inner, len([]rune(logHeader))))

	if len(m.log) == 0 {
		rows = append(rows, borderRow("  no activity", aDimT, inner, len("  no activity")))
	} else {
		for _, entry := range m.log {
			var line string
			if entry.Event == "done" && entry.CostUSD > 0 {
				line = fmt.Sprintf("  %s NODE%02d done $%.3f",
					entry.At.Format("15:04"), entry.Node, entry.CostUSD)
			} else {
				line = fmt.Sprintf("  %s NODE%02d %s",
					entry.At.Format("15:04"), entry.Node, entry.Event)
			}
			rows = append(rows, borderRow(line, aDimT, inner, len([]rune(line))))
		}
	}

	// ── Footer ────────────────────────────────────────────────────────────────
	footer := " enter focus · x kill · ↑↓ nav"
	rows = append(rows,
		borderRow(footer, aBlue, inner, len([]rune(footer))),
		borderBot(w),
	)

	return strings.Join(rows, "\n")
}

// Run starts the sysop panel as a bubbletea program.
func Run() {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("sidebar error: %v\n", err)
	}
}

// ── Panel toggle ───────────────────────────────────────────────────────────────

// resolveSysopBin returns the path to the orcai-sysop binary.
// It checks PATH first, then falls back to the same directory as the caller.
func resolveSysopBin() string {
	if bin, err := exec.LookPath("orcai-sysop"); err == nil {
		return bin
	}
	self, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	return filepath.Join(filepath.Dir(self), "orcai-sysop")
}

// RunToggle opens the sysop panel as a tmux popup.
func RunToggle() {
	bin := resolveSysopBin()
	exec.Command("tmux", "display-popup", "-E", "-w", "120", "-h", "40", bin).Run() //nolint:errcheck
}
