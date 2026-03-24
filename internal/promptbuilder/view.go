package promptbuilder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/pipeline"
)

var (
	borderStyle  = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("63"))
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	selectedStep = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStep      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	statusBar    = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
)

var pluginList = []string{"claude", "gemini", "openspec", "openclaw"}

var modelsByPlugin = map[string][]string{
	"claude":   {"claude-sonnet-4-6", "claude-opus-4-6", "claude-haiku-4-5-20251001"},
	"gemini":   {"gemini-2.0-flash", "gemini-1.5-pro"},
	"openspec": {},
	"openclaw": {},
}

// BubbleModel wraps Model and implements tea.Model.
type BubbleModel struct {
	inner       *Model
	width       int
	height      int
	activeField int // 0=Plugin 1=Model 2=Prompt
	pluginIndex int
	modelIndex  int
	promptInput textinput.Model
}

// NewBubble creates a bubbletea-compatible model.
func NewBubble(m *Model) *BubbleModel {
	ti := textinput.New()
	ti.Placeholder = "enter prompt..."
	return &BubbleModel{inner: m, promptInput: ti}
}

func (b *BubbleModel) Init() tea.Cmd { return nil }

func (b *BubbleModel) syncIndicesFromStep() {
	b.pluginIndex = 0
	b.modelIndex = 0
	steps := b.inner.Steps()
	if len(steps) == 0 {
		return
	}
	sel := steps[b.inner.SelectedIndex()]
	for i, p := range pluginList {
		if p == sel.Plugin {
			b.pluginIndex = i
			break
		}
	}
	models := modelsByPlugin[pluginList[b.pluginIndex]]
	for i, mo := range models {
		if mo == sel.Model {
			b.modelIndex = i
			break
		}
	}
}

func (b *BubbleModel) applyPlugin() {
	steps := b.inner.Steps()
	if len(steps) == 0 {
		return
	}
	idx := b.inner.SelectedIndex()
	s := steps[idx]
	s.Plugin = pluginList[b.pluginIndex]
	b.modelIndex = 0
	models := modelsByPlugin[s.Plugin]
	if len(models) > 0 {
		s.Model = models[0]
	} else {
		s.Model = ""
	}
	b.inner.UpdateStep(idx, s)
}

func (b *BubbleModel) applyModel() {
	steps := b.inner.Steps()
	if len(steps) == 0 {
		return
	}
	idx := b.inner.SelectedIndex()
	s := steps[idx]
	models := modelsByPlugin[pluginList[b.pluginIndex]]
	if len(models) > 0 {
		s.Model = models[b.modelIndex]
	}
	b.inner.UpdateStep(idx, s)
}

func (b *BubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.width = msg.Width
		b.height = msg.Height
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return b, tea.Quit

		case key.Matches(msg, keys.Tab):
			if len(b.inner.Steps()) > 0 {
				b.activeField = (b.activeField + 1) % 3
				if b.activeField == 2 {
					b.promptInput.SetValue(b.inner.Steps()[b.inner.SelectedIndex()].Prompt)
					b.promptInput.Focus()
				} else {
					b.promptInput.Blur()
				}
			}

		case key.Matches(msg, keys.ShiftTab):
			if len(b.inner.Steps()) > 0 {
				b.activeField = (b.activeField + 2) % 3
				if b.activeField == 2 {
					b.promptInput.SetValue(b.inner.Steps()[b.inner.SelectedIndex()].Prompt)
					b.promptInput.Focus()
				} else {
					b.promptInput.Blur()
				}
			}

		case key.Matches(msg, keys.Up):
			b.inner.SelectStep(b.inner.SelectedIndex() - 1)
			b.activeField = 0
			b.promptInput.Blur()
			b.syncIndicesFromStep()

		case key.Matches(msg, keys.Down):
			b.inner.SelectStep(b.inner.SelectedIndex() + 1)
			b.activeField = 0
			b.promptInput.Blur()
			b.syncIndicesFromStep()

		case key.Matches(msg, keys.Left):
			if len(b.inner.Steps()) == 0 {
				break
			}
			switch b.activeField {
			case 0:
				b.pluginIndex = (b.pluginIndex - 1 + len(pluginList)) % len(pluginList)
				b.applyPlugin()
			case 1:
				models := modelsByPlugin[pluginList[b.pluginIndex]]
				if len(models) > 0 {
					b.modelIndex = (b.modelIndex - 1 + len(models)) % len(models)
					b.applyModel()
				}
			}

		case key.Matches(msg, keys.Right):
			if len(b.inner.Steps()) == 0 {
				break
			}
			switch b.activeField {
			case 0:
				b.pluginIndex = (b.pluginIndex + 1) % len(pluginList)
				b.applyPlugin()
			case 1:
				models := modelsByPlugin[pluginList[b.pluginIndex]]
				if len(models) > 0 {
					b.modelIndex = (b.modelIndex + 1) % len(models)
					b.applyModel()
				}
			}

		case key.Matches(msg, keys.AddStep):
			id := fmt.Sprintf("step%d", len(b.inner.Steps())+1)
			b.inner.AddStep(pipeline.Step{ID: id, Plugin: pluginList[0]})
			b.inner.SelectStep(len(b.inner.Steps()) - 1)
			b.activeField = 0
			b.pluginIndex = 0
			b.modelIndex = 0

		case key.Matches(msg, keys.Save):
			home, err := os.UserHomeDir()
			if err == nil {
				dir := filepath.Join(home, ".config", "orcai", "pipelines")
				os.MkdirAll(dir, 0o755) //nolint:errcheck
				path := filepath.Join(dir, b.inner.Name()+".pipeline.yaml")
				Save(b.inner, path) //nolint:errcheck
			}

		default:
			if b.activeField == 2 && len(b.inner.Steps()) > 0 {
				var cmd tea.Cmd
				b.promptInput, cmd = b.promptInput.Update(msg)
				idx := b.inner.SelectedIndex()
				s := b.inner.Steps()[idx]
				s.Prompt = b.promptInput.Value()
				b.inner.UpdateStep(idx, s)
				return b, cmd
			}
		}
	}
	return b, nil
}

