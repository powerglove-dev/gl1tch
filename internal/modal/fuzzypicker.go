package modal

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/panelrender"
	"github.com/8op-org/gl1tch/internal/styles"
)

// fuzzyScore returns a relevance score for how well query matches s.
// Returns 0 if there is no match. Higher scores are better.
// Shared by FuzzyPickerModel and DirPickerModel.
func fuzzyScore(s, query string) int {
	if query == "" {
		return 1
	}
	sLow := strings.ToLower(s)
	qLow := strings.ToLower(query)

	// Contiguous match in the base name gets the highest score.
	base := strings.ToLower(filepath.Base(s))
	if idx := strings.Index(base, qLow); idx >= 0 {
		return 2000 + len(qLow)*10 - idx
	}
	// Contiguous match anywhere in the full string.
	if idx := strings.Index(sLow, qLow); idx >= 0 {
		return 1000 + len(qLow)*5 - idx/10
	}
	// Fuzzy: all query chars appear in order anywhere in s.
	qi := 0
	for _, c := range sLow {
		if qi < len(qLow) && c == rune(qLow[qi]) {
			qi++
		}
	}
	if qi == len(qLow) {
		return 1
	}
	return 0
}

// FuzzyPickerEvent signals the result of a key event in the picker.
type FuzzyPickerEvent int

const (
	FuzzyPickerNone      FuzzyPickerEvent = iota
	FuzzyPickerConfirmed                  // enter was pressed; use SelectedItem / SelectedOriginalIdx
	FuzzyPickerCancelled                  // esc was pressed
)

// FuzzyPickerModel is a reusable inline fuzzy picker for static string lists.
// Call Open to activate it; route all key events to Update while IsOpen is true.
type FuzzyPickerModel struct {
	input       textinput.Model
	items       []string // full item list set via Open / SetItems
	shown       []string // filtered subset
	shownOrig   []int    // original indices of shown items in items
	cursor      int
	open        bool
	maxVisible  int
}

// NewFuzzyPickerModel returns an initialized FuzzyPickerModel.
func NewFuzzyPickerModel(maxVisible int) FuzzyPickerModel {
	ti := textinput.New()
	ti.Placeholder = "type to filter…"
	ti.CharLimit = 200
	if maxVisible < 1 {
		maxVisible = 8
	}
	return FuzzyPickerModel{input: ti, maxVisible: maxVisible}
}

// Open activates the picker with the given items, resetting filter and cursor.
func (m *FuzzyPickerModel) Open(items []string) {
	m.items = items
	m.input.SetValue("")
	m.input.Focus()
	m.cursor = 0
	m.open = true
	m.applyFilter()
}

// Close deactivates the picker.
func (m *FuzzyPickerModel) Close() {
	m.open = false
	m.input.Blur()
}

// IsOpen reports whether the picker is active.
func (m FuzzyPickerModel) IsOpen() bool { return m.open }

// SelectedItem returns the string at the current cursor position in the filtered list.
func (m FuzzyPickerModel) SelectedItem() string {
	if m.cursor < 0 || m.cursor >= len(m.shown) {
		return ""
	}
	return m.shown[m.cursor]
}

// SelectedOriginalIdx returns the index of the selected item in the original items slice.
func (m FuzzyPickerModel) SelectedOriginalIdx() int {
	if m.cursor < 0 || m.cursor >= len(m.shownOrig) {
		return 0
	}
	return m.shownOrig[m.cursor]
}

// SetItems updates the item list without opening the picker.
func (m *FuzzyPickerModel) SetItems(items []string) {
	m.items = items
	if m.open {
		m.applyFilter()
	}
}

// applyFilter recomputes m.shown from m.items using the current filter query.
func (m *FuzzyPickerModel) applyFilter() {
	query := m.input.Value()
	if query == "" {
		end := len(m.items)
		m.shown = make([]string, end)
		m.shownOrig = make([]int, end)
		for i, it := range m.items {
			m.shown[i] = it
			m.shownOrig[i] = i
		}
		m.cursor = 0
		return
	}
	type scored struct {
		item  string
		orig  int
		score int
	}
	results := make([]scored, 0, len(m.items))
	for i, it := range m.items {
		if s := fuzzyScore(it, query); s > 0 {
			results = append(results, scored{it, i, s})
		}
	}
	// stable sort: higher score first, then original order
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].score > results[j-1].score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
	m.shown = make([]string, len(results))
	m.shownOrig = make([]int, len(results))
	for i, r := range results {
		m.shown[i] = r.item
		m.shownOrig[i] = r.orig
	}
	m.cursor = 0
}

// Update handles tea messages for the fuzzy picker.
// Returns the updated model, an event signal, and an optional command.
func (m FuzzyPickerModel) Update(msg tea.Msg) (FuzzyPickerModel, FuzzyPickerEvent, tea.Cmd) {
	if !m.open {
		return m, FuzzyPickerNone, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, FuzzyPickerNone, nil
	}
	key := keyMsg.String()
	queryEmpty := m.input.Value() == ""

	switch {
	case key == "up" || (key == "k" && queryEmpty):
		if m.cursor > 0 {
			m.cursor--
		}
		return m, FuzzyPickerNone, nil

	case key == "down" || (key == "j" && queryEmpty):
		if m.cursor < len(m.shown)-1 {
			m.cursor++
		}
		return m, FuzzyPickerNone, nil

	case key == "enter":
		if len(m.shown) > 0 {
			m.open = false
			m.input.Blur()
			return m, FuzzyPickerConfirmed, nil
		}
		return m, FuzzyPickerNone, nil

	case key == "esc":
		m.open = false
		m.input.Blur()
		return m, FuzzyPickerCancelled, nil

	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(keyMsg)
		m.applyFilter()
		return m, FuzzyPickerNone, cmd
	}
}

