package assistant

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/8op-org/gl1tch/internal/styles"
	"github.com/8op-org/gl1tch/internal/themes"
	"github.com/8op-org/gl1tch/internal/welcome"
)

// SentinelFile is the path (relative to cfgDir) that marks assistant intro as completed.
const SentinelFile = ".assistant_intro_seen"

// IsFirstRun returns true if the assistant intro has not been shown yet.
func IsFirstRun(cfgDir string) bool {
	_, err := os.Stat(filepath.Join(cfgDir, SentinelFile))
	return os.IsNotExist(err)
}

// MarkSeen writes the sentinel file to indicate assistant intro has been shown.
func MarkSeen(cfgDir string) error {
	f, err := os.Create(filepath.Join(cfgDir, SentinelFile))
	if err != nil {
		return err
	}
	return f.Close()
}

// ── Tea message types ─────────────────────────────────────────────────────────

type streamMsg struct {
	token string
	ch    <-chan string
}

type streamDoneMsg struct{}

type streamErrMsg struct{ err error }

// streamNextToken returns a Cmd that reads one token from ch and emits it.
func streamNextToken(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		token, ok := <-ch
		if !ok {
			return streamDoneMsg{}
		}
		return streamMsg{token: token, ch: ch}
	}
}

// ── Speaker types ─────────────────────────────────────────────────────────────

type speaker int

const (
	speakerGlitch speaker = iota
	speakerUser
	speakerSystem
)

type entry struct {
	who  speaker
	text string
}

// ── Model ─────────────────────────────────────────────────────────────────────

// Model is the GL1TCH assistant BubbleTea model.
type Model struct {
	width, height int
	pal           styles.ANSIPalette

	titleLines   []string
	titleHasANSI bool
	messages     []entry
	viewport     viewport.Model
	viewportReady bool
	input        textinput.Model

	streaming    bool
	streamBuf    string
	backend      Backend
	turns        []Turn // full conversation history for brain save

	cfgDir string
}

// New creates a new assistant Model.
func New(cfgDir string, backend Backend) Model {
	input := textinput.New()
	input.Placeholder = "type a message…"
	input.Prompt = " >> "
	input.CharLimit = 2000
	input.Focus()

	title, titleHasANSI := welcome.RenderTitle()

	m := Model{
		width:        80,
		height:       24,
		titleLines:   title,
		titleHasANSI: titleHasANSI,
		input:        input,
		backend:      backend,
		cfgDir:       cfgDir,
	}

	// Load palette from active theme.
	home, _ := os.UserHomeDir()
	if reg, err := themes.NewRegistry(filepath.Join(home, ".config", "glitch", "themes")); err == nil {
		if bundle := reg.Active(); bundle != nil {
			m.pal = styles.BundleANSI(bundle)
		}
	}

	return m
}

// Turns returns the conversation history (for brain save after Run returns).
func (m Model) Turns() []Turn {
	return m.turns
}

// offlineMsg is sent by Init when no backend is available.
type offlineMsg struct{}

// introMsg is sent by Init to trigger the first-run intro stream.
type introMsg struct{}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	if m.backend == nil {
		return tea.Batch(textinput.Blink, func() tea.Msg { return offlineMsg{} })
	}

	firstRun := IsFirstRun(m.cfgDir)
	MarkSeen(m.cfgDir) //nolint:errcheck

	if firstRun {
		return tea.Batch(textinput.Blink, func() tea.Msg { return introMsg{} })
	}

	return textinput.Blink
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case offlineMsg:
		m.messages = append(m.messages, entry{
			who:  speakerSystem,
			text: "-=[ GLITCH OFFLINE ]=-\n\nno AI provider available.\ninstall ollama (ollama.ai) or configure a provider in ~/.config/glitch/wrappers/\n\npress Esc to close",
		})
		m.updateViewport()

	case introMsg:
		m.streaming = true
		cmds = append(cmds, func() tea.Msg {
			ch, err := m.backend.StreamIntro(context.Background())
			if err != nil {
				return streamErrMsg{err: err}
			}
			return streamNextToken(ch)()
		})

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewportReady = true
		m.resizeViewport()

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.backend == nil {
				break // no provider — disable input
			}
			if m.streaming {
				break // ignore input while streaming
			}
			userText := strings.TrimSpace(m.input.Value())
			if userText == "" {
				break
			}
			m.input.SetValue("")
			m.messages = append(m.messages, entry{who: speakerUser, text: userText})
			m.turns = append(m.turns, Turn{Role: "user", Text: userText})
			m.updateViewport()

			m.streaming = true
			m.streamBuf = ""
			history := make([]Turn, len(m.turns))
			copy(history, m.turns)
			// Remove the last user turn from history passed to backend
			// (we pass it separately as userMsg).
			if len(history) > 0 && history[len(history)-1].Role == "user" {
				history = history[:len(history)-1]
			}
			cmds = append(cmds, func() tea.Msg {
				ch, err := m.backend.Stream(context.Background(), history, userText)
				if err != nil {
					return streamErrMsg{err: err}
				}
				return streamNextToken(ch)()
			})
		}

	case streamMsg:
		m.streamBuf += msg.token
		m.upsertStreamEntry()
		m.updateViewport()
		cmds = append(cmds, streamNextToken(msg.ch))

	case streamDoneMsg:
		m.streaming = false
		if m.streamBuf != "" {
			m.upsertStreamEntry()
			// Add completed assistant turn to history.
			m.turns = append(m.turns, Turn{Role: "assistant", Text: m.streamBuf})
		}
		m.streamBuf = ""
		m.updateViewport()
		// Do NOT quit — free-form chat continues.

	case streamErrMsg:
		m.streaming = false
		m.streamBuf = ""
		m.messages = append(m.messages, entry{
			who:  speakerSystem,
			text: fmt.Sprintf("stream error: %v", msg.err),
		})
		m.updateViewport()
	}

	// Update sub-models.
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	cmds = append(cmds, inputCmd)

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// upsertStreamEntry updates the last glitch entry with streamBuf, or appends a new one.
func (m *Model) upsertStreamEntry() {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].who == speakerGlitch {
			m.messages[i].text = m.streamBuf
			return
		}
		if m.messages[i].who == speakerUser {
			break
		}
	}
	m.messages = append(m.messages, entry{who: speakerGlitch, text: m.streamBuf})
}

