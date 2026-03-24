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

	"github.com/adam-stokes/orcai/internal/picker"
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

// BubbleModel wraps Model and implements tea.Model.
type BubbleModel struct {
	inner       *Model
	width       int
	height      int
	providers   []picker.ProviderDef
	activeField int // 0=Plugin 1=Model 2=Prompt
	pluginIndex int
	modelIndex  int
	promptInput textinput.Model
}

// NewBubble creates a bubbletea-compatible model.
func NewBubble(m *Model, providers []picker.ProviderDef) *BubbleModel {
	ti := textinput.New()
	ti.Placeholder = "enter prompt..."
	return &BubbleModel{inner: m, providers: providers, promptInput: ti}
}

func (b *BubbleModel) Init() tea.Cmd { return nil }

func (b *BubbleModel) syncIndicesFromStep() {
	b.pluginIndex = 0
	b.modelIndex = 0
	steps := b.inner.Steps()
	if len(steps) == 0 || len(b.providers) == 0 {
		return
	}
	sel := steps[b.inner.SelectedIndex()]
	for i, p := range b.providers {
		if p.ID == sel.Plugin {
			b.pluginIndex = i
			break
		}
	}
	for i, mo := range b.providers[b.pluginIndex].Models {
		if !mo.Separator && mo.ID == sel.Model {
			b.modelIndex = i
			break
		}
	}
}

func (b *BubbleModel) applyPlugin() {
	steps := b.inner.Steps()
	if len(steps) == 0 || len(b.providers) == 0 {
		return
	}
	idx := b.inner.SelectedIndex()
	s := steps[idx]
	s.Plugin = b.providers[b.pluginIndex].ID
	b.modelIndex = 0
	s.Model = ""
	for _, mo := range b.providers[b.pluginIndex].Models {
		if !mo.Separator {
			s.Model = mo.ID
			break
		}
	}
	b.inner.UpdateStep(idx, s)
}

func (b *BubbleModel) applyModel() {
	steps := b.inner.Steps()
	if len(steps) == 0 || len(b.providers) == 0 {
		return
	}
	idx := b.inner.SelectedIndex()
	s := steps[idx]
	models := b.providers[b.pluginIndex].Models
	if b.modelIndex < len(models) && !models[b.modelIndex].Separator {
		s.Model = models[b.modelIndex].ID
	}
	b.inner.UpdateStep(idx, s)
}

// renderSelector renders a cycle-selector field. Active fields show ◀ value ▶.
func (b *BubbleModel) renderSelector(label, value string, fieldIdx int) string {
	l := labelStyle.Render(label)
	if b.activeField == fieldIdx && len(b.inner.Steps()) > 0 {
		return l + selectedStep.Render("◀ "+value+" ▶") + "\n"
	}
	return l + value + "\n"
}

