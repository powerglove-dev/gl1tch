package promptmgr

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/plugin"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/tuikit"
)

// Message types

type promptsLoadedMsg struct{ prompts []store.Prompt }
type modelSlugsLoadedMsg struct{ slugs []string }
type runTokenMsg struct{ token string }
type runDoneMsg struct{ full string }
type runErrMsg struct{ err error }
type statusClearMsg struct{}

// loadPromptsCmd fetches all prompts from the store.
func loadPromptsCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		prompts, err := st.ListPrompts(context.Background())
		if err != nil {
			return runErrMsg{err: err}
		}
		return promptsLoadedMsg{prompts: prompts}
	}
}

// loadModelSlugsCmd fetches available model slugs from the plugin manager.
func loadModelSlugsCmd(mgr *plugin.Manager) tea.Cmd {
	return func() tea.Msg {
		if mgr == nil {
			return modelSlugsLoadedMsg{slugs: []string{}}
		}
		var slugs []string
		for _, p := range mgr.List() {
			slugs = append(slugs, p.Name())
		}
		return modelSlugsLoadedMsg{slugs: slugs}
	}
}

// Update handles all incoming messages for the prompt manager.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Delegate theme messages first.
	if ts, cmd, ok := m.themeState.Handle(msg); ok {
		m.themeState = ts
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch {
		case msg.String() == "q", msg.String() == "esc", msg.String() == "ctrl+c":
			return m, tea.Quit
		}

	case promptsLoadedMsg:
		m.prompts = msg.prompts
		m.filtered = msg.prompts

	case modelSlugsLoadedMsg:
		m.modelSlugs = msg.slugs

	case tuikit.ThemeChangedMsg:
		// Already handled above via themeState.Handle; included here to avoid
		// falling through to the default no-op path if Handle didn't claim it.

	}

	return m, nil
}
