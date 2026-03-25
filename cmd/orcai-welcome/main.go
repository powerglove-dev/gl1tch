// Binary orcai-welcome is the ABS welcome dashboard widget.
//
// It is launched via `tmux new-window` (or the orcai _welcome shim) and
// connects to the busd Unix socket to receive live theme / session events.
// When the user presses any key it replaces itself with $SHELL via
// syscall.Exec, so the tmux window gets a proper interactive shell.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/adam-stokes/orcai/internal/busd"
)

// ── Bus protocol ───────────────────────────────────────────────────────────────

const (
	busDialTimeout = 2 * time.Second
	busWidgetName  = "orcai-welcome"
)

var busSubscriptions = []string{
	"theme.changed",
	"session.started",
	"session.ended",
	"orcai.telemetry",
}

// registrationFrame is sent to the bus daemon on connect.
type registrationFrame struct {
	Name      string   `json:"name"`
	Subscribe []string `json:"subscribe"`
}

// busEvent is a decoded server-to-client frame.
type busEvent struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}

// themeChangedPayload is the payload for the "theme.changed" event.
type themeChangedPayload struct {
	Name string `json:"name"`
}

// ── Tea messages ───────────────────────────────────────────────────────────────

// themeChangedMsg is sent to the BubbleTea program when the active theme changes.
type themeChangedMsg struct {
	ThemeName string
}

// ── ANSI palette ───────────────────────────────────────────────────────────────

// palette holds the ANSI escape sequences used to render the welcome screen.
// The ABS/Dracula defaults are set at initialisation; they update when a
// theme.changed event is received.
type palette struct {
	purple string // \x1b[38;5;141m
	pink   string // \x1b[38;5;212m
	bold   string // \x1b[1;38;5;212m
	blue   string // \x1b[38;5;61m
	dim    string // \x1b[38;5;66m
	reset  string // \x1b[0m
}

// absDefaults returns the ABS/Dracula default palette.
func absDefaults() palette {
	return palette{
		purple: "\x1b[38;5;141m",
		pink:   "\x1b[38;5;212m",
		bold:   "\x1b[1;38;5;212m",
		blue:   "\x1b[38;5;61m",
		dim:    "\x1b[38;5;66m",
		reset:  "\x1b[0m",
	}
}

// paletteForTheme returns a palette for the given theme name.
// Only "abs" (and the empty string / no event yet) produces distinct colors;
// all other themes fall back to the ABS defaults as a forward-compat placeholder.
func paletteForTheme(name string) palette {
	switch name {
	case "abs", "":
		return absDefaults()
	default:
		// Unknown theme — keep the ABS defaults so the widget remains readable.
		return absDefaults()
	}
}

// ── Banner / Help ───────────────────────────────────────────────────────────────

func buildWelcomeArt(width int, p palette) string {
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

	top := p.purple + "╔" + strings.Repeat("═", inner) + "╗" + p.reset

	const logoPrefixLen = 37
	logoLine := p.purple + "║" + p.pink + " ░▒▓ " + p.bold + "O R C A I" + p.reset +
		p.pink + " ▓▒░" + p.blue + "  Your AI Workspace" + pad(inner-logoPrefixLen) +
		p.purple + "║" + p.reset

	const subtitlePrefixLen = 38
	subtitleLine := p.purple + "║" + p.blue + "      tmux · AI agents · open sessions" +
		pad(inner-subtitlePrefixLen) + p.purple + "║" + p.reset

	mid := p.purple + "╠" + strings.Repeat("═", inner) + "╣" + p.reset

	scanContent := strings.Repeat("▄▀", inner/2)
	if inner%2 == 1 {
		scanContent += "▄"
	}
	scanLine := p.purple + "║" + p.pink + scanContent + p.purple + "║" + p.reset

	bot := p.purple + "╚" + strings.Repeat("═", inner) + "╝" + p.reset

	return strings.Join([]string{top, logoLine, subtitleLine, mid, scanLine, bot}, "\n")
}

