// Package opsx provides the OpenSpec workflow integration for glitch.
// It exposes a Bubble Tea prompt for entering a feature name and helpers
// for sending the /opsx:propose command to the active tmux window.
package opsx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/8op-org/gl1tch/internal/styles"
)

// providerDef holds OpenSpec configuration for a single glitch provider.
// slashCmd is the slash command string to send (e.g. "/opsx:propose"); if
// empty the fallback prompt-injection path is used instead.
type providerDef struct {
	toolID   string // openspec --tools ID (e.g. "github-copilot")
	slashCmd string // empty → no slash command support
}

// providerOpsx maps glitch provider IDs to OpenSpec configuration.
// Only providers that support the full OpenSpec workflow are listed here;
// unknown providers are silently skipped by ProviderSend.
var providerOpsx = map[string]providerDef{
	"claude":   {toolID: "claude", slashCmd: "/opsx:propose"},
	"opencode": {toolID: "opencode", slashCmd: "/opsx:propose"},
	"copilot":  {toolID: "github-copilot", slashCmd: ""},
}

// FeatureSlug converts a human feature name to a lowercase hyphenated slug
// suitable for use as an OpenSpec feature directory name.
func FeatureSlug(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToLower(strings.Join(strings.Fields(name), "-"))
}

// isOpenSpecInstalled reports whether the openspec CLI is available in PATH.
func isOpenSpecInstalled() bool {
	_, err := exec.LookPath("openspec")
	return err == nil
}

// EnsureInit runs "openspec init --tools <toolID>" in workdir when the
// openspec/config.yaml file does not yet exist there.
// Does nothing if openspec is not installed, workdir is empty, or the
// provider is not in providerOpsx.
func EnsureInit(providerID, workdir string) {
	if !isOpenSpecInstalled() || workdir == "" {
		return
	}
	info, ok := providerOpsx[providerID]
	if !ok {
		return
	}
	if _, err := os.Stat(filepath.Join(workdir, "openspec", "config.yaml")); err == nil {
		return // already initialised
	}
	cmd := exec.Command("openspec", "init", "--tools", info.toolID)
	cmd.Dir = workdir
	cmd.Run() //nolint:errcheck
}

// ProviderSend runs the OpenSpec propose workflow appropriate for providerID.
//
//   - claude / opencode: ensures init, then sends the /opsx:propose slash
//     command via tmux send-keys.
//   - copilot: ensures init, scaffolds "openspec new change <feature>", then
//     injects a natural-language prompt so the AI fills in the spec files.
//
// workdir is the session's working directory (worktree or project root).
// It may be empty when the session was not started inside a git repo, in
// which case openspec init is skipped but the slash command is still sent.
func ProviderSend(feature, providerID, workdir string) {
	if feature == "" {
		return
	}
	info, ok := providerOpsx[providerID]
	if !ok {
		return // unsupported provider — nothing to do
	}

	EnsureInit(providerID, workdir)
	time.Sleep(2 * time.Second)

	if info.slashCmd != "" {
		// Provider supports slash commands — send directly.
		exec.Command("tmux", "send-keys", "-t", "glitch",
			info.slashCmd+" "+feature, "Enter").Run() //nolint:errcheck
		return
	}

	// Fallback: scaffold the change directory then inject a natural-language
	// prompt so the AI generates the proposal, design, and task files.
	if isOpenSpecInstalled() && workdir != "" {
		cmd := exec.Command("openspec", "new", "change", feature)
		cmd.Dir = workdir
		cmd.Run() //nolint:errcheck
	}
	prompt := "I've scaffolded openspec/changes/" + feature + "/. " +
		"Please read openspec/config.yaml for project context, then " +
		"populate proposal.md (problem, solution, approach, success criteria), " +
		"design.md (technical design), and tasks.md (implementation checklist) " +
		"for the feature: " + feature
	exec.Command("tmux", "send-keys", "-t", "glitch", prompt, "Enter").Run() //nolint:errcheck
}

// DetectActiveProvider returns the glitch provider ID of the currently active
// window in the glitch tmux session (e.g. "claude" from window name "claude-1").
// Returns "" if the window name does not match a known provider.
func DetectActiveProvider() string {
	out, err := exec.Command("tmux", "display-message", "-t", "glitch", "-p", "#{window_name}").Output()
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(out))
	for id := range providerOpsx {
		if name == id || strings.HasPrefix(name, id+"-") {
			return id
		}
	}
	return ""
}

// ActivePanePath returns the working directory of the active pane in the
// glitch tmux session.
func ActivePanePath() string {
	out, err := exec.Command("tmux", "display-message", "-t", "glitch", "-p", "#{pane_current_path}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ── Prompt TUI ───────────────────────────────────────────────────────────────

type promptModel struct {
	input  textinput.Model
	result string
	done   bool
	width  int
}

func newPromptModel() promptModel {
	ti := textinput.New()
	ti.Placeholder = "e.g. add-login-flow"
	ti.Focus()
	ti.CharLimit = 80
	return promptModel{input: ti}
}

func (m promptModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m promptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			slug := FeatureSlug(m.input.Value())
			if slug != "" {
				m.result = slug
				m.done = true
				return m, tea.Quit
			}
		case "esc", "ctrl+c":
			m.done = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m promptModel) View() string {
	w := m.width
	if w <= 0 {
		w = 44
	}

	headerStyle := lipgloss.NewStyle().
		Background(styles.Purple).
		Foreground(styles.Bg).
		Bold(true).
		Width(w).
		Padding(0, 1)

	bodyStyle := lipgloss.NewStyle().
		Width(w).
		Padding(1, 2)

	footerStyle := lipgloss.NewStyle().
		Foreground(styles.Comment).
		Width(w).
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().
		Foreground(styles.Fg).
		Bold(true)

	body := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("Feature name:"),
		m.input.View(),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		headerStyle.Render("GLITCH  OpenSpec"),
		bodyStyle.Render(body),
		footerStyle.Render("enter confirm  esc cancel"),
	)
}

// Prompt runs a minimal altscreen TUI asking for a feature name.
// Returns the slug-ified feature name, or "" if the user cancelled.
func Prompt() string {
	m := newPromptModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return ""
	}
	pm, ok := result.(promptModel)
	if !ok {
		return ""
	}
	return pm.result
}
