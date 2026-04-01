package apmmanager

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/themes"
	"github.com/powerglove-dev/gl1tch/internal/tuikit"
)

// Dracula palette constants for color-coded status indicators.
const (
	draculaGreen  = "#50fa7b"
	draculaYellow = "#f1fa8c"
	draculaRed    = "#ff5555"
	draculaDim    = "#6272a4"
	draculaCyan   = "#8be9fd"
	draculaBg     = "#282a36"
	draculaFg     = "#f8f8f2"
	draculaBorder = "#44475a"
)

// InstallState represents the lifecycle state of an APM agent.
type InstallState int

const (
	StateAvailable  InstallState = iota // known in manifest, not yet fetched
	StateInstalling                     // apm install running in background
	StateInstalled                      // .agent.md on disk, CliAdapter registered
	StateError                          // last install attempt failed
)

// Agent is the canonical in-memory representation of one APM agent entry.
type Agent struct {
	// ID is the APM dependency string (e.g. "anthropics/skills/agents/api-architect").
	ID string
	// Name is the human-readable display name derived from the .agent.md filename.
	Name string
	// Version is the pinned ref from apm.yml, or empty if floating.
	Version string
	// Description is extracted from the agent frontmatter or the first non-blank line.
	Description string
	// Capabilities are tags from the agent frontmatter's capabilities list.
	Capabilities []string
	// InstallState reflects the current lifecycle phase.
	InstallState InstallState
	// ErrMsg holds the error string when InstallState == StateError.
	ErrMsg string
	// AgentMDPath is the absolute path to the deployed .agent.md file, empty if not installed.
	AgentMDPath string
	// ExecutorID is the registered executor name in the executor.Manager, empty if not installed.
	ExecutorID string
}

// pane indices for two-pane layout.
const (
	paneList   = 0
	paneDetail = 1
)

// Model is the BubbleTea model for the APM agent manager TUI.
type Model struct {
	themeState tuikit.ThemeState
	bundle     *themes.Bundle

	agents      []Agent
	selectedIdx int // index into agents slice

	// scrollOffset tracks vertical scroll position in the list pane.
	scrollOffset int

	activePane int // paneList or paneDetail

	helpOpen bool
	width    int
	height   int

	// projectRoot is the directory containing apm.yml, used to locate deployed agents.
	projectRoot string

	executorMgr *executor.Manager
}

// New returns an ApmManager model ready for use. projectRoot should be the
// directory containing apm.yml (usually the git root). executorMgr is the
// shared executor registry where installed agents are registered as CliAdapters.
func New(projectRoot string, executorMgr *executor.Manager) Model {
	return Model{
		themeState:  tuikit.NewThemeState(nil),
		projectRoot: projectRoot,
		executorMgr:   executorMgr,
	}
}

// Init starts the theme subscription and kicks off the initial agent scan.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.themeState.Init(),
		loadAgentsCmd(m.projectRoot),
	)
}

// Update handles incoming messages and returns the updated model plus any commands.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Theme is highest priority — handle before all other messages.
	if ts, cmd, ok := m.themeState.Handle(msg); ok {
		m.themeState = ts
		m.bundle = ts.Bundle()
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case agentScanResultMsg:
		if msg.err != nil {
			// Surface the scan error as a single pseudo-agent in error state.
			m.agents = []Agent{{
				ID:           "scan-error",
				Name:         "Error loading agents",
				Description:  msg.err.Error(),
				InstallState: StateError,
				ErrMsg:       msg.err.Error(),
			}}
		} else {
			m.agents = msg.agents
		}
		return m, nil

	case AgentInstallStartMsg:
		m.setAgentState(msg.AgentID, StateInstalling, "")
		return m, nil

	case agentInstallResultMsg:
		if msg.err != nil {
			m.setAgentState(msg.agentID, StateError, msg.err.Error())
			return m, func() tea.Msg { return AgentInstallErrMsg{AgentID: msg.agentID, Err: msg.err} }
		}
		m.setAgentState(msg.agentID, StateInstalled, "")
		if idx := m.indexByID(msg.agentID); idx >= 0 {
			m.agents[idx].ExecutorID = msg.executorID
		}
		return m, func() tea.Msg {
			return AgentInstallDoneMsg{
				AgentID:  msg.agentID,
				ExecutorID: msg.executorID,
				Adapter:  msg.adapter,
			}
		}

	case AgentUninstallDoneMsg:
		m.setAgentState(msg.AgentID, StateAvailable, "")
		if idx := m.indexByID(msg.AgentID); idx >= 0 {
			m.agents[idx].ExecutorID = ""
			m.agents[idx].AgentMDPath = ""
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// handleKey dispatches key events depending on which pane is active.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.helpOpen {
		if msg.String() == "?" || msg.String() == "esc" || msg.String() == "q" {
			m.helpOpen = false
		}
		return m, nil
	}

	switch msg.String() {
	case "?":
		m.helpOpen = true
		return m, nil

	case "tab":
		m.activePane = 1 - m.activePane // toggle between list and detail
		return m, nil

	case "up", "k":
		if m.activePane == paneList && m.selectedIdx > 0 {
			m.selectedIdx--
			m.clampScroll()
		}
		return m, nil

	case "down", "j":
		if m.activePane == paneList && m.selectedIdx < len(m.agents)-1 {
			m.selectedIdx++
			m.clampScroll()
		}
		return m, nil

	case "i":
		return m.installSelected()

	case "u":
		return m.uninstallSelected()

	case "enter":
		return m.activateSelected()
	}

	return m, nil
}