func buildHelp(width int, p palette) string {
	col := p.dim + strings.Repeat("─", width) + p.reset

	lines := []string{
		col,
		"",
		p.blue + "  Press  " + p.pink + "ctrl+space" + p.blue + "  to open the chord menu from anywhere." + p.reset,
		"",
		p.blue + "    " + p.pink + "n" + p.dim + "  new session   " + p.blue + "(pick AI provider + model)" + p.reset,
		p.blue + "    " + p.pink + "t" + p.dim + "  sysop panel   " + p.blue + "(agent monitor in current window)" + p.reset,
		p.blue + "    " + p.pink + "p" + p.dim + "  prompt builder" + p.blue + p.reset,
		p.blue + "    " + p.pink + "q" + p.dim + "  quit ORCAI" + p.reset,
		p.blue + "    " + p.pink + "d" + p.dim + "  detach        " + p.blue + "(reconnect later: orcai)" + p.reset,
		"",
		col,
		"",
		p.dim + "  ── enter new session · any key continue ──" + p.reset,
	}
	return strings.Join(lines, "\n")
}

// ── BubbleTea model ────────────────────────────────────────────────────────────

type model struct {
	width   int
	height  int
	self    string
	palette palette
}

func newModel() model {
	self, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	return model{
		self:    self,
		palette: absDefaults(),
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case themeChangedMsg:
		m.palette = paletteForTheme(msg.ThemeName)
	case tea.KeyMsg:
		if msg.String() == "enter" && m.self != "" {
			self := m.self
			return m, tea.Batch(
				func() tea.Msg {
					exec.Command("tmux", "display-popup", "-E",
						"-w", "120", "-h", "40", self, "_picker").Run() //nolint:errcheck
					return nil
				},
				tea.Quit,
			)
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	return buildWelcomeArt(w, m.palette) + "\n" + buildHelp(w, m.palette)
}

// ── Bus connection ─────────────────────────────────────────────────────────────

// connectBus dials the busd socket and sends the registration frame. Returns
// the connection, or nil if the daemon is not running (non-fatal).
func connectBus() net.Conn {
	sockPath, err := busd.SocketPath()
	if err != nil {
		return nil
	}

	conn, err := net.DialTimeout("unix", sockPath, busDialTimeout)
	if err != nil {
		// Bus daemon not running — proceed without it.
		return nil
	}

	reg := registrationFrame{
		Name:      busWidgetName,
		Subscribe: busSubscriptions,
	}
	data, _ := json.Marshal(reg)
	data = append(data, '\n')
	conn.SetWriteDeadline(time.Now().Add(busDialTimeout)) //nolint:errcheck
	conn.Write(data)                                      //nolint:errcheck
	conn.SetWriteDeadline(time.Time{})                    //nolint:errcheck

	return conn
}

// readBusEvents reads newline-delimited JSON frames from conn and forwards
// relevant messages to the BubbleTea program p. It runs until conn is closed.
func readBusEvents(conn net.Conn, p *tea.Program) {
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		var ev busEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		switch ev.Event {
		case "theme.changed":
			var pl themeChangedPayload
			if err := json.Unmarshal(ev.Payload, &pl); err == nil {
				p.Send(themeChangedMsg{ThemeName: pl.Name})
			}
		// session.started, session.ended, orcai.telemetry — consume and discard
		}
	}
}

// ── Entry point ────────────────────────────────────────────────────────────────

func main() {
	// Connect to bus (non-fatal if not running).
	conn := connectBus()

	p := tea.NewProgram(newModel(), tea.WithAltScreen())

	// Start bus event reader goroutine (only if we have a connection).
	if conn != nil {
		go readBusEvents(conn, p)
		defer conn.Close()
	}

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "orcai-welcome: %v\n", err)
	}

	execShell()
}

func execShell() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	if err := syscall.Exec(shell, []string{shell}, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "orcai-welcome: exec shell: %v\n", err)
		os.Exit(1)
	}
}