const headerTopPad = 2

func (m *Model) resizeViewport() {
	fixedH := headerTopPad + len(m.titleLines) + 1 + 1 + 1 + 1
	vpH := m.height - fixedH
	if vpH < 5 {
		vpH = 5
	}
	vpW := m.width - 4
	if vpW < 20 {
		vpW = 20
	}
	if m.viewport.Width == 0 {
		m.viewport = viewport.New(vpW, vpH)
		m.viewport.YPosition = 0
	} else {
		m.viewport.Width = vpW
		m.viewport.Height = vpH
	}
	m.updateViewport()
}

func (m *Model) updateViewport() {
	if !m.viewportReady {
		return
	}
	m.viewport.SetContent(m.renderConversation())
	m.viewport.GotoBottom()
}

func (m *Model) renderConversation() string {
	if len(m.messages) == 0 {
		return ""
	}

	accent := lipgloss.Color("#bd93f9")
	userColor := lipgloss.Color("#8be9fd")
	textColor := lipgloss.Color("#f8f8f2")
	dimColor := lipgloss.Color("#6272a4")
	if m.pal.Accent != "" {
		accent = lipgloss.Color(m.pal.Accent)
	}

	glitchLabel := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("GL1TCH")
	userLabel := lipgloss.NewStyle().Foreground(userColor).Bold(true).Render("YOU   ")
	textStyle := lipgloss.NewStyle().Foreground(textColor)
	dimStyle := lipgloss.NewStyle().Foreground(dimColor)

	var sb strings.Builder
	for i, e := range m.messages {
		if i > 0 {
			sb.WriteString("\n")
		}
		switch e.who {
		case speakerGlitch:
			lines := strings.Split(e.text, "\n")
			for j, line := range lines {
				if j == 0 {
					sb.WriteString(fmt.Sprintf("  %s > %s\n", glitchLabel, textStyle.Render(line)))
				} else {
					sb.WriteString(fmt.Sprintf("         %s\n", textStyle.Render(line)))
				}
			}
			if m.streaming && i == len(m.messages)-1 {
				sb.WriteString(dimStyle.Render("         ▋") + "\n")
			}
		case speakerUser:
			sb.WriteString(fmt.Sprintf("  %s > %s\n", userLabel, textStyle.Render(e.text)))
		case speakerSystem:
			lines := strings.Split(e.text, "\n")
			for _, line := range lines {
				sb.WriteString(dimStyle.Render("  -- "+line+" --") + "\n")
			}
		}
	}
	return sb.String()
}

// titleVisualWidth returns the visual width of the widest title line.
func (m *Model) titleVisualWidth() int {
	max := 0
	for _, line := range m.titleLines {
		w := len([]rune(stripANSI(line)))
		if w > max {
			max = w
		}
	}
	return max
}

// stripANSI removes ESC[ ... m sequences for width measurement.
func stripANSI(s string) string {
	out := make([]byte, 0, len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		out = append(out, s[i])
		i++
	}
	return string(out)
}

func padCenter(s string, visualWidth, termWidth int) string {
	pad := (termWidth - visualWidth) / 2
	if pad <= 0 {
		return s
	}
	return strings.Repeat(" ", pad) + s
}

// View implements tea.Model.
func (m Model) View() string {
	accent := lipgloss.Color("#bd93f9")
	dimColor := lipgloss.Color("#6272a4")
	if m.pal.Accent != "" {
		accent = lipgloss.Color(m.pal.Accent)
	}

	accentStyle := lipgloss.NewStyle().Foreground(accent).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(dimColor)

	var sb strings.Builder

	// Vertical padding above title.
	for range headerTopPad {
		sb.WriteString("\n")
	}

	// Title block.
	titleW := m.titleVisualWidth()
	for _, line := range m.titleLines {
		centered := padCenter(line, titleW, m.width)
		if m.titleHasANSI {
			sb.WriteString(centered + "\n")
		} else {
			sb.WriteString(accentStyle.Render(centered) + "\n")
		}
	}

	// Subtitle.
	var providerName string
	if m.backend != nil {
		providerName = m.backend.Name()
	} else {
		providerName = "no provider"
	}
	subtitle := ">> GL1TCH AI assistant  //  " + providerName
	sb.WriteString(padCenter(dimStyle.Render(subtitle), len(subtitle), m.width) + "\n")

	// Divider.
	sb.WriteString(dimStyle.Render(strings.Repeat("─", m.width)) + "\n")

	// Conversation viewport.
	margin := strings.Repeat(" ", 2)
	for _, line := range strings.Split(m.viewport.View(), "\n") {
		sb.WriteString(margin + line + "\n")
	}

	// Input bar.
	sb.WriteString(dimStyle.Render(strings.Repeat("─", m.width)) + "\n")
	if m.backend != nil {
		sb.WriteString(m.input.View())
	} else {
		sb.WriteString(dimStyle.Render("  [ press Esc to close ]"))
	}

	return sb.String()
}
