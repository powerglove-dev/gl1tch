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

// ── ANSI palette — ABS/Dracula BBS aesthetic ───────────────────────────────
const (
	aBC    = "\x1b[36m"       // cyan — borders and general text
	aBrC   = "\x1b[96m"       // bright cyan — section headers and key labels
	aDim   = "\x1b[38;5;66m"  // dim teal — secondary text, [IDLE] badge
	aWht   = "\x1b[97m"       // bright white — selected row text
	aSelBg = "\x1b[48;5;235m" // dark background — cursor row highlight
	aGrn   = "\x1b[92m"       // bright green — [BUSY] badge
	aYlw   = "\x1b[93m"       // bright yellow — [WAIT] badge
	aReset = "\x1b[0m"
)

// ── View helpers ───────────────────────────────────────────────────────────────

// visLen returns the number of visible (non-ANSI-escape) runes in s.
func visLen(s string) int {
	n, esc := 0, false
	for _, r := range s {
		if r == '\x1b' {
			esc = true
			continue
		}
		if esc {
			if r == 'm' {
				esc = false
			}
			continue
		}
		n++
	}
	return n
}

// padToVis right-pads s with spaces until its visible length equals w.
func padToVis(s string, w int) string {
	vl := visLen(s)
	if vl >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vl)
}

// boxTop renders the top edge of a box. If title is non-empty it is inset
// into the border: ┌─── Title ───┐
func boxTop(w int, title string) string {
	if title == "" {
		return aBC + "┌" + strings.Repeat("─", w-2) + "┐" + aReset
	}
	label := " " + title + " "
	dashes := max(w-2-len(label), 0)
	left := dashes / 2
	right := dashes - left
	return aBC + "┌" + strings.Repeat("─", left) + aBrC + label + aBC + strings.Repeat("─", right) + "┐" + aReset
}

// boxBot renders the bottom edge of a box.
func boxBot(w int) string {
	return aBC + "└" + strings.Repeat("─", w-2) + "┘" + aReset
}

// boxRow renders one content row inside a box, padded to width w.
// content is an ANSI string; contentVis is its visible length.
func boxRow(content string, contentVis, w int) string {
	inner := w - 2
	pad := max(inner-contentVis, 0)
	return aBC + "│" + aReset + content + strings.Repeat(" ", pad) + aBC + "│" + aReset
}

// sideBySide merges three equal-height column slices into one slice,
// padding each column to colW visible characters with gap spaces between.
func sideBySide(left, mid, right []string, colW, gap int) []string {
	h := max(max(len(left), len(mid)), len(right))
	sp := strings.Repeat(" ", gap)
	rows := make([]string, h)
	for i := range h {
		l, m, r := "", "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(mid) {
			m = mid[i]
		}
		if i < len(right) {
			r = right[i]
		}
		rows[i] = padToVis(l, colW) + sp + padToVis(m, colW) + sp + r
	}
	return rows
}

// sessionForWindow returns the SessionTelemetry for the given window name
// (searching by WindowName across all session entries) and whether one exists.
func (m Model) sessionForWindow(windowName string) (SessionTelemetry, bool) {
	for _, st := range m.sessions {
		if st.WindowName == windowName {
			return st, true
		}
	}
	return SessionTelemetry{}, false
}

// buildNodesColumn renders the "Active Nodes" column as a slice of lines of width w.
func (m Model) buildNodesColumn(w int) []string {
	inner := w - 2
	rows := []string{boxTop(w, "Active Nodes")}

	if len(m.windows) == 0 {
		nodesLine := aDim + "  no active nodes" + aReset
		rows = append(rows, boxRow(nodesLine, 18, w))
	}

	for i, win := range m.windows {
		st, hasTel := m.sessionForWindow(win.Name)

		var badge, badgeCol string
		switch {
		case !hasTel:
			badge, badgeCol = "[WAIT]", aYlw
		case st.Status == "streaming":
			badge, badgeCol = "[BUSY]", aGrn
		default:
			badge, badgeCol = "[IDLE]", aDim
		}

		keyLabel := fmt.Sprintf("[%d]", i+1)
		maxName := max(inner-len(keyLabel)-2-len(badge), 1)
		name := win.Name
		if len(name) > maxName {
			name = name[:maxName-1] + "…"
		}
		dotCount := max(inner-len(keyLabel)-1-len(name)-1-len(badge), 1)

		contentVis := len(keyLabel) + 1 + len(name) + dotCount + len(badge)

		if i == m.cursor {
			content := aSelBg + aBrC + keyLabel + " " + aWht + name +
				aDim + strings.Repeat(".", dotCount) +
				badgeCol + badge + aReset
			rows = append(rows, aBC+"│"+content+strings.Repeat(" ", max(inner-contentVis, 0))+aBC+"│"+aReset)
		} else {
			content := aBrC + keyLabel + " " + aBC + name +
				aDim + strings.Repeat(".", dotCount) +
				badgeCol + badge + aReset
			rows = append(rows, boxRow(content, contentVis, w))
		}
	}

	rows = append(rows, boxBot(w))
	return rows
}

