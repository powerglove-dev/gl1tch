// Package welcome implements the SYSOP first-run onboarding TUI.
package welcome

import (
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
)

// SentinelFile is the path (relative to cfgDir) that marks welcome as completed.
const SentinelFile = ".welcome_seen"

// IsFirstRun returns true if the welcome has not yet been completed.
func IsFirstRun(cfgDir string) bool {
	_, err := os.Stat(filepath.Join(cfgDir, SentinelFile))
	return os.IsNotExist(err)
}

// MarkSeen writes the sentinel file to indicate welcome has been shown.
func MarkSeen(cfgDir string) error {
	f, err := os.Create(filepath.Join(cfgDir, SentinelFile))
	if err != nil {
		return err
	}
	return f.Close()
}

// --- Tea messages ---

// glitchTokenMsg carries a streamed token from the LLM.
type glitchTokenMsg struct{ token string }

// glitchDoneMsg signals the LLM stream has finished.
type glitchDoneMsg struct{}

// glitchStreamErrMsg signals a streaming error.
type glitchStreamErrMsg struct{ err error }

// streamMsg is the internal message type for chaining async channel reads.
type streamMsg struct {
	token string
	ch    <-chan string
}

// --- Conversation entry ---

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

// Model is the welcome widget BubbleTea model.
type Model struct {
	width, height int
	pal           styles.ANSIPalette

	titleLines    []string // rendered title (tdfiglet or fallback)
	titleHasANSI  bool     // true when titleLines already carry ANSI color (TDF output)
	messages      []entry  // conversation history
	viewport      viewport.Model
	viewportReady bool // true after first WindowSizeMsg
	input         textinput.Model
	phase         Phase

	streaming bool   // LLM response in progress
	streamBuf string // accumulates current token stream
	useOllama bool
	guide     *Guide

	cfgDir string
	done   bool
}

// New creates a new welcome Model.
func New(cfgDir string) Model {
	input := textinput.New()
	input.Placeholder = "type a message…"
	input.Prompt = " >> "
	input.CharLimit = 2000
	input.Focus()

	title, titleHasANSI := RenderTitle()

	useOllama := OllamaAvailable()
	var guide *Guide
	if useOllama {
		modelName := BestModel()
		guide = NewGuide(modelName)
	}

	m := Model{
		width:        80,
		height:       24,
		titleLines:   title,
		titleHasANSI: titleHasANSI,
		input:        input,
		phase:        PhaseIntro,
		useOllama:    useOllama,
		guide:        guide,
		cfgDir:       cfgDir,
	}

	// Load palette
	home, _ := os.UserHomeDir()
	if reg, err := themes.NewRegistry(filepath.Join(home, ".config", "glitch", "themes")); err == nil {
		if bundle := reg.Active(); bundle != nil {
			m.pal = styles.BundleANSI(bundle)
		}
	}

	// Mark as seen immediately — any exit path (Ctrl-C, close window, completion)
	// prevents the welcome from auto-launching again on the next session start.
	MarkSeen(cfgDir) //nolint:errcheck

	return m
}

func (m Model) Init() tea.Cmd {
	// Kick off the intro phase response immediately.
	return tea.Batch(
		textinput.Blink,
		m.startPhaseOpener(PhaseIntro),
	)
}

// startPhaseOpener triggers the SYSOP opener for the given phase.
func (m Model) startPhaseOpener(phase Phase) tea.Cmd {
	if m.useOllama && m.guide != nil {
		return func() tea.Msg {
			ch, err := m.guide.StreamResponse(phase, "")
			if err != nil {
				return glitchStreamErrMsg{err: err}
			}
			return streamNextToken(ch)()
		}
	}
	// Scripted fallback: emit the whole response immediately via glitchDoneMsg.
	return func() tea.Msg {
		return glitchDoneMsg{}
	}
}