// ViewBox renders the picker as a standalone bordered box suitable for use
// with panelrender.OverlayCenter. w is the total available terminal width.
func (m FuzzyPickerModel) ViewBox(w int, pal styles.ANSIPalette) string {
	boxW := min(max(w-4, 40), 72)
	if w < 44 {
		boxW = w
	}

	const rst = "\x1b[0m"
	const bld = "\x1b[1m"

	borderColor := pal.Border
	accent := pal.Accent
	dim := pal.Dim
	fg := pal.FG

	var rows []string
	rows = append(rows, panelrender.BoxTop(boxW, "SAVED PROMPT", borderColor, accent))

	// Filter input row.
	inputView := "  " + m.input.View()
	rows = append(rows, panelrender.BoxRow(inputView, boxW, borderColor))
	rows = append(rows, panelrender.BoxRow("", boxW, borderColor))

	if len(m.shown) == 0 {
		rows = append(rows, panelrender.BoxRow(dim+"  no matches"+rst, boxW, borderColor))
	} else {
		visible := m.maxVisible
		offset := m.cursor - visible + 1
		if offset < 0 {
			offset = 0
		}
		if m.cursor < offset {
			offset = m.cursor
		}
		end := offset + visible
		if end > len(m.shown) {
			end = len(m.shown)
		}
		for i := offset; i < end; i++ {
			item := m.shown[i]
			maxLen := boxW - 6
			if len([]rune(item)) > maxLen && maxLen > 1 {
				item = string([]rune(item)[:maxLen-1]) + "…"
			}
			if i == m.cursor {
				content := accent + bld + "  > " + rst + fg + item + rst
				visLen := 4 + len([]rune(item))
				rows = append(rows, borderColor+"│"+content+strings.Repeat(" ", max(boxW-2-visLen, 0))+borderColor+"│"+rst)
			} else {
				content := dim + "    " + rst + item
				rows = append(rows, panelrender.BoxRow(content, boxW, borderColor))
			}
		}
	}

	rows = append(rows, panelrender.BoxRow("", boxW, borderColor))
	hint := "  " + accent + "↑↓" + dim + " nav · " + rst + accent + "enter" + dim + " select · " + rst + accent + "esc" + dim + " cancel" + rst
	rows = append(rows, panelrender.BoxRow(hint, boxW, borderColor))
	rows = append(rows, panelrender.BoxBot(boxW, borderColor))

	return strings.Join(rows, "\n")
}

// ViewInline renders the picker as inline rows suitable for embedding in a parent
// panel layout. w is the available width; pal provides theme colors.
// Returns an empty string when the picker is closed.
func (m FuzzyPickerModel) ViewInline(w int, pal styles.ANSIPalette) string {
	if !m.open {
		return ""
	}

	const rst = "\x1b[0m"
	const bld = "\x1b[1m"

	borderColor := pal.Border
	accent := pal.Accent
	dim := pal.Dim
	fg := pal.FG

	var rows []string

	// Filter input row.
	inputView := "  " + m.input.View()
	rows = append(rows, panelrender.BoxRow(inputView, w, borderColor))
	rows = append(rows, panelrender.BoxRow("", w, borderColor))

	if len(m.shown) == 0 {
		rows = append(rows, panelrender.BoxRow(dim+"  no matches"+rst, w, borderColor))
	} else {
		visible := m.maxVisible
		offset := m.cursor - visible + 1
		if offset < 0 {
			offset = 0
		}
		if m.cursor < offset {
			offset = m.cursor
		}
		end := offset + visible
		if end > len(m.shown) {
			end = len(m.shown)
		}
		for i := offset; i < end; i++ {
			item := m.shown[i]
			maxLen := w - 6
			if len([]rune(item)) > maxLen && maxLen > 1 {
				item = string([]rune(item)[:maxLen-1]) + "…"
			}
			if i == m.cursor {
				content := accent + bld + "  > " + rst + fg + item + rst
				visLen := 4 + len([]rune(item))
				rows = append(rows, borderColor+"│"+content+strings.Repeat(" ", max(w-2-visLen, 0))+borderColor+"│"+rst)
			} else {
				content := dim + "    " + rst + item
				rows = append(rows, panelrender.BoxRow(content, w, borderColor))
			}
		}
	}

	rows = append(rows, panelrender.BoxRow("", w, borderColor))
	hint := "  " + accent + "↑↓" + dim + " nav · " + rst + accent + "enter" + dim + " select · " + rst + accent + "esc" + dim + " cancel" + rst
	rows = append(rows, panelrender.BoxRow(hint, w, borderColor))

	return strings.Join(rows, "\n")
}
