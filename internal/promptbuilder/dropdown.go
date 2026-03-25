package promptbuilder

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Dropdown is an inline overlay dropdown component for the BubbleTea TUI.
// Uses ABBS Dracula palette; renders as a bordered overlay below the field label.
type Dropdown struct {
	items      []string
	separators map[int]bool
	selected   int
	prev       int  // saved selection before open, for revert on Escape
	open       bool
	scrollOff  int // index of first visible item in the overlay
}

const ddMaxVisible = 8

func NewDropdown(items []string, separators map[int]bool) Dropdown {
	d := Dropdown{items: items, separators: separators}
	if d.separators == nil {
		d.separators = make(map[int]bool)
	}
	// advance past any leading separator
	for d.selected < len(d.items) && d.separators[d.selected] {
		d.selected++
	}
	return d
}

func (d *Dropdown) Open()   { d.prev = d.selected; d.open = true }
func (d *Dropdown) Close()  { d.open = false }
func (d *Dropdown) Toggle() { if d.open { d.Close() } else { d.Open() } }
func (d Dropdown) IsOpen() bool { return d.open }
func (d Dropdown) Selected() int { return d.selected }

// Value returns the currently selected item string, or "" if empty.
func (d Dropdown) Value() string {
	if d.selected < 0 || d.selected >= len(d.items) {
		return ""
	}
	return d.items[d.selected]
}

// SetSelected sets the selection by index, skipping separators.
func (d *Dropdown) SetSelected(i int) {
	if i < 0 || i >= len(d.items) {
		return
	}
	d.selected = i
	for d.selected < len(d.items) && d.separators[d.selected] {
		d.selected++
	}
}

// SetValue selects the item matching val; no-op if not found.
func (d *Dropdown) SetValue(val string) {
	for i, it := range d.items {
		if it == val && !d.separators[i] {
			d.selected = i
			return
		}
	}
}

// Update handles keyboard input when the dropdown is open.
// Returns true if the selection changed and was confirmed (Enter).
func (d *Dropdown) Update(msg tea.KeyMsg) (confirmed bool, changed bool) {
	if !d.open {
		return false, false
	}
	switch msg.Type {
	case tea.KeyUp:
		d.moveUp()
	case tea.KeyDown:
		d.moveDown()
	case tea.KeyEnter:
		d.open = false
		return true, d.selected != d.prev
	case tea.KeyEsc:
		d.selected = d.prev
		d.open = false
	}
	return false, false
}

func (d *Dropdown) moveDown() {
	next := d.selected + 1
	for next < len(d.items) && d.separators[next] {
		next++
	}
	if next < len(d.items) {
		d.selected = next
		if d.selected >= d.scrollOff+ddMaxVisible {
			d.scrollOff = d.selected - ddMaxVisible + 1
		}
	}
}

func (d *Dropdown) moveUp() {
	next := d.selected - 1
	for next >= 0 && d.separators[next] {
		next--
	}
	if next >= 0 {
		d.selected = next
		if d.selected < d.scrollOff {
			d.scrollOff = d.selected
		}
	}
}

// View renders the dropdown.
// closed: "  label : [value]"
// open: closed line + overlay box below
func (d Dropdown) View(label string, width int, p palette) string {
	// Closed line
	val := d.Value()
	if val == "" {
		val = "—"
	}
	closedLine := p.blue + "  " + label + ": " + p.reset + p.pink + "[" + val + "]" + p.reset
	if !d.open {
		return closedLine
	}

	// Overlay box
	maxW := width - 4
	if maxW < 10 {
		maxW = 10
	}

	var sb strings.Builder
	sb.WriteString(closedLine + "\n")

	// top border
	sb.WriteString(p.purple + "  ┌" + strings.Repeat("─", maxW) + "┐" + p.reset + "\n")

	end := d.scrollOff + ddMaxVisible
	if end > len(d.items) {
		end = len(d.items)
	}

	for i := d.scrollOff; i < end; i++ {
		if d.separators[i] {
			sb.WriteString(p.purple + "  │" + p.dim + strings.Repeat("─", maxW) + p.reset + p.purple + "│" + p.reset + "\n")
			continue
		}
		item := d.items[i]
		if len([]rune(item)) > maxW-2 {
			item = string([]rune(item)[:maxW-3]) + "…"
		}
		pad := strings.Repeat(" ", maxW-2-len([]rune(item)))
		if i == d.selected {
			sb.WriteString(p.purple + "  │ " + p.pink + "▶ " + item + pad + p.reset + p.purple + "│" + p.reset + "\n")
		} else {
			sb.WriteString(p.purple + "  │ " + p.dim + "  " + item + pad + p.reset + p.purple + "│" + p.reset + "\n")
		}
	}

	// bottom border
	sb.WriteString(p.purple + "  └" + strings.Repeat("─", maxW) + "┘" + p.reset)
	return sb.String()
}
