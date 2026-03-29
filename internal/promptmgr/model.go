package promptmgr

// Model is the BubbleTea model for the prompt manager TUI.
// It has three panels:
//   - left: prompt list with fuzzy search
//   - right-top: prompt editor (title, body, model selector)
//   - right-bottom: test runner output viewport

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"

	"github.com/adam-stokes/orcai/internal/modal"
	"github.com/adam-stokes/orcai/internal/plugin"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/themes"
	"github.com/adam-stokes/orcai/internal/tuikit"
)

// Model holds all state for the prompt manager TUI.
type Model struct {
	store      *store.Store
	pluginMgr  *plugin.Manager
	themeState tuikit.ThemeState

	width, height int

	// List state
	prompts     []store.Prompt // full list from store
	filtered    []store.Prompt // after search filter
	selectedIdx int            // cursor in filtered list
	scrollOffset int           // for list scrolling

	// Input widgets
	filterInput textinput.Model
	titleInput  textinput.Model
	bodyInput   textarea.Model

	// Dir picker (replaces cwdInput text field)
	dirPicker       modal.DirPickerModel
	dirPickerActive bool

	// Model selector
	modelSlugs []string // available model slugs
	modelIdx   int      // selected model index

	// Editor state
	editingPrompt  store.Prompt // prompt currently being edited (zero value = new)
	editorSubFocus int          // 0=title, 1=body, 2=cwd

	// Panel focus: 0=list, 1=editor, 2=runner
	focusPanel int

	// Runner state
	runnerOutput       string
	runnerStreaming     bool
	runnerScrollOffset int
	runnerErrMsg       string
	runCancel          context.CancelFunc // cancel func for active run; nil if not running

	// Overlay / status
	confirmDelete bool   // whether delete confirmation overlay is showing
	statusMsg     string // transient status line message
}

// New creates a new prompt manager model seeded with the given store, plugin
// manager, and theme bundle.
func New(st *store.Store, pluginMgr *plugin.Manager, bundle *themes.Bundle) *Model {
	fi := textinput.New()
	fi.Placeholder = "/ filter..."
	fi.CharLimit = 128

	ti := textinput.New()
	ti.Placeholder = "Prompt title..."
	ti.CharLimit = 256

	bi := textarea.New()
	bi.Placeholder = "Prompt body..."

	return &Model{
		store:       st,
		pluginMgr:   pluginMgr,
		themeState:  tuikit.NewThemeState(bundle),
		filterInput: fi,
		titleInput:  ti,
		bodyInput:   bi,
		dirPicker:   modal.NewDirPickerModel(),
	}
}

// Init starts the theme subscription and kicks off initial data loads.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.themeState.Init(),
		loadPromptsCmd(m.store),
		loadModelSlugsCmd(m.pluginMgr),
	)
}