// installSelected starts a background install for the selected agent.
func (m Model) installSelected() (tea.Model, tea.Cmd) {
	a := m.selectedAgent()
	if a == nil || a.InstallState == StateInstalling || a.InstallState == StateInstalled {
		return m, nil
	}
	m.setAgentState(a.ID, StateInstalling, "")
	return m, tea.Batch(
		func() tea.Msg { return AgentInstallStartMsg{AgentID: a.ID} },
		installAgentCmd(m.projectRoot, *a, m.executorMgr),
	)
}

// uninstallSelected removes the selected agent from disk and the executor registry.
func (m Model) uninstallSelected() (tea.Model, tea.Cmd) {
	a := m.selectedAgent()
	if a == nil || a.InstallState != StateInstalled {
		return m, nil
	}
	agentID := a.ID
	root := m.projectRoot
	mgr := m.executorMgr
	return m, func() tea.Msg {
		if err := uninstallAgent(root, agentID, mgr); err != nil {
			return AgentInstallErrMsg{AgentID: agentID, Err: err}
		}
		return AgentUninstallDoneMsg{AgentID: agentID}
	}
}

// activateSelected emits AgentActivatedMsg for the selected installed agent.
func (m Model) activateSelected() (tea.Model, tea.Cmd) {
	a := m.selectedAgent()
	if a == nil || a.InstallState != StateInstalled {
		return m, nil
	}
	executorID := a.ExecutorID
	agentID := a.ID
	return m, func() tea.Msg {
		return AgentActivatedMsg{AgentID: agentID, ExecutorID: executorID}
	}
}

// View renders the full two-pane layout.
func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	// Reserve 1 row for footer.
	contentH := m.height - 2
	if contentH < 4 {
		contentH = 4
	}

	listW := m.width / 3
	detailW := m.width - listW - 1

	left := m.viewList(listW, contentH)
	right := m.viewDetail(detailW, contentH)

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color(draculaBorder)).
		Render(strings.Repeat("│\n", contentH))

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
	footer := m.viewFooter()

	content := lipgloss.JoinVertical(lipgloss.Left, body, footer)

	if m.helpOpen {
		return renderOverlay(content, m.viewHelp(), m.width, m.height)
	}

	return content
}

// viewList renders the left pane: scrollable agent list with status indicators.
func (m Model) viewList(w, h int) string {
	style := lipgloss.NewStyle().Width(w).Height(h)

	if len(m.agents) == 0 {
		return style.Render(lipgloss.NewStyle().
			Foreground(lipgloss.Color(draculaDim)).
			Render("  No agents found.\n  Add dependencies to apm.yml\n  and run: task apm:fetch"))
	}

	var sb strings.Builder
	visible := h
	start := m.scrollOffset
	end := start + visible
	if end > len(m.agents) {
		end = len(m.agents)
	}

	for i := start; i < end; i++ {
		a := m.agents[i]
		indicator := m.stateIndicator(a.InstallState)
		name := truncate(a.Name, w-6)

		line := fmt.Sprintf(" %s %-*s", indicator, w-5, name)

		if i == m.selectedIdx {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color(draculaBorder)).
				Foreground(lipgloss.Color(draculaFg)).
				Bold(true).
				Render(line)
		} else {
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color(draculaFg)).
				Render(line)
		}
		sb.WriteString(line + "\n")
	}

	return style.Render(sb.String())
}