func (b *BubbleModel) View() string {
	if b.width == 0 {
		return "Loading..."
	}

	w := b.width * 80 / 100
	h := b.height * 80 / 100
	leftW := w * 30 / 100
	rightW := w - leftW - 4

	// Left pane: step list.
	leftContent := titleStyle.Render("STEPS") + "\n" + strings.Repeat("─", leftW-2) + "\n"
	for i, s := range b.inner.Steps() {
		label := fmt.Sprintf("[%d] %s", i+1, stepLabel(s))
		if i == b.inner.SelectedIndex() {
			leftContent += selectedStep.Render("→ "+label) + "\n"
		} else {
			leftContent += dimStep.Render("  "+label) + "\n"
		}
	}
	leftContent += "\n" + dimStep.Render("[+] add step")

	// Right pane: config for selected step.
	rightContent := ""
	steps := b.inner.Steps()
	if len(steps) > 0 {
		sel := steps[b.inner.SelectedIndex()]
		rightContent = titleStyle.Render(fmt.Sprintf("STEP %d — CONFIG", b.inner.SelectedIndex()+1)) + "\n"
		rightContent += strings.Repeat("─", rightW-2) + "\n"
		rightContent += labelStyle.Render("ID:      ") + sel.ID + "\n"
		rightContent += labelStyle.Render("Plugin:  ") + sel.Plugin + "\n"
		rightContent += labelStyle.Render("Model:   ") + sel.Model + "\n"
		rightContent += labelStyle.Render("Prompt:  ") + sel.Prompt + "\n"
		if sel.Condition.If != "" {
			rightContent += labelStyle.Render("Cond:    ") + sel.Condition.If + "\n"
			rightContent += labelStyle.Render("  then→  ") + sel.Condition.Then + "\n"
			rightContent += labelStyle.Render("  else→  ") + sel.Condition.Else + "\n"
		}
	}

	left := lipgloss.NewStyle().Width(leftW).Height(h - 6).Render(leftContent)
	right := lipgloss.NewStyle().Width(rightW).Height(h - 6).Render(rightContent)
	panes := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)

	header := titleStyle.Render("PIPELINE BUILDER") +
		lipgloss.NewStyle().Width(w-20).Render("") +
		dimStep.Render("[?] help  [x]")
	nameRow := labelStyle.Render("NAME: ") + b.inner.Name()
	footer := statusBar.Render("[r] run  [s] save  [tab] next field  [↑↓] steps  [esc] quit")

	modal := borderStyle.Width(w).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			header,
			nameRow,
			strings.Repeat("═", w-4),
			panes,
			strings.Repeat("═", w-4),
			footer,
		),
	)

	marginLeft := (b.width - w) / 2
	marginTop := (b.height - h) / 2
	return lipgloss.NewStyle().
		MarginLeft(marginLeft).
		MarginTop(marginTop).
		Render(modal)
}

func stepLabel(s pipeline.Step) string {
	if s.Type != "" {
		return s.Type
	}
	if s.Plugin != "" {
		return s.Plugin
	}
	return s.ID
}
