package modal

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/styles"
)

// ANSI escape constants used by the dir picker box renderer.
const (
	dpARst = "\x1b[0m" // reset
	dpABld = "\x1b[1m" // bold
)

// DirSelectedMsg is emitted when the user confirms a directory selection.
type DirSelectedMsg struct{ Path string }

// DirCancelledMsg is emitted when the user dismisses the dir picker without selecting.
type DirCancelledMsg struct{}

// DirWalkResultMsg carries a batch of discovered directories from the async walk.
// It is exported so that callers embedding DirPickerModel can forward it
// in their own Update type-switch when needed.
type DirWalkResultMsg struct{ Dirs []string }

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
			return DirWalkResultMsg{}
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
		return DirWalkResultMsg{Dirs: dirs}
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
	case DirWalkResultMsg:
		m.allDirs = msg.Dirs
		m.walking = false
		m.applyFilter()
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		queryEmpty := m.input.Value() == ""

		// Arrow keys always navigate; j/k only navigate when query is empty.
		switch {
		case key == "up" || (key == "k" && queryEmpty):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case key == "down" || (key == "j" && queryEmpty):
			if m.cursor < len(m.shown)-1 {
				m.cursor++
			}
			return m, nil
		case key == "enter":
			if len(m.shown) > 0 && m.cursor < len(m.shown) {
				selected := m.shown[m.cursor]
				return m, func() tea.Msg { return DirSelectedMsg{Path: selected} }
			}
			return m, nil
		case key == "esc":
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

// ViewDirPickerBox renders the dir picker as a box string suitable for
// use with panelrender.OverlayCenter. w is the total available terminal width.
// pal is the active theme palette so the box honors theme colors.
func (m DirPickerModel) ViewDirPickerBox(w int, pal styles.ANSIPalette) string {
	modalW := min(max(w-4, 60), 80)
	if w < 62 {
		modalW = w
	}

	borderColor := pal.Border
	accent := pal.Accent
	dim := pal.Dim
	fg := pal.FG

	var rows []string
	rows = append(rows, panelrender.BoxTop(modalW, "DIRECTORY", borderColor, accent))

	// Query input row.
	inputView := "  " + m.input.View()
	rows = append(rows, panelrender.BoxRow(inputView, modalW, borderColor))
	rows = append(rows, panelrender.BoxRow("", modalW, borderColor))

	if m.walking && len(m.allDirs) == 0 {
		rows = append(rows, panelrender.BoxRow(dim+"  scanning directories…"+dpARst, modalW, borderColor))
	} else if len(m.shown) == 0 {
		rows = append(rows, panelrender.BoxRow(dim+"  no matches"+dpARst, modalW, borderColor))
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
				content := accent + dpABld + "  > " + dpARst + fg + display + dpARst
				visLen := 4 + len(display)
				rows = append(rows, borderColor+"│"+content+strings.Repeat(" ", max(modalW-2-visLen, 0))+borderColor+"│"+dpARst)
			} else {
				content := dim + "    " + dpARst + display
				rows = append(rows, panelrender.BoxRow(content, modalW, borderColor))
			}
		}
	}

	rows = append(rows, panelrender.BoxRow("", modalW, borderColor))
	hint := "  " + accent + "↑↓" + dim + " nav · " + dpARst + accent + "enter" + dim + " select · " + dpARst + accent + "esc" + dim + " cancel" + dpARst
	rows = append(rows, panelrender.BoxRow(hint, modalW, borderColor))
	rows = append(rows, panelrender.BoxBot(modalW, borderColor))

	return strings.Join(rows, "\n")
}