// viewDetail renders the right pane: metadata for the selected agent.
// The pane border is highlighted when paneDetail is active.
func (m Model) viewDetail(w, h int) string {
	borderColor := draculaBorder
	if m.activePane == paneDetail {
		borderColor = draculaCyan
	}
	_ = borderColor // used for future border rendering
	style := lipgloss.NewStyle().Width(w).Height(h).PaddingLeft(1)

	a := m.selectedAgent()
	if a == nil {
		return style.Render("")
	}

	label := lipgloss.NewStyle().
		Foreground(lipgloss.Color(draculaCyan)).
		Bold(true)

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(draculaDim))

	var sb strings.Builder
	sb.WriteString(label.Render(a.Name) + "\n")

	if a.Version != "" {
		sb.WriteString(dim.Render("version: ") + a.Version + "\n")
	}
	sb.WriteString(dim.Render("id:      ") + truncate(a.ID, w-10) + "\n")
	sb.WriteString(dim.Render("status:  ") + m.stateLabel(a.InstallState) + "\n")

	if a.ErrMsg != "" {
		sb.WriteString("\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color(draculaRed)).
			Render("error: "+a.ErrMsg) + "\n")
	}

	if a.Description != "" {
		sb.WriteString("\n" + a.Description + "\n")
	}

	if len(a.Capabilities) > 0 {
		sb.WriteString("\n" + dim.Render("capabilities:") + "\n")
		for _, cap := range a.Capabilities {
			sb.WriteString("  • " + cap + "\n")
		}
	}

	if a.ExecutorID != "" {
		sb.WriteString("\n" + dim.Render("executor: ") + a.ExecutorID + "\n")
	}

	return style.Render(sb.String())
}

// viewFooter renders the keybinding hint bar.
func (m Model) viewFooter() string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(draculaDim))
	acc := lipgloss.NewStyle().Foreground(lipgloss.Color(draculaCyan))

	keys := []struct{ k, d string }{
		{"↑/↓", "navigate"},
		{"tab", "switch pane"},
		{"i", "install"},
		{"u", "uninstall"},
		{"enter", "activate"},
		{"?", "help"},
	}

	var parts []string
	for _, kd := range keys {
		parts = append(parts, acc.Render(kd.k)+" "+dim.Render(kd.d))
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(draculaDim)).
		Width(m.width).
		Render(strings.Join(parts, "  "))
}

// viewHelp renders the help overlay content.
func (m Model) viewHelp() string {
	lines := []string{
		"APM Agent Manager — Help",
		"",
		"  ↑ / k       Move selection up",
		"  ↓ / j       Move selection down",
		"  tab         Switch between list and detail pane",
		"  i           Install selected agent",
		"  u           Uninstall selected agent",
		"  enter       Activate selected agent (emits AgentActivatedMsg)",
		"  ? / esc     Toggle this help",
		"",
		"Agents are fetched from apm.yml and wrapped as CliAdapter executors.",
		"Press i to install, then enter to make the agent available to executors.",
	}
	return strings.Join(lines, "\n")
}

// renderOverlay renders a centered modal box over the background view.
// background is the fully rendered TUI content; overlay is the modal text.
// lipgloss.Place composites the modal box at the center of the terminal area.
func renderOverlay(background, overlay string, w, h int) string {
	_ = background // BubbleTea re-renders the background; we return the modal view
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(draculaCyan)).
		Background(lipgloss.Color(draculaBg)).
		Padding(1, 2).
		MaxWidth(w - 4).
		Render(overlay)

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceBackground(lipgloss.Color(draculaBg)))
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func (m Model) selectedAgent() *Agent {
	if len(m.agents) == 0 || m.selectedIdx >= len(m.agents) {
		return nil
	}
	a := m.agents[m.selectedIdx]
	return &a
}

func (m *Model) setAgentState(id string, state InstallState, errMsg string) {
	for i := range m.agents {
		if m.agents[i].ID == id {
			m.agents[i].InstallState = state
			m.agents[i].ErrMsg = errMsg
			return
		}
	}
}

