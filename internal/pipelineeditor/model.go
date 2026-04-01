// Package pipelineeditor implements a full-screen two-column pipeline editor TUI.
// It replaces the old overlay-based pipeline editor in the switchboard package.
package pipelineeditor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/buildershared"
	"github.com/8op-org/gl1tch/internal/modal"
	"github.com/8op-org/gl1tch/internal/picker"
	"github.com/8op-org/gl1tch/internal/pipeline"
	"github.com/8op-org/gl1tch/internal/store"
	"github.com/8op-org/gl1tch/internal/styles"

	"gopkg.in/yaml.v3"
)

// Focus area constants.
const (
	FocusList   = 0
	FocusEditor = 1
	FocusYAML   = 2
	FocusRunner = 3
	FocusChat   = 4
)

// Legacy editor subfield constants — used by save/load helpers.
const (
	editorFieldPicker = 0
	editorFieldName   = 1
	editorFieldPrompt = 2
)

// CloseMsg is posted when the editor should be dismissed by the parent.
type CloseMsg struct{}

// ClarifyPollMsg signals a pending clarification question from the store.
type ClarifyPollMsg struct {
	RunID    string
	Question string
}

// Model holds all state for the pipeline editor TUI.
type Model struct {
	width, height int
	pipelinesDir  string
	store         *store.Store
	pal           styles.ANSIPalette

	// Shared sub-models (buildershared).
	sidebar buildershared.Sidebar
	editor  buildershared.EditorPanel
	runner  buildershared.RunnerPanel

	// Left panel (legacy — kept for backward compat; sidebar mirrors these)
	pipelines     []string
	listSel       int
	listScroll    int
	listSearch    string
	listSearching bool
	confirmDelete bool

	// Focus
	focus int // FocusList, FocusEditor, FocusYAML, FocusRunner

	// Editor (legacy — kept for backward compat)
	editorFocus int // editorFieldPicker, editorFieldName, editorFieldPrompt
	providers   []picker.ProviderDef
	picker      modal.AgentPickerModel
	nameInput   textinput.Model
	promptArea  textarea.Model
	currentPath string // empty = new pipeline

	// YAML preview
	yamlScroll int

	// Test runner (legacy — kept for backward compat)
	runLines    []string
	runRunning  bool
	runCancel   context.CancelFunc
	runOutputCh chan string

	// Clarification (in test runner)
	clarifyActive bool
	clarifyInput  textinput.Model
	clarifyRunID  string
	clarifyQ      string

	// Status
	statusMsg string
	statusErr bool

	// Send panel for agent runner at bottom of right column.
	send buildershared.SendPanel

	// Feedback loop state.
	feedbackHistory []string
	firstPrompt     string
	sentOnce        bool
}

// New creates a new pipelineeditor Model.
func New(providers []picker.ProviderDef, pipelinesDir string, st *store.Store) Model {
	ni := textinput.New()
	ni.Placeholder = "my-pipeline"
	ni.CharLimit = 128

	pa := textarea.New()
	pa.Placeholder = "Describe what this pipeline step should do…"
	pa.ShowLineNumbers = false
	pa.SetHeight(6)
	pa.SetWidth(80)

	ci := textinput.New()
	ci.Placeholder = "type your answer…"
	ci.CharLimit = 2000

	// Default Dracula-ish palette — overridden by SetPalette.
	pal := styles.ANSIPalette{
		Accent:  "\x1b[35m",
		Dim:     "\x1b[2m",
		Success: "\x1b[32m",
		Error:   "\x1b[31m",
		Warn:    "\x1b[33m",
		FG:      "\x1b[97m",
		BG:      "\x1b[40m",
		Border:  "\x1b[36m",
		SelBG:   "\x1b[44m",
	}

	m := Model{
		pipelinesDir: pipelinesDir,
		store:        st,
		pal:          pal,
		providers:    providers,
		picker:       modal.NewAgentPickerModel(providers),
		nameInput:    ni,
		promptArea:   pa,
		clarifyInput: ci,
		// Shared sub-models.
		sidebar: buildershared.NewSidebar("PIPELINES", nil),
		editor:  buildershared.NewEditorPanel(providers),
		runner:  buildershared.NewRunnerPanel(),
		send:    buildershared.NewSendPanel(providers),
	}
	m.pipelines = m.loadPipelines()
	m.sidebar = m.sidebar.SetItems(m.pipelines)
	return m
}

// SetPalette updates the color palette used for rendering.
func (m *Model) SetPalette(pal styles.ANSIPalette) {
	m.pal = pal
}

// SetSize updates the terminal dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetProviders updates the provider list and reinitialises the picker.
func (m *Model) SetProviders(providers []picker.ProviderDef) {
	m.providers = providers
	m.picker = modal.NewAgentPickerModel(providers)
}