// streamNextToken returns a Cmd that reads one token from ch and emits it.
func streamNextToken(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		token, ok := <-ch
		if !ok {
			return glitchDoneMsg{}
		}
		return streamMsg{token: token, ch: ch}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewportReady = true // must be set before resizeViewport calls updateViewport
		m.resizeViewport()

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			if m.phase == PhaseDone {
				return m, tea.Quit
			}
		case tea.KeyEnter:
			if m.streaming {
				break // ignore input while streaming
			}
			userText := strings.TrimSpace(m.input.Value())
			if userText == "" {
				break
			}
			m.input.SetValue("")
			m.messages = append(m.messages, entry{who: speakerUser, text: userText})
			m.updateViewport()

			// If done phase, just quit.
			if m.phase == PhaseDone {
				return m, tea.Quit
			}

			// Advance phase on user input.
			nextPhase := m.advancePhase(userText)

			m.streaming = true
			m.streamBuf = ""
			cmds = append(cmds, m.streamResponse(nextPhase, userText))
		}

	case streamMsg:
		m.streamBuf += msg.token
		// Update last glitch message (or append new one)
		m.upsertStreamEntry()
		m.updateViewport()
		// Schedule reading next token
		cmds = append(cmds, streamNextToken(msg.ch))

	case glitchDoneMsg:
		m.streaming = false
		if m.streamBuf == "" && !m.useOllama {
			// Scripted: flush full text now
			text := scriptedText(m.phase)
			m.messages = append(m.messages, entry{who: speakerGlitch, text: text})
		} else if m.streamBuf != "" {
			m.upsertStreamEntry()
		}
		m.streamBuf = ""
		m.updateViewport()

		// Auto-quit after the done phase response finishes.
		if m.phase == PhaseDone {
			return m, tea.Quit
		}

	case glitchStreamErrMsg:
		m.streaming = false
		m.streamBuf = ""
		// Fall back to scripted on error
		text := scriptedText(m.phase)
		m.messages = append(m.messages, entry{who: speakerGlitch, text: text})
		m.updateViewport()
	}

	// Update sub-models
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	cmds = append(cmds, inputCmd)

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// advancePhase determines the next phase based on current phase and user input.
// Returns the phase to use for the NEXT LLM response.
func (m *Model) advancePhase(_ string) Phase {
	switch m.phase {
	case PhaseIntro:
		m.phase = PhaseUseCase
	case PhaseUseCase:
		m.phase = PhaseProviders
	case PhaseProviders:
		m.phase = PhasePipeline
	case PhasePipeline:
		m.phase = PhaseNavigation
	case PhaseNavigation:
		m.phase = PhaseBrain
	case PhaseBrain:
		m.phase = PhaseDone
	}
	return m.phase
}

// streamResponse triggers LLM streaming for the given phase and user message.
func (m Model) streamResponse(phase Phase, userText string) tea.Cmd {
	if m.useOllama && m.guide != nil {
		return func() tea.Msg {
			ch, err := m.guide.StreamResponse(phase, userText)
			if err != nil {
				return glitchStreamErrMsg{err: err}
			}
			return streamNextToken(ch)()
		}
	}
	// Scripted fallback
	return func() tea.Msg {
		return glitchDoneMsg{}
	}
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

const headerTopPad = 2 // blank lines above the title

func (m *Model) resizeViewport() {
	// fixed rows: top padding + title lines + subtitle + divider + bottom divider + input
	fixedH := headerTopPad + len(m.titleLines) + 1 + 1 + 1 + 1
	vpH := m.height - fixedH
	if vpH < 5 {
		vpH = 5
	}
	vpW := m.width - 4 // 2-char margin each side
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

// titleVisualWidth returns the visual (ANSI-stripped) width of the widest title line.
func (m *Model) titleVisualWidth() int {
	max := 0
	for _, line := range m.titleLines {
		w := len([]rune(stripANSISimple(line)))
		if w > max {
			max = w
		}
	}
	return max
}

// stripANSISimple removes ESC[ ... m sequences for width measurement.
func stripANSISimple(s string) string {
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

func (m *Model) updateViewport() {
	if !m.viewportReady {
		return
	}
	m.viewport.SetContent(m.renderConversation())
	m.viewport.GotoBottom()
}

// renderConversation renders all conversation entries into a single string.
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
			// Format multi-line glitch responses with hanging indent
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
			sb.WriteString(dimStyle.Render("  -- "+e.text+" --") + "\n")
		}
	}
	return sb.String()
}

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

	// Title block — centered horizontally.
	// TDF lines carry their own ANSI color; plain fallback gets accent style.
	titleW := m.titleVisualWidth()
	for _, line := range m.titleLines {
		centered := padCenter(line, titleW, m.width)
		if m.titleHasANSI {
			sb.WriteString(centered + "\n")
		} else {
			sb.WriteString(accentStyle.Render(centered) + "\n")
		}
	}

	// Centered subtitle.
	subtitle := ">> your AI, your terminal, your rules  //  first-run setup"
	sb.WriteString(padCenter(dimStyle.Render(subtitle), len(subtitle), m.width) + "\n")

	// Full-width divider.
	sb.WriteString(dimStyle.Render(strings.Repeat("─", m.width)) + "\n")

	// Conversation viewport — padded to match margins.
	margin := strings.Repeat(" ", 2)
	for _, line := range strings.Split(m.viewport.View(), "\n") {
		sb.WriteString(margin + line + "\n")
	}

	// Input bar.
	sb.WriteString(dimStyle.Render(strings.Repeat("─", m.width)) + "\n")
	sb.WriteString(m.input.View())

	if m.phase == PhaseDone {
		sb.WriteString("\n" + dimStyle.Render("  [ press Enter or Ctrl+C to exit ]"))
	}

	return sb.String()
}
