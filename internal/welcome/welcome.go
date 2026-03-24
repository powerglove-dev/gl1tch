// Package welcome implements the live agent dashboard shown in window 0.
// It subscribes to the orcai.telemetry bus and displays per-session cards
// with provider, status, token counts, and cost. Enter opens the provider
// picker; q / ctrl+c quits to $SHELL.
package welcome

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/adam-stokes/orcai/proto/orcai/v1"
)

// ── Telemetry types ────────────────────────────────────────────────────────────

// sessionTelemetry holds live telemetry for one session window.
type sessionTelemetry struct {
	WindowName   string
	Provider     string
	Status       string // "streaming" | "done"
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// telemetryMsg carries a parsed telemetry event from the bus.
type telemetryMsg struct {
	SessionID    string
	WindowName   string
	Provider     string
	Status       string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

type tickMsg time.Time

// ── Bus helpers ────────────────────────────────────────────────────────────────

// connectBus reads ~/.config/orcai/bus.addr with up to 3 seconds of retry
// and returns a connected EventBusClient. Returns nil on failure.
func connectBus() (pb.EventBusClient, *grpc.ClientConn) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	addrPath := filepath.Join(home, ".config", "orcai", "bus.addr")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(addrPath)
		if err == nil && len(strings.TrimSpace(string(data))) > 0 {
			addr := strings.TrimSpace(string(data))
			conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err == nil {
				return pb.NewEventBusClient(conn), conn
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, nil
}

// subscribeCmd connects to the bus and returns a tea.Cmd that emits one telemetryMsg.
func subscribeCmd(client pb.EventBusClient) tea.Cmd {
	if client == nil {
		return nil
	}
	return func() tea.Msg {
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
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			return nil
		}
		return telemetryMsg{
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

func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// ── Window list ────────────────────────────────────────────────────────────────

// listWindowNames returns the names of non-home tmux windows in the orcai session.
func listWindowNames() []string {
	out, err := exec.Command("tmux", "list-windows", "-t", "orcai",
		"-F", "#{window_index} #{window_name}").Output()
	if err != nil {
		return nil
	}
	var names []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil || idx == 0 {
			continue
		}
		names = append(names, parts[1])
	}
	return names
}

// ── ANSI palette ───────────────────────────────────────────────────────────────

const (
	aPurple = "\x1b[38;5;141m"
	aPink   = "\x1b[38;5;212m"
	aBold   = "\x1b[1;38;5;212m"
	aBlue   = "\x1b[38;5;61m"
	aTeal   = "\x1b[38;5;87m"
	aDimT   = "\x1b[38;5;66m"
	aGreen  = "\x1b[38;5;84m"
	aYellow = "\x1b[38;5;228m"
	aReset  = "\x1b[0m"
)

// ── Banner ─────────────────────────────────────────────────────────────────────

// buildWelcomeArt generates the ANSI welcome banner scaled to width columns.
func buildWelcomeArt(width int) string {
	if width < 10 {
		width = 52
	}
	inner := width - 2

	pad := func(n int) string {
		if n <= 0 {
			return ""
		}
		return strings.Repeat(" ", n)
	}

	top := aPurple + "╔" + strings.Repeat("═", inner) + "╗" + aReset

	const logoPrefixLen = 37
	logoLine := aPurple + "║" + aPink + " ░▒▓ " + aBold + "O R C A I" + aReset +
		aPink + " ▓▒░" + aBlue + "  Your AI Workspace" + pad(inner-logoPrefixLen) +
		aPurple + "║" + aReset

	const subtitlePrefixLen = 38
	subtitleLine := aPurple + "║" + aBlue + "      tmux · AI agents · open sessions" +
		pad(inner-subtitlePrefixLen) + aPurple + "║" + aReset

	mid := aPurple + "╠" + strings.Repeat("═", inner) + "╣" + aReset

	scanContent := strings.Repeat("▄▀", inner/2)
	if inner%2 == 1 {
		scanContent += "▄"
	}
	scanLine := aPurple + "║" + aPink + scanContent + aPurple + "║" + aReset

	bot := aPurple + "╚" + strings.Repeat("═", inner) + "╝" + aReset

	return strings.Join([]string{top, logoLine, subtitleLine, mid, scanLine, bot}, "\n")
}

// ── Session card ───────────────────────────────────────────────────────────────

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

// buildSessionCard renders a BBS-style box card for one session window.
// st may be nil when no telemetry has arrived yet.
func buildSessionCard(name string, st *sessionTelemetry, cardWidth int) string {
	inner := max(cardWidth-2, 4)

	pad := func(n int) string {
		if n <= 0 {
			return ""
		}
		return strings.Repeat(" ", n)
	}

	top := aTeal + "╔" + strings.Repeat("═", inner) + "╗" + aReset

	// Name line
	displayName := truncate(name, inner-1)
	nameLen := len([]rune(displayName))
	nameLine := aTeal + "║" + aPink + " " + displayName + pad(inner-1-nameLen) + aTeal + "║" + aReset

	// Metrics line
	var metricsLine string
	if st == nil {
		placeholder := " no data"
		metricsLine = aTeal + "║" + aDimT + placeholder + pad(inner-len(placeholder)) + aTeal + "║" + aReset
	} else {
		statusIcon := aGreen + "●" + aReset
		statusLabel := "streaming"
		if st.Status == "done" {
			statusIcon = aDimT + "○" + aReset
			statusLabel = "idle     "
		}
		provider := truncate(st.Provider, 12)
		metrics := fmt.Sprintf(" %s%s %s %dk↑ %d↓ %s$%.3f%s",
			statusIcon, aYellow,
			provider,
			st.InputTokens/1000, st.OutputTokens,
			aTeal, st.CostUSD, aReset)
		// Visible length (strip ANSI for padding calc)
		visibleLen := 1 + 1 + len(statusLabel) + 1 + len(provider) + 1 +
			len(strconv.Itoa(st.InputTokens/1000)) + 2 +
			len(strconv.Itoa(st.OutputTokens)) + 2 +
			len(fmt.Sprintf("$%.3f", st.CostUSD))
		metricsLine = aTeal + "║" + metrics + pad(inner-visibleLen) + aTeal + "║" + aReset
	}

	bot := aTeal + "╚" + strings.Repeat("═", inner) + "╝" + aReset

	return strings.Join([]string{top, nameLine, metricsLine, bot}, "\n")
}

// ── Totals row ─────────────────────────────────────────────────────────────────

func buildTotalsRow(sessions map[string]sessionTelemetry, width int) string {
	var totalIn, totalOut int
	var totalCost float64
	for _, st := range sessions {
		totalIn += st.InputTokens
		totalOut += st.OutputTokens
		totalCost += st.CostUSD
	}
	inner := width - 2
	content := fmt.Sprintf(" TOTAL  %dk↑ %d↓ $%.3f",
		totalIn/1000, totalOut, totalCost)
	pad := max(inner-len(content), 0)
	return aPurple + "╔" + strings.Repeat("═", inner) + "╗" + aReset + "\n" +
		aPurple + "║" + aYellow + content + strings.Repeat(" ", pad) + aPurple + "║" + aReset + "\n" +
		aPurple + "╚" + strings.Repeat("═", inner) + "╝" + aReset
}

// ── BubbleTea model ────────────────────────────────────────────────────────────

type model struct {
	sessions  map[string]sessionTelemetry
	windows   []string // non-home window names, ordered
	busClient pb.EventBusClient
	busConn   *grpc.ClientConn
	width     int
	height    int
	self      string
}

func newModel() model {
	self, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	busClient, busConn := connectBus()
	return model{
		sessions:  make(map[string]sessionTelemetry),
		windows:   listWindowNames(),
		busClient: busClient,
		busConn:   busConn,
		self:      self,
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd()}
	if sub := subscribeCmd(m.busClient); sub != nil {
		cmds = append(cmds, sub)
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		m.windows = listWindowNames()
		return m, tickCmd()

	case telemetryMsg:
		m.sessions[msg.SessionID] = sessionTelemetry{
			WindowName:   msg.WindowName,
			Provider:     msg.Provider,
			Status:       msg.Status,
			InputTokens:  msg.InputTokens,
			OutputTokens: msg.OutputTokens,
			CostUSD:      msg.CostUSD,
		}
		return m, subscribeCmd(m.busClient)

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.busConn != nil {
				m.busConn.Close() //nolint:errcheck
			}
			return m, tea.Quit
		case "enter":
			if m.self != "" {
				exec.Command("tmux", "display-popup", "-E",
					"-w", "120", "-h", "40", m.self, "_picker").Run() //nolint:errcheck
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	divider := aDimT + strings.Repeat("─", w) + aReset

	var rows []string
	rows = append(rows, buildWelcomeArt(w), divider)

	// ── Session cards ──────────────────────────────────────────────────────────
	if len(m.windows) == 0 {
		rows = append(rows, aBlue+"  no active sessions"+aReset)
	} else {
		// Build a name→telemetry lookup by WindowName field.
		byName := make(map[string]sessionTelemetry)
		for _, st := range m.sessions {
			byName[st.WindowName] = st
		}

		twoCol := w >= 100
		if twoCol {
			cardW := (w - 3) / 2
			for i := 0; i < len(m.windows); i += 2 {
				leftName := m.windows[i]
				leftSt, leftOk := byName[leftName]
				var leftPtr *sessionTelemetry
				if leftOk {
					leftPtr = &leftSt
				}
				leftCard := buildSessionCard(leftName, leftPtr, cardW)

				if i+1 < len(m.windows) {
					rightName := m.windows[i+1]
					rightSt, rightOk := byName[rightName]
					var rightPtr *sessionTelemetry
					if rightOk {
						rightPtr = &rightSt
					}
					rightCard := buildSessionCard(rightName, rightPtr, cardW)
					// Zip lines side by side.
					leftLines := strings.Split(leftCard, "\n")
					rightLines := strings.Split(rightCard, "\n")
					for j := range leftLines {
						r := ""
						if j < len(rightLines) {
							r = rightLines[j]
						}
						rows = append(rows, leftLines[j]+" "+r)
					}
				} else {
					rows = append(rows, strings.Split(leftCard, "\n")...)
				}
			}
		} else {
			for _, name := range m.windows {
				st, ok := byName[name]
				var stPtr *sessionTelemetry
				if ok {
					stPtr = &st
				}
				rows = append(rows, strings.Split(buildSessionCard(name, stPtr, w), "\n")...)
			}
		}
	}

	// ── Totals ─────────────────────────────────────────────────────────────────
	rows = append(rows, divider)
	rows = append(rows, strings.Split(buildTotalsRow(m.sessions, w), "\n")...)

	// ── Footer ─────────────────────────────────────────────────────────────────
	rows = append(rows, divider)
	rows = append(rows, aBlue+"  ^spc n new · ^spc p build · enter new session · q quit"+aReset)

	return strings.Join(rows, "\n")
}

// ── Entry point ────────────────────────────────────────────────────────────────

// Run launches the dashboard TUI. After the user quits (q / ctrl+c) it
// replaces the current process with $SHELL.
func Run() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "welcome: %v\n", err)
	}
	execShell()
}

func execShell() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	if err := syscall.Exec(shell, []string{shell}, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "welcome: exec shell: %v\n", err)
		os.Exit(0)
	}
}