// loadPipelines scans pipelinesDir for *.pipeline.yaml files.
func (m Model) loadPipelines() []string {
	dir := m.pipelinesDir
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if len(n) == 0 || n[0] == '.' {
			continue
		}
		if name := strings.TrimSuffix(n, ".pipeline.yaml"); name != n && name != "" {
			names = append(names, name)
		}
	}
	return names
}

// OpenNew clears the editor for a brand-new pipeline (exported for switchboard).
func (m Model) OpenNew() Model { return m.openNew() }

// OpenEdit loads an existing pipeline for editing (exported for switchboard).
func (m Model) OpenEdit(name string) Model { return m.openEdit(name) }

// openNew clears the editor for a brand-new pipeline.
func (m Model) openNew() Model {
	defaultName := fmt.Sprintf("pipeline-%d", time.Now().Unix())
	m.editor = m.editor.SetName(defaultName)
	m.editor = m.editor.SetContent("")
	m.currentPath = ""
	m.focus = FocusEditor
	m.statusMsg = ""
	m.statusErr = false
	return m
}

// openEdit loads an existing pipeline for editing.
func (m Model) openEdit(name string) Model {
	path := filepath.Join(m.pipelinesDir, name+".pipeline.yaml")
	m.currentPath = path
	m.statusMsg = ""
	m.statusErr = false

	m.editor = m.editor.SetName(name)
	m.editor = m.editor.SetContent("")

	data, err := os.ReadFile(path)
	if err == nil {
		var pl pipeline.Pipeline
		if yaml.Unmarshal(data, &pl) == nil && len(pl.Steps) == 1 {
			step := pl.Steps[0]
			m.editor = m.editor.SetContent(step.Prompt)
			// Restore picker selection from executor/model.
			if step.Executor != "" || step.Model != "" {
				m.editor = m.editor.SelectBySlug(step.Executor + "/" + step.Model)
			}
		} else {
			// Multi-step or unparseable: show raw YAML as content.
			m.editor = m.editor.SetContent(string(data))
		}
		// Show the existing YAML in the runner panel so the user can see
		// what was previously generated before deciding whether to re-run.
		yamlLines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		m.runner = m.runner.SetLines(yamlLines, "loaded — ready to run")
	}

	m.focus = FocusEditor
	return m
}

// save persists the current editor state to a .pipeline.yaml file.
func (m Model) save() (Model, error) {
	name := strings.TrimSpace(m.editor.Name())
	if name == "" {
		return m, fmt.Errorf("pipeline name is required")
	}

	dir := m.pipelinesDir
	if dir == "" {
		return m, fmt.Errorf("pipelines directory not configured")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return m, fmt.Errorf("mkdir: %w", err)
	}

	executorID := m.editor.SelectedProviderID()
	modelID := m.editor.SelectedModelID()
	prompt := m.editor.Content()

	content := buildYAML(name, executorID, modelID, prompt)

	path := filepath.Join(dir, name+".pipeline.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return m, fmt.Errorf("write: %w", err)
	}
	m.currentPath = path
	m.pipelines = m.loadPipelines()

	// Move selection to the saved pipeline.
	for i, p := range m.pipelines {
		if p == name {
			m.listSel = i
			break
		}
	}
	return m, nil
}

// buildYAML generates a single-step pipeline YAML string.
func buildYAML(name, executorID, modelID, prompt string) string {
	if name == "" {
		name = "unnamed"
	}
	if executorID == "" {
		executorID = "claude"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("name: %s\nversion: \"1\"\nsteps:\n", name))
	sb.WriteString(fmt.Sprintf("  - id: run\n    executor: %s\n", executorID))
	if modelID != "" {
		sb.WriteString(fmt.Sprintf("    model: %s\n", modelID))
	}
	if prompt != "" {
		sb.WriteString("    prompt: |\n")
		for _, line := range strings.Split(prompt, "\n") {
			sb.WriteString("      ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// syncEditorFocus updates Focus/Blur state on editor widgets.
func (m *Model) syncEditorFocus() {
	switch m.editorFocus {
	case editorFieldName:
		m.nameInput.Focus()
		m.promptArea.Blur()
	case editorFieldPrompt:
		m.nameInput.Blur()
		m.promptArea.Focus()
	default:
		m.nameInput.Blur()
		m.promptArea.Blur()
	}
}

// clampListSel keeps listSel in bounds.
func (m *Model) clampListSel() {
	if len(m.pipelines) == 0 {
		m.listSel = 0
		return
	}
	if m.listSel < 0 {
		m.listSel = 0
	}
	if m.listSel >= len(m.pipelines) {
		m.listSel = len(m.pipelines) - 1
	}
}

// filteredPipelines returns pipelines matching the current search query.
func (m Model) filteredPipelines() []string {
	if m.listSearch == "" {
		return m.pipelines
	}
	q := strings.ToLower(m.listSearch)
	var out []string
	for _, p := range m.pipelines {
		if strings.Contains(strings.ToLower(p), q) {
			out = append(out, p)
		}
	}
	return out
}

// Init implements tea.Model (no-op; parent drives the loop).
func (m Model) Init() tea.Cmd { return nil }