func (b *BubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.width = msg.Width
		b.height = msg.Height
	case tea.KeyMsg:
		// Prompt field (activeField==2): intercept only structural non-printable
		// keys via msg.Type. Everything else — including action-bound runes like
		// 's', 'r', 'q', 'k', 'j', '+' — must reach the textinput unchanged.
		if b.activeField == 2 && len(b.inner.Steps()) > 0 {
			switch msg.Type {
			case tea.KeyEsc:
				return b, tea.Quit
			case tea.KeyTab:
				b.activeField = (b.activeField + 1) % 3
				b.promptInput.Blur()
			case tea.KeyShiftTab:
				b.activeField = (b.activeField + 2) % 3
				b.promptInput.Blur()
			case tea.KeyUp:
				b.inner.SelectStep(b.inner.SelectedIndex() - 1)
				b.activeField = 0
				b.promptInput.Blur()
				b.syncIndicesFromStep()
			case tea.KeyDown:
				b.inner.SelectStep(b.inner.SelectedIndex() + 1)
				b.activeField = 0
				b.promptInput.Blur()
				b.syncIndicesFromStep()
			default:
				var cmd tea.Cmd
				b.promptInput, cmd = b.promptInput.Update(msg)
				idx := b.inner.SelectedIndex()
				s := b.inner.Steps()[idx]
				s.Prompt = b.promptInput.Value()
				b.inner.UpdateStep(idx, s)
				return b, cmd
			}
			return b, nil
		}

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
			if len(b.inner.Steps()) == 0 || len(b.providers) == 0 {
				break
			}
			switch b.activeField {
			case 0:
				b.pluginIndex = (b.pluginIndex - 1 + len(b.providers)) % len(b.providers)
				b.applyPlugin()
			case 1:
				models := b.providers[b.pluginIndex].Models
				if len(models) == 0 {
					break
				}
				next := (b.modelIndex - 1 + len(models)) % len(models)
				for models[next].Separator {
					next = (next - 1 + len(models)) % len(models)
				}
				b.modelIndex = next
				b.applyModel()
			}

		case key.Matches(msg, keys.Right):
			if len(b.inner.Steps()) == 0 || len(b.providers) == 0 {
				break
			}
			switch b.activeField {
			case 0:
				b.pluginIndex = (b.pluginIndex + 1) % len(b.providers)
				b.applyPlugin()
			case 1:
				models := b.providers[b.pluginIndex].Models
				if len(models) == 0 {
					break
				}
				next := (b.modelIndex + 1) % len(models)
				for models[next].Separator {
					next = (next + 1) % len(models)
				}
				b.modelIndex = next
				b.applyModel()
			}

		case key.Matches(msg, keys.AddStep):
			id := fmt.Sprintf("step%d", len(b.inner.Steps())+1)
			plugin := ""
			if len(b.providers) > 0 {
				plugin = b.providers[0].ID
			}
			b.inner.AddStep(pipeline.Step{ID: id, Plugin: plugin})
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

		pluginLabel := ""
		if len(b.providers) > 0 && b.pluginIndex < len(b.providers) {
			pluginLabel = b.providers[b.pluginIndex].Label
		}
		rightContent += b.renderSelector("Plugin:  ", pluginLabel, 0)

		modelLabel := "(none)"
		if len(b.providers) > 0 && b.pluginIndex < len(b.providers) {
			models := b.providers[b.pluginIndex].Models
			if b.modelIndex < len(models) && !models[b.modelIndex].Separator {
				modelLabel = models[b.modelIndex].Label
			} else if sel.Model != "" {
				modelLabel = sel.Model
			}
		}
		rightContent += b.renderSelector("Model:   ", modelLabel, 1)

		if b.activeField == 2 {
			rightContent += labelStyle.Render("Prompt:  ") + b.promptInput.View() + "\n"
		} else {
			rightContent += labelStyle.Render("Prompt:  ") + sel.Prompt + "\n"
		}
		if sel.Condition.If != "" {
			rightContent += labelStyle.Render("Cond:    ") + sel.Condition.If + "\n"
			rightContent += labelStyle.Render("  then→  ") + sel.Condition.Then + "\n"
			rightContent += labelStyle.Render("  else→  ") + sel.Condition.Else + "\n"
		}
	} else {
		rightContent = "\n\n" +
			dimStep.Render("  No steps yet.") + "\n\n" +
			dimStep.Render("  Press [+] to add your first step.") + "\n" +
			dimStep.Render("  Each step requires a provider (Plugin).") + "\n\n" +
			dimStep.Render("  Once a step is selected:") + "\n" +
			dimStep.Render("    [←→]  cycle Plugin or Model") + "\n" +
			dimStep.Render("    [tab] move between fields") + "\n" +
			dimStep.Render("    type  enter a Prompt")
	}

	left := lipgloss.NewStyle().Width(leftW).Height(h - 6).Render(leftContent)
	right := lipgloss.NewStyle().Width(rightW).Height(h - 6).Render(rightContent)
	panes := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)

	header := titleStyle.Render("PIPELINE BUILDER") +
		lipgloss.NewStyle().Width(w-20).Render("") +
		dimStep.Render("[?] help")
	nameRow := labelStyle.Render("NAME: ") + b.inner.Name()
	footer := statusBar.Render("[+] add  [←→] cycle  [tab] next field  [↑↓] steps  [s] save  [esc] quit")

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
