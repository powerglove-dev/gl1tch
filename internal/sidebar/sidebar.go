package sidebar

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/adam-stokes/orcai/proto/orcai/v1"
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
	Status       string // "streaming" | "done" | "error"
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

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Model is the bubbletea agent context panel model.
type Model struct {
	windows  []Window
	cursor   int
	width    int
	height   int
	sessions map[string]SessionTelemetry // keyed by session_id
	busConn  *grpc.ClientConn
}

// NewWithWindows creates a Model with a fixed window list — used in tests.
func NewWithWindows(windows []Window) Model {
	return Model{windows: windows, sessions: make(map[string]SessionTelemetry)}
}

// Cursor returns the current cursor position — used in tests.
func (m Model) Cursor() int { return m.cursor }

// busAddrPath returns the path to the bus address file.
func busAddrPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "orcai", "bus.addr"), nil
}

// readBusAddr reads the bus address with up to 3 seconds of retry.
func readBusAddr() string {
	path, err := busAddrPath()
	if err != nil {
		return ""
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return strings.TrimSpace(string(data))
		}
		time.Sleep(250 * time.Millisecond)
	}
	return ""
}

// subscribeCmd connects to the bus and returns a tea.Cmd that emits TelemetryMsg values.
func subscribeCmd(conn *grpc.ClientConn) tea.Cmd {
	return func() tea.Msg {
		client := pb.NewEventBusClient(conn)
		stream, err := client.Subscribe(context.Background(), &pb.SubscribeRequest{
			Topics: []string{"orcai.telemetry"},
		})
		if err != nil {
			return nil
		}
		evt, err := stream.Recv()
		if err != nil {
			return nil
		}
		var payload struct {
			SessionID    string  `json:"session_id"`
			WindowName   string  `json:"window_name"`
			Provider     string  `json:"provider"`
			Status       string  `json:"status"`
			InputTokens  int     `json:"input_tokens"`
			OutputTokens int     `json:"output_tokens"`
			CostUSD      float64 `json:"cost_usd"`
		}
		if err := json.Unmarshal([]byte(evt.Payload), &payload); err != nil {
			return nil
		}
		return TelemetryMsg{
			SessionID:    payload.SessionID,
			WindowName:   payload.WindowName,
			Provider:     payload.Provider,
			Status:       payload.Status,
			InputTokens:  payload.InputTokens,
			OutputTokens: payload.OutputTokens,
			CostUSD:      payload.CostUSD,
		}
	}
}

// New creates the sidebar model and connects to the event bus if available.
func New() Model {
	m := Model{
		windows:  listWindows(),
		sessions: make(map[string]SessionTelemetry),
	}
	if addr := readBusAddr(); addr != "" {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			m.busConn = conn
		}
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd()}
	if m.busConn != nil {
		cmds = append(cmds, subscribeCmd(m.busConn))
	}
	return tea.Batch(cmds...)
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
		var next tea.Cmd
		if m.busConn != nil {
			next = subscribeCmd(m.busConn)
		}
		return m, next

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
		switch msg.String() {
		case "ctrl+c":
			if m.busConn != nil {
				m.busConn.Close() //nolint:errcheck
			}
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

// ANSI colour constants (Dracula palette, 256-colour).
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

func (m Model) View() string {
	w := m.width
	if w <= 0 {
		w = 28
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
	const centerLen = 18
	sideLen := max((w-centerLen)/2, 0)
	rightLen := max(w-centerLen-sideLen, 0)

	var dotsRow strings.Builder
	for i := 0; i < w; i++ {
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

	divider := aDimT + strings.Repeat("─", w) + aReset

	rows := []string{dotsRow.String(), bannerRow, divider}

	// ── Session list with telemetry overlay ───────────────────────────────────
	byName := make(map[string]SessionTelemetry)
	for _, st := range m.sessions {
		byName[st.WindowName] = st
	}

	if len(m.windows) == 0 {
		rows = append(rows, aBlue+"  no sessions yet"+aReset)
	} else {
		maxName := max(w-3, 1)
		for i, win := range m.windows {
			name := truncate(win.Name, maxName)
			nameLen := len([]rune(name))
			trailing := max(w-2-nameLen, 0)

			var nameLine string
			if i == m.cursor {
				nameLine = aSelBg + aPink + "▎ " + name + pad(trailing) + aReset
			} else {
				nameLine = aBlue + "  " + name + aReset
			}
			rows = append(rows, nameLine)

			if st, ok := byName[win.Name]; ok {
				statusIcon := aGreen + "●" + aReset
				statusLabel := "running"
				if st.Status == "done" {
					statusIcon = aDimT + "○" + aReset
					statusLabel = "idle   "
				}
				var telLine string
				if st.InputTokens > 0 {
					telLine = fmt.Sprintf("  %s %s %dk↑ %d↓ $%.3f",
						statusIcon, statusLabel,
						st.InputTokens/1000, st.OutputTokens, st.CostUSD)
				} else {
					telLine = fmt.Sprintf("  %s %s", statusIcon, statusLabel)
				}
				rows = append(rows, aYellow+telLine+aReset)
			} else {
				rows = append(rows, aDimT+"  no data"+aReset)
			}
		}
	}

	// ── Footer ────────────────────────────────────────────────────────────────
	rows = append(rows, divider, aBlue+"enter focus  x kill  ↑↓ nav"+aReset)

	return strings.Join(rows, "\n")
}

// Run starts the sidebar as a bubbletea program.
func Run() {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("sidebar error: %v\n", err)
	}
}

// ── Sidebar toggle ────────────────────────────────────────────────────────────

func sidebarVisiblePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "orcai", ".sidebar-visible"), nil
}

func isSidebarVisible() bool {
	path, err := sidebarVisiblePath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "true"
}

func setSidebarVisible(visible bool) {
	path, err := sidebarVisiblePath()
	if err != nil {
		return
	}
	val := "false"
	if visible {
		val = "true"
	}
	os.WriteFile(path, []byte(val), 0o644) //nolint:errcheck
}

// RunToggle shows or hides the sidebar pane based on the current marker file state.
func RunToggle() {
	self, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}

	if isSidebarVisible() {
		exec.Command("tmux", "kill-pane", "-t", ".0").Run() //nolint:errcheck
		setSidebarVisible(false)
	} else {
		exec.Command("tmux", "split-window",
			"-d", "-h", "-b", "-f", "-l", "25%",
			"-t", "orcai:0", self, "_sidebar").Run() //nolint:errcheck
		setSidebarVisible(true)
	}
}
