package promptbuilder

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/plugin"
)

// Run launches the prompt builder as a standalone BubbleTea program.
func Run() {
	providers := picker.BuildProviders()

	mgr := plugin.NewManager()
	for _, p := range providers {
		if err := mgr.Register(plugin.NewCliAdapter(p.ID, p.Label+" CLI adapter", p.ID)); err != nil {
			fmt.Printf("prompt builder: register plugin %q: %v\n", p.ID, err)
		}
	}

	m := New(mgr)
	m.SetName("new-pipeline")

	bubble := NewBubble(m, providers)
	prog := tea.NewProgram(bubble, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		fmt.Printf("prompt builder error: %v\n", err)
	}
}