func (m Model) indexByID(id string) int {
	for i, a := range m.agents {
		if a.ID == id {
			return i
		}
	}
	return -1
}

// clampScroll keeps m.scrollOffset such that selectedIdx is visible.
func (m *Model) clampScroll() {
	visible := m.height - 3
	if visible < 1 {
		visible = 1
	}
	if m.selectedIdx < m.scrollOffset {
		m.scrollOffset = m.selectedIdx
	} else if m.selectedIdx >= m.scrollOffset+visible {
		m.scrollOffset = m.selectedIdx - visible + 1
	}
}

func (m Model) stateIndicator(s InstallState) string {
	switch s {
	case StateInstalled:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(draculaGreen)).Render("●")
	case StateInstalling:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(draculaYellow)).Render("◌")
	case StateError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(draculaRed)).Render("✗")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(draculaDim)).Render("○")
	}
}

func (m Model) stateLabel(s InstallState) string {
	switch s {
	case StateInstalled:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(draculaGreen)).Render("installed")
	case StateInstalling:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(draculaYellow)).Render("installing…")
	case StateError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(draculaRed)).Render("error")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(draculaDim)).Render("available")
	}
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}

// ── Commands ─────────────────────────────────────────────────────────────────

// loadAgentsCmd scans the project's apm.yml and .claude/agents/ directory to
// build the initial agent list, cross-referencing install state with the executor
// manager.
func loadAgentsCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		agents, err := scanAgents(projectRoot)
		return agentScanResultMsg{agents: agents, err: err}
	}
}

// installAgentCmd runs `apm install <id>` in the background, then reads the
// deployed .agent.md and registers a CliAdapter in executorMgr.
func installAgentCmd(projectRoot string, a Agent, executorMgr *executor.Manager) tea.Cmd {
	return func() tea.Msg {
		agentMDPath, adapter, err := installAndWrap(context.Background(), projectRoot, a, executorMgr)
		if err != nil {
			return agentInstallResultMsg{agentID: a.ID, err: err}
		}
		_ = agentMDPath
		return agentInstallResultMsg{
			agentID:  a.ID,
			executorID: adapter.Name(),
			adapter:  adapter,
		}
	}
}

// ── Business logic ────────────────────────────────────────────────────────────

// scanAgents reads apm.yml for declared dependencies and .claude/agents/ for
// installed .agent.md files, returning a merged Agent list.
func scanAgents(projectRoot string) ([]Agent, error) {
	agentsDir := filepath.Join(projectRoot, ".claude", "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("scan agents dir %s: %w", agentsDir, err)
	}

	seen := map[string]bool{}
	var agents []Agent

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".agent.md") {
			continue
		}
		path := filepath.Join(agentsDir, entry.Name())
		a := parseAgentMD(path)
		if a.ID == "" {
			a.ID = strings.TrimSuffix(entry.Name(), ".agent.md")
		}
		a.InstallState = StateInstalled
		a.AgentMDPath = path
		a.ExecutorID = agentExecutorID(a.ID)
		seen[a.ID] = true
		agents = append(agents, a)
	}

	return agents, nil
}

// parseAgentMD reads a .agent.md file and extracts name, description, version,
// and capabilities from its YAML frontmatter (--- delimited block).
func parseAgentMD(path string) Agent {
	f, err := os.Open(path)
	if err != nil {
		return Agent{Name: filepath.Base(path)}
	}
	defer f.Close()

	a := Agent{}
	a.Name = strings.TrimSuffix(filepath.Base(path), ".agent.md")

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	frontmatterDone := false
	inCapabilities := false

	for scanner.Scan() {
		line := scanner.Text()

		if !inFrontmatter && line == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter && !frontmatterDone {
			if line == "---" {
				frontmatterDone = true
				inCapabilities = false
				continue
			}
			if v, ok := strings.CutPrefix(line, "name:"); ok {
				a.ID = strings.TrimSpace(v)
			} else if v, ok := strings.CutPrefix(line, "description:"); ok {
				a.Description = strings.TrimSpace(v)
			} else if v, ok := strings.CutPrefix(line, "version:"); ok {
				a.Version = strings.TrimSpace(v)
			} else if strings.HasPrefix(line, "capabilities:") {
				inCapabilities = true
			} else if inCapabilities && strings.HasPrefix(line, "  -") {
				cap := strings.TrimSpace(strings.TrimPrefix(line, "  -"))
				a.Capabilities = append(a.Capabilities, cap)
			} else if !strings.HasPrefix(line, " ") {
				inCapabilities = false
			}
		}

		// Fall back: use first non-blank, non-heading line as description.
		if frontmatterDone && a.Description == "" &&
			!strings.HasPrefix(line, "#") && strings.TrimSpace(line) != "" {
			a.Description = strings.TrimSpace(line)
		}
	}

	return a
}

