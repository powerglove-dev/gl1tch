package promptbuilder

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	Open     key.Binding
	AddStep  key.Binding
	Run      key.Binding
	Save     key.Binding
	Quit     key.Binding
	Help     key.Binding
}

var keys = keyMap{
	Up:       key.NewBinding(key.WithKeys("up", "k"),    key.WithHelp("↑/k", "prev step")),
	Down:     key.NewBinding(key.WithKeys("down", "j"),  key.WithHelp("↓/j", "next step")),
	Tab:      key.NewBinding(key.WithKeys("tab"),         key.WithHelp("tab", "next field")),
	ShiftTab: key.NewBinding(key.WithKeys("shift+tab"),  key.WithHelp("shift+tab", "prev field")),
	Open:     key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter/space", "open dropdown")),
	AddStep:  key.NewBinding(key.WithKeys("+"),           key.WithHelp("+", "add step")),
	Run:      key.NewBinding(key.WithKeys("r"),           key.WithHelp("r", "run")),
	Save:     key.NewBinding(key.WithKeys("s"),           key.WithHelp("s", "save")),
	Quit:     key.NewBinding(key.WithKeys("esc", "q"),   key.WithHelp("esc", "quit")),
	Help:     key.NewBinding(key.WithKeys("?"),           key.WithHelp("?", "help")),
}
