package switchboard

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// DirSelectedMsg is emitted when the user confirms a directory selection.
type DirSelectedMsg struct{ Path string }

// DirCancelledMsg is emitted when the user dismisses the dir picker without selecting.
type DirCancelledMsg struct{}

// dirWalkResultMsg carries a batch of discovered directories from the async walk.
type dirWalkResultMsg struct{ dirs []string }

const (
	dirPickerMaxDepth   = 3
	dirPickerMaxResults = 50
)

// DirPickerModel is a reusable BubbleTea component for fuzzy directory selection.
// Callers embed it in their own model and delegate Update/View when active.
type DirPickerModel struct {
	input   textinput.Model
	allDirs []string
	shown   []string // filtered + ranked subset, capped at dirPickerMaxResults
	cursor  int
	walking bool
}

// NewDirPickerModel returns an initialized DirPickerModel ready to use.
func NewDirPickerModel() DirPickerModel {
	ti := textinput.New()
	ti.Placeholder = "type to filter…"
	ti.CharLimit = 200
	ti.Focus()
	return DirPickerModel{
		input:   ti,
		walking: true,
	}
}

// DirPickerInit returns the tea.Cmd that starts the async home directory walk.
// Call this from your model's Init() when the dir picker becomes active.
func DirPickerInit() tea.Cmd {
	return walkHomeCmd()
}

// walkHomeCmd returns a tea.Cmd that walks ~/  up to dirPickerMaxDepth and
// sends back all discovered directories as a dirWalkResultMsg.
func walkHomeCmd() tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return dirWalkResultMsg{}
		}
		var dirs []string
		var walk func(dir string, depth int)
		walk = func(dir string, depth int) {
			if depth > dirPickerMaxDepth {
				return
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return
			}
			for _, e := range entries {
				if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				full := filepath.Join(dir, e.Name())
				dirs = append(dirs, full)
				walk(full, depth+1)
			}
		}
		walk(home, 1)
		sort.Strings(dirs)
		return dirWalkResultMsg{dirs: dirs}
	}
}

// fuzzyScore returns a relevance score for how well query matches the path s.
// Returns 0 if there is no match. Higher scores are better.
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
	// Contiguous match anywhere in the full path.
	if idx := strings.Index(sLow, qLow); idx >= 0 {
		return 1000 + len(qLow)*5 - idx/10
	}
	// Fuzzy: all query chars appear in order anywhere in the path.
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

// applyFilter recomputes m.shown from m.allDirs based on the current query.
func (m *DirPickerModel) applyFilter() {
	query := m.input.Value()
	if query == "" {
		end := min(len(m.allDirs), dirPickerMaxResults)
		m.shown = m.allDirs[:end]
		m.cursor = 0
		return
	}
	type scored struct {
		dir   string
		score int
	}
	results := make([]scored, 0, 64)
	for _, d := range m.allDirs {
		if s := fuzzyScore(d, query); s > 0 {
			results = append(results, scored{d, s})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].dir < results[j].dir
	})
	end := min(len(results), dirPickerMaxResults)
	m.shown = make([]string, end)
	for i := range end {
		m.shown[i] = results[i].dir
	}
	m.cursor = 0
}

// Update handles tea messages for the dir picker.
// The caller should route all key events and dirWalkResultMsg here when the
// dir picker is active.
func (m DirPickerModel) Update(msg tea.Msg) (DirPickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case dirWalkResultMsg:
		m.allDirs = msg.dirs
		m.walking = false
		m.applyFilter()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.shown)-1 {
				m.cursor++
			}
			return m, nil
		case "enter":
			if len(m.shown) > 0 && m.cursor < len(m.shown) {
				selected := m.shown[m.cursor]
				return m, func() tea.Msg { return DirSelectedMsg{Path: selected} }
			}
			return m, nil
		case "esc":
			return m, func() tea.Msg { return DirCancelledMsg{} }
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.applyFilter()
			return m, cmd
		}
	}
	return m, nil
}

// viewDirPickerBox renders the dir picker as a box string suitable for
// use with overlayCenter. w is the total available terminal width.
func (m DirPickerModel) viewDirPickerBox(w int) string {
	modalW := min(max(w-4, 60), 80)
	if w < 62 {
		modalW = w
	}

	borderColor := aPur
	var rows []string
	rows = append(rows, boxTop(modalW, "DIRECTORY", borderColor, borderColor))

	// Query input row.
	inputView := "  " + m.input.View()
	rows = append(rows, boxRow(inputView, modalW, borderColor))
	rows = append(rows, boxRow("", modalW, borderColor))

	if m.walking && len(m.allDirs) == 0 {
		rows = append(rows, boxRow(aDim+"  scanning directories…"+aRst, modalW, borderColor))
	} else if len(m.shown) == 0 {
		rows = append(rows, boxRow(aDim+"  no matches"+aRst, modalW, borderColor))
	} else {
		// Show up to 12 results in the visible window.
		const visWindow = 12
		offset := m.cursor - visWindow + 1
		if offset < 0 {
			offset = 0
		}
		if m.cursor < offset {
			offset = m.cursor
		}
		end := min(offset+visWindow, len(m.shown))

		for i := offset; i < end; i++ {
			d := m.shown[i]
			// Trim to home-relative display (~/...).
			home, _ := os.UserHomeDir()
			display := d
			if home != "" && strings.HasPrefix(d, home) {
				display = "~" + d[len(home):]
			}
			// Truncate long paths.
			maxLen := modalW - 6
			if len(display) > maxLen {
				display = "…" + display[len(display)-maxLen+1:]
			}

			if i == m.cursor {
				content := aPur + aBld + "  > " + aRst + aWht + display + aRst
				visLen := 4 + len(display)
				rows = append(rows, borderColor+"│"+content+strings.Repeat(" ", max(modalW-2-visLen, 0))+borderColor+"│"+aRst)
			} else {
				content := aDim + "    " + aRst + display
				rows = append(rows, boxRow(content, modalW, borderColor))
			}
		}
	}

	rows = append(rows, boxRow("", modalW, borderColor))
	hint := "  " + aPur + "↑↓" + aDim + " nav · " + aRst + aPur + "enter" + aDim + " select · " + aRst + aPur + "esc" + aDim + " cancel" + aRst
	rows = append(rows, boxRow(hint, modalW, borderColor))
	rows = append(rows, boxBot(modalW, borderColor))

	return strings.Join(rows, "\n")
}
