package promptbuilder

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/adam-stokes/orcai/internal/plugin"
)

// Run launches the prompt builder as a standalone BubbleTea program.
func Run() {
	mgr := plugin.NewManager()
	for _, name := range []string{"claude", "gemini", "openspec", "openclaw"} {
		mgr.Register(plugin.NewCliAdapter(name, name+" CLI adapter", name))
	}

	m := New(mgr)
	m.SetName("new-pipeline")

	bubble := NewBubble(m, nil)
	p := tea.NewProgram(bubble, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("prompt builder error: %v\n", err)
	}
}
