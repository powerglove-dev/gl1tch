package promptbuilder

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// MultiSelect is a multi-select dropdown. Space toggles; Enter confirms; Escape reverts.
type MultiSelect struct {
	Dropdown
	checked  map[int]bool
	prevChk  map[int]bool
}

func NewMultiSelect(items []string, separators map[int]bool) MultiSelect {
	return MultiSelect{
		Dropdown: NewDropdown(items, separators),
		checked:  make(map[int]bool),
		prevChk:  make(map[int]bool),
	}
}

func (ms *MultiSelect) Open() {
	// save snapshot of checked for revert
	ms.prevChk = make(map[int]bool, len(ms.checked))
	for k, v := range ms.checked {
		ms.prevChk[k] = v
	}
	ms.Dropdown.Open()
}

// Update handles keys. Returns confirmed=true when Enter pressed.
func (ms *MultiSelect) Update(msg tea.KeyMsg) (confirmed bool) {
	if !ms.open {
		return false
	}
	switch {
	case msg.Type == tea.KeyUp:
		ms.moveUp()
	case msg.Type == tea.KeyDown:
		ms.moveDown()
	case msg.String() == " ":
		ms.checked[ms.selected] = !ms.checked[ms.selected]
	case msg.Type == tea.KeyEnter:
		ms.open = false
		return true
	case msg.Type == tea.KeyEsc:
		ms.checked = ms.prevChk
		ms.open = false
	}
	return false
}

// Selected returns the string values of all checked items.
func (ms MultiSelect) Selected() []string {
	var out []string
	for i, item := range ms.items {
		if ms.checked[i] && !ms.separators[i] {
			out = append(out, item)
		}
	}
	return out
}

// SetChecked marks the items matching vals as checked.
func (ms *MultiSelect) SetChecked(vals []string) {
	ms.checked = make(map[int]bool)
	for i, item := range ms.items {
		for _, v := range vals {
			if item == v && !ms.separators[i] {
				ms.checked[i] = true
			}
		}
	}
}

// View renders the multi-select dropdown.
func (ms MultiSelect) View(label string, width int, p palette) string {
	val := strings.Join(ms.Selected(), ", ")
	if val == "" {
		val = "—"
	}
	closedLine := p.blue + "  " + label + ": " + p.reset + p.pink + "[" + val + "]" + p.reset
	if !ms.open {
		return closedLine
	}

	maxW := width - 4
	if maxW < 10 {
		maxW = 10
	}

	var sb strings.Builder
	sb.WriteString(closedLine + "\n")
	sb.WriteString(p.purple + "  ┌" + strings.Repeat("─", maxW) + "┐" + p.reset + "\n")

	end := ms.scrollOff + ddMaxVisible
	if end > len(ms.items) {
		end = len(ms.items)
	}

	for i := ms.scrollOff; i < end; i++ {
		if ms.separators[i] {
			sb.WriteString(p.purple + "  │" + p.dim + strings.Repeat("─", maxW) + p.reset + p.purple + "│" + p.reset + "\n")
			continue
		}
		item := ms.items[i]
		check := "  "
		col := p.dim
		if ms.checked[i] {
			check = "✓ "
			col = p.pink
		}
		if i == ms.selected {
			col = p.pink
		}
		if len([]rune(item)) > maxW-4 {
			item = string([]rune(item)[:maxW-5]) + "…"
		}
		pad := strings.Repeat(" ", maxW-4-len([]rune(item)))
		sb.WriteString(p.purple + "  │ " + col + check + item + pad + p.reset + p.purple + "│" + p.reset + "\n")
	}
	sb.WriteString(p.purple + "  └" + strings.Repeat("─", maxW) + "┘" + p.reset)
	return sb.String()
}