// buildDetailsColumn renders the "Node Details" column for the cursor node.
func (m Model) buildDetailsColumn(w int) []string {
	rows := []string{boxTop(w, "Node Details")}

	if len(m.windows) == 0 || m.cursor >= len(m.windows) {
		rows = append(rows, boxRow(aDim+"  no node selected"+aReset, 18, w))
		rows = append(rows, boxBot(w))
		return rows
	}

	win := m.windows[m.cursor]
	st, hasTel := m.sessionForWindow(win.Name)

	field := func(label, value string) string {
		dots := max(12-len(label), 1)
		line := "  " + aBrC + label + " " + aDim + strings.Repeat(".", dots) + " " + aBC + value + aReset
		return boxRow(line, 2+len(label)+1+dots+1+len(value), w)
	}

	rows = append(rows, field("Window", win.Name))

	if hasTel {
		rows = append(rows, field("Provider", st.Provider))

		var statusVal string
		if st.Status == "streaming" {
			statusVal = aGrn + "streaming" + aReset
		} else {
			statusVal = aDim + st.Status + aReset
		}
		statusLine := "  " + aBrC + "Status" + " " + aDim + strings.Repeat(".", 7) + " " + statusVal + aReset
		rows = append(rows, boxRow(statusLine, 2+6+1+7+1+len(st.Status), w))

		if st.InputTokens > 0 {
			tokens := fmt.Sprintf("%dk↑ / %d↓", st.InputTokens/1000, st.OutputTokens)
			rows = append(rows, field("Tokens", tokens))
			cost := fmt.Sprintf("$%.4f", st.CostUSD)
			rows = append(rows, field("Cost", cost))
		}
	} else {
		rows = append(rows, boxRow(aDim+"  no telemetry yet"+aReset, 18, w))
	}

	rows = append(rows, boxBot(w))
	return rows
}

// buildLogColumn renders the "ACTIVITY LOG" column.
func (m Model) buildLogColumn(w int) []string {
	rows := []string{boxTop(w, "ACTIVITY LOG")}

	if len(m.log) == 0 {
		rows = append(rows, boxRow(aDim+"  no activity"+aReset, 13, w))
	}

	for _, entry := range m.log {
		nodeLabel := fmt.Sprintf("NODE%02d", entry.Node)
		var line string
		if entry.Event == "done" && entry.CostUSD > 0 {
			line = fmt.Sprintf("  %s  %s  done  $%.3f",
				entry.At.Format("15:04"), nodeLabel, entry.CostUSD)
		} else {
			line = fmt.Sprintf("  %s  %s  %s",
				entry.At.Format("15:04"), nodeLabel, entry.Event)
		}
		rows = append(rows, boxRow(aDim+line+aReset, len(line), w))
	}

	rows = append(rows, boxBot(w))
	return rows
}

// View renders the sysop panel with a BBS-style three-column layout.
func (m Model) View() string {
	w := m.width
	if w <= 0 {
		w = 120
	}

	gap := 2
	colW := (w - gap*2) / 3

	var lines []string

	// ── Title bar ─────────────────────────────────────────────────────────────
	title := "ABS · SYSOP MONITOR"
	titleVis := len(title)
	pad := max((w-2-titleVis)/2, 0)
	centred := strings.Repeat(" ", pad) + aBrC + title + aReset
	lines = append(lines,
		boxTop(w, ""),
		boxRow(centred, pad+titleVis, w),
		boxBot(w),
		"",
	)

	// ── Three columns ──────────────────────────────────────────────────────────
	left := m.buildNodesColumn(colW)
	mid := m.buildDetailsColumn(colW)
	right := m.buildLogColumn(colW)
	lines = append(lines, sideBySide(left, mid, right, colW, gap)...)
	lines = append(lines, "")

	// ── Actions bar ────────────────────────────────────────────────────────────
	actionsContent := "  " + aBrC + "[enter]" + aBC + " focus    " +
		aBrC + "[x]" + aBC + " kill    " +
		aBrC + "[↑↓]" + aBC + " navigate    " +
		aBrC + "[ctrl+c]" + aBC + " quit" + aReset
	actionsVis := 2 + 7 + 10 + 3 + 9 + 4 + 12 + 8 + 5
	lines = append(lines,
		boxTop(w, ""),
		boxRow(actionsContent, actionsVis, w),
		boxBot(w),
	)

	// ── Status strip ───────────────────────────────────────────────────────────
	var totalCost float64
	var activeProvider string
	var anyStreaming bool
	for _, st := range m.sessions {
		totalCost += st.CostUSD
		if st.Provider != "" && activeProvider == "" {
			activeProvider = st.Provider
		}
		if st.Status == "streaming" {
			anyStreaming = true
		}
	}
	sep := aDim + "  │  " + aReset
	statusParts := []string{
		fmt.Sprintf("%sNODES: %d ACTIVE%s", aBrC, len(m.windows), aReset),
	}
	if anyStreaming {
		statusParts = append(statusParts, aGrn+"STREAMING"+aReset)
	}
	if activeProvider != "" {
		statusParts = append(statusParts, aBC+strings.ToUpper(activeProvider)+aReset)
	}
	if totalCost > 0 {
		statusParts = append(statusParts, aBC+fmt.Sprintf("$%.3f TOTAL", totalCost)+aReset)
	}
	statusParts = append(statusParts, aDim+time.Now().Format("15:04")+aReset)
	lines = append(lines, strings.Join(statusParts, sep))

	return strings.Join(lines, "\n")
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
