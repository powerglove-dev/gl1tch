package promptmgr

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	New      key.Binding
	Edit     key.Binding
	Delete   key.Binding
	Save     key.Binding
	Run      key.Binding
	Cancel   key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	Up       key.Binding
	Down     key.Binding
	Quit     key.Binding
}

var keys = keyMap{
	New:      key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
	Edit:     key.NewBinding(key.WithKeys("e", "enter"), key.WithHelp("e/enter", "edit")),
	Delete:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
	Save:     key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
	Run:      key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "run")),
	Cancel:   key.NewBinding(key.WithKeys("ctrl+c", "esc"), key.WithHelp("ctrl+c", "cancel")),
	Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next panel")),
	ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev panel")),
	Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
	Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
	Quit:     key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
}