// installAndWrap runs `apm install <id>`, locates the deployed .agent.md, reads
// it as a system prompt, and registers a CliAdapter in executorMgr.
// It returns the path to the deployed file and the registered adapter.
func installAndWrap(
	ctx context.Context,
	projectRoot string,
	a Agent,
	executorMgr *executor.Manager,
) (string, *executor.CliAdapter, error) {
	// Run `apm install <id>` to fetch and deploy the agent file.
	cmd := exec.CommandContext(ctx, "apm", "install", a.ID)
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("apm install %s: %w\n%s", a.ID, err, out)
	}

	// Locate the deployed .agent.md under .claude/agents/.
	agentMDPath, err := findDeployedAgentMD(projectRoot, a.ID)
	if err != nil {
		return "", nil, err
	}

	// Read the agent.md as a system prompt.
	systemPrompt, err := os.ReadFile(agentMDPath)
	if err != nil {
		return "", nil, fmt.Errorf("read agent md %s: %w", agentMDPath, err)
	}

	// Wrap as a CliAdapter: claude --print receives user input on stdin and
	// uses the agent.md content as the system prompt.
	executorID := agentExecutorID(a.ID)
	desc := a.Description
	if desc == "" {
		desc = "APM agent: " + a.Name
	}

	adapter := executor.NewCliAdapter(
		executorID,
		desc,
		"claude",
		"--print",
		"--system", string(systemPrompt),
	)

	if err := executorMgr.Register(adapter); err != nil {
		// If already registered (e.g. re-install), treat as success.
		if !strings.Contains(err.Error(), "already registered") {
			return "", nil, fmt.Errorf("register executor %s: %w", executorID, err)
		}
	}

	return agentMDPath, adapter, nil
}

// findDeployedAgentMD searches .claude/agents/ for a file matching the agent ID.
// APM normalizes file names so we try several candidates.
func findDeployedAgentMD(projectRoot, agentID string) (string, error) {
	agentsDir := filepath.Join(projectRoot, ".claude", "agents")

	// Try the last path segment as the filename.
	segments := strings.Split(agentID, "/")
	base := segments[len(segments)-1]
	if !strings.HasSuffix(base, ".agent.md") {
		base = base + ".agent.md"
	}

	candidates := []string{
		filepath.Join(agentsDir, base),
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	// Walk the directory as a fallback.
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return "", fmt.Errorf("agents dir missing after install: %w", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".agent.md") {
			return filepath.Join(agentsDir, e.Name()), nil
		}
	}

	return "", fmt.Errorf("deployed agent.md not found for %s in %s", agentID, agentsDir)
}

// uninstallAgent removes the deployed .agent.md from disk.
// executor.Manager has no Remove method; the executor stays registered for the
// current session (safe — it won't accept new work once removed from the
// filesystem). Full deregistration takes effect on the next glitch restart.
func uninstallAgent(projectRoot, agentID string, mgr *executor.Manager) error {
	_ = mgr // reserved for when executor.Manager gains a Remove method
	agentsDir := filepath.Join(projectRoot, ".claude", "agents")
	segments := strings.Split(agentID, "/")
	name := segments[len(segments)-1]
	if !strings.HasSuffix(name, ".agent.md") {
		name += ".agent.md"
	}
	path := filepath.Join(agentsDir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// agentExecutorID derives a stable executor registry name from an APM agent ID.
// e.g. "anthropics/skills/agents/api-architect" → "apm.api-architect"
func agentExecutorID(agentID string) string {
	segments := strings.Split(agentID, "/")
	base := segments[len(segments)-1]
	base = strings.TrimSuffix(base, ".agent.md")
	return "apm." + base
}
