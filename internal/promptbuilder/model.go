package promptbuilder

import (
	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/pipeline"
)

// argsRow is one key/value pair in the args editor.
type argsRow struct {
	key   string
	value string
	// editingKey is true when the inline editor is focused on the key half.
	editingKey bool
}

// Model is the state for the prompt builder TUI.
type Model struct {
	name          string
	steps         []pipeline.Step
	selectedIndex int
	executorMgr     *executor.Manager
}

// New creates a new prompt builder model. executorMgr may be nil in tests.
func New(executorMgr *executor.Manager) *Model {
	return &Model{executorMgr: executorMgr}
}

func (m *Model) Name() string           { return m.name }
func (m *Model) SetName(name string)    { m.name = name }
func (m *Model) Steps() []pipeline.Step { return m.steps }
func (m *Model) SelectedIndex() int     { return m.selectedIndex }

// AddStep appends a step to the pipeline.
func (m *Model) AddStep(s pipeline.Step) {
	m.steps = append(m.steps, s)
}

// SelectStep sets the active step by index, clamped to valid range.
func (m *Model) SelectStep(i int) {
	if len(m.steps) == 0 {
		m.selectedIndex = 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(m.steps) {
		i = len(m.steps) - 1
	}
	m.selectedIndex = i
}

// UpdateStep replaces the step at index i. No-op if i is out of range.
func (m *Model) UpdateStep(i int, s pipeline.Step) {
	if i < 0 || i >= len(m.steps) {
		return
	}
	m.steps[i] = s
}

// ToPipeline converts the current model state to a Pipeline.
func (m *Model) ToPipeline() *pipeline.Pipeline {
	steps := make([]pipeline.Step, len(m.steps))
	copy(steps, m.steps)
	return &pipeline.Pipeline{
		Name:    m.name,
		Version: "1.0",
		Steps:   steps,
	}
}
