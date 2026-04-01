package promptbuilder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/powerglove-dev/gl1tch/internal/picker"
	"github.com/powerglove-dev/gl1tch/internal/pipeline"
)

// ── palette ──────────────────────────────────────────────────────────────────

type palette struct {
	purple, pink, bold, blue, dim, reset string
}

func defaultPalette() palette {
	return palette{
		purple: "\x1b[38;5;141m",
		pink:   "\x1b[38;5;212m",
		bold:   "\x1b[1;38;5;212m",
		blue:   "\x1b[38;5;61m",
		dim:    "\x1b[38;5;66m",
		reset:  "\x1b[0m",
	}
}

// ── builtin executor metadata ─────────────────────────────────────────────────

var builtinExecutors = []struct{ name, desc string }{
	{"builtin.assert", "evaluate condition, fail if false"},
	{"builtin.log", "write message to output"},
	{"builtin.sleep", "sleep for duration"},
	{"builtin.http_get", "HTTP GET request"},
	{"builtin.set_data", "merge map data into output"},
}

var builtinArgKeys = map[string][]string{
	"builtin.assert":   {"value", "condition"},
	"builtin.log":      {"message"},
	"builtin.sleep":    {"duration"},
	"builtin.http_get": {"url"},
}

// isBuiltin returns true if the executor ID starts with "builtin."
func isBuiltin(exec string) bool {
	return strings.HasPrefix(exec, "builtin.")
}

// ── BubbleModel ───────────────────────────────────────────────────────────────

// BubbleModel wraps Model and implements tea.Model.
type BubbleModel struct {
	inner     *Model
	width     int
	height    int
	providers []picker.ProviderDef
	palette   palette

	// Group navigation: 0=Core, 1=Execution, 2=Advanced
	activeGroup        int
	activeFieldInGroup int

	// Pane focus: 0=steps (left), 1=fields (right)
	focusPane int

	// Accordion state — true=expanded; index 0=Core, 1=Execution, 2=Advanced
	groupExpanded [3]bool

	// Right pane scroll offset (top line into rendered field content)
	rightScroll int

	// Left pane scroll offset
	leftScroll int

	// Core group (fields 0,1,2)
	executorDD   Dropdown
	modelDD      Dropdown
	modelEnabled bool
	promptInput  textinput.Model

	// Execution group (fields 0,1,2,3,4,5)
	needsDD            MultiSelect
	retryMaxInput      textinput.Model
	retryIntervalInput textinput.Model
	retryOnDD          Dropdown
	forEachInput       textinput.Model
	onFailureDD        Dropdown

	// Advanced group (fields 0,1,2,3,4)
	condIfInput    textinput.Model
	condThenDD     Dropdown
	condElseDD     Dropdown
	publishToInput textinput.Model
	argsRows        []argsRow
	argsSelected    int
	argsEditing     bool // true when inline editing an args row
	argsKeyInput    textinput.Model
	argsValueInput  textinput.Model
	argsEditingKey  bool // true=editing key half, false=editing value half

	// activeField is the flat index within the active group's fields
	activeField int

	// Pipeline name editing
	nameInput   textinput.Model
	editingName bool

	// Step ID editing
	stepIDInput   textinput.Model
	editingStepID bool

	// Help overlay
	showHelp bool

	// Status message (shown in footer, auto-cleared)
	statusMsg string
}

// groupFieldCount returns the number of fields in each group.
func (b *BubbleModel) groupFieldCount(g int) int {
	switch g {
	case 0:
		if b.modelEnabled {
			return 3 // executorDD, modelDD, promptInput
		}
		return 2 // executorDD, promptInput (modelDD hidden)
	case 1:
		return 6 // needsDD, retryMax, retryInterval, retryOnDD, forEachInput, onFailureDD
	case 2:
		return 5 // condIfInput, condThenDD, condElseDD, publishToInput, argsRows
	}
	return 1
}

// NewBubble creates a bubbletea-compatible model.
func NewBubble(m *Model, providers []picker.ProviderDef) *BubbleModel {
	b := &BubbleModel{
		inner:        m,
		providers:    providers,
		palette:      defaultPalette(),
		modelEnabled: true,
		groupExpanded: [3]bool{true, false, false},
		focusPane:    0,
	}

	// ── executor dropdown ──
	var execItems []string
	var execSeps map[int]bool
	execSeps = make(map[int]bool)
	for _, p := range providers {
		execItems = append(execItems, p.ID)
	}
	if len(providers) > 0 {
		execSeps[len(execItems)] = true
		execItems = append(execItems, "")
	}
	for _, bi := range builtinExecutors {
		execItems = append(execItems, bi.name)
	}
	b.executorDD = NewDropdown(execItems, execSeps)

	// ── model dropdown (initially for first provider) ──
	b.modelDD = b.buildModelDD(0)

	// ── retryOnDD ──
	b.retryOnDD = NewDropdown([]string{"always", "on_failure"}, nil)

	// ── needsDD ── (initially empty — rebuilt in syncFromStep)
	b.needsDD = NewMultiSelect(nil, nil)

	// ── onFailureDD ── (initially empty — rebuilt)
	b.onFailureDD = NewDropdown(nil, nil)

	// ── condThenDD / condElseDD ── (initially empty — rebuilt)
	b.condThenDD = NewDropdown(nil, nil)
	b.condElseDD = NewDropdown(nil, nil)

	// ── text inputs ──
	b.promptInput = makeInput("enter prompt...")
	b.retryMaxInput = makeInput("e.g. 3")
	b.retryIntervalInput = makeInput("e.g. 2s")
	b.forEachInput = makeInput("e.g. {{.items}}")
	b.condIfInput = makeInput("e.g. {{gt .score 0.5}}")
	b.publishToInput = makeInput("e.g. result_key")
	b.argsKeyInput = makeInput("key")
	b.argsValueInput = makeInput("value")

	// ── name input ──
	ni := textinput.New()
	ni.Placeholder = "pipeline name"
	ni.Width = 24
	ni.SetValue(m.Name())
	b.nameInput = ni

	// ── step ID input ──
	si := textinput.New()
	si.Placeholder = "step id"
	si.Width = 20
	b.stepIDInput = si

	// sync first step if any
	if len(m.Steps()) > 0 {
		b.syncFromStep(m.SelectedIndex())
	}

	return b
}

func makeInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	return ti
}

// buildModelDD builds a model dropdown for the given provider index.
func (b *BubbleModel) buildModelDD(provIdx int) Dropdown {
	if provIdx < 0 || provIdx >= len(b.providers) {
		return NewDropdown(nil, nil)
	}
	p := b.providers[provIdx]
	var items []string
	var seps map[int]bool
	seps = make(map[int]bool)
	for _, mo := range p.Models {
		if mo.Separator {
			seps[len(items)] = true
		}
		items = append(items, mo.ID)
	}
	return NewDropdown(items, seps)
}

// buildSiblingIDs returns a list of step IDs that are siblings of the current step.
func (b *BubbleModel) buildSiblingIDs(excludeIdx int) []string {
	var ids []string
	for i, s := range b.inner.Steps() {
		if i != excludeIdx {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// rebuildSiblingDropdowns rebuilds needsDD, onFailureDD, condThenDD, condElseDD
// with the current sibling step IDs, preserving checked/selected values.
func (b *BubbleModel) rebuildSiblingDropdowns(currentIdx int) {
	siblings := b.buildSiblingIDs(currentIdx)

	// needsDD — preserve checked
	prevNeeds := b.needsDD.Selected()
	b.needsDD = NewMultiSelect(siblings, nil)
	b.needsDD.SetChecked(prevNeeds)

	// onFailureDD — preserve value
	prevOnFail := b.onFailureDD.Value()
	b.onFailureDD = NewDropdown(append([]string{""}, siblings...), nil)
	if prevOnFail != "" {
		b.onFailureDD.SetValue(prevOnFail)
	}

	// condThenDD / condElseDD
	prevThen := b.condThenDD.Value()
	prevElse := b.condElseDD.Value()
	stepIDs := append([]string{""}, siblings...)
	b.condThenDD = NewDropdown(stepIDs, nil)
	b.condElseDD = NewDropdown(stepIDs, nil)
	if prevThen != "" {
		b.condThenDD.SetValue(prevThen)
	}
	if prevElse != "" {
		b.condElseDD.SetValue(prevElse)
	}
}

// syncFromStep reads state from the step at idx and populates all widgets.
func (b *BubbleModel) syncFromStep(idx int) {
	steps := b.inner.Steps()
	if idx < 0 || idx >= len(steps) {
		return
	}
	s := steps[idx]

	// step ID
	b.stepIDInput.SetValue(s.ID)

	// executor — use Executor field
	exec := s.Executor
	b.executorDD.SetValue(exec)

	if exec == "" || isBuiltin(exec) {
		b.modelEnabled = false
		b.modelDD = NewDropdown(nil, nil)
	} else {
		b.modelEnabled = true
		// find provider index
		provIdx := -1
		for i, p := range b.providers {
			if p.ID == exec {
				provIdx = i
				break
			}
		}
		if provIdx >= 0 {
			b.modelDD = b.buildModelDD(provIdx)
			b.modelDD.SetValue(s.Model)
		} else {
			b.modelDD = NewDropdown(nil, nil)
		}
	}

	// prompt
	b.promptInput.SetValue(s.Prompt)

	// needs — rebuild with sibling IDs
	b.rebuildSiblingDropdowns(idx)
	b.needsDD.SetChecked(s.Needs)

	// retry
	if s.Retry != nil {
		b.retryMaxInput.SetValue(fmt.Sprintf("%d", s.Retry.MaxAttempts))
		if s.Retry.Interval.Duration > 0 {
			b.retryIntervalInput.SetValue(s.Retry.Interval.Duration.String())
		} else {
			b.retryIntervalInput.SetValue("")
		}
		b.retryOnDD.SetValue(s.Retry.On)
	} else {
		b.retryMaxInput.SetValue("")
		b.retryIntervalInput.SetValue("")
		b.retryOnDD.SetSelected(0)
	}

	// forEachInput
	b.forEachInput.SetValue(s.ForEach)

	// onFailureDD
	b.onFailureDD.SetValue(s.OnFailure)

	// condition
	b.condIfInput.SetValue(s.Condition.If)
	b.condThenDD.SetValue(s.Condition.Then)
	b.condElseDD.SetValue(s.Condition.Else)

	// publishTo
	b.publishToInput.SetValue(s.PublishTo)

	// args
	b.argsRows = nil
	if len(s.Args) > 0 {
		for k, v := range s.Args {
			b.argsRows = append(b.argsRows, argsRow{key: k, value: fmt.Sprintf("%v", v)})
		}
	} else if isBuiltin(exec) {
		// pre-populate known arg keys for builtins
		if argKeys, ok := builtinArgKeys[exec]; ok {
			for _, k := range argKeys {
				b.argsRows = append(b.argsRows, argsRow{key: k})
			}
		}
	}
	b.argsSelected = 0
}

// applyToStep reads all widget state and writes it back to the current step.
func (b *BubbleModel) applyToStep() {
	idx := b.inner.SelectedIndex()
	steps := b.inner.Steps()
	if idx < 0 || idx >= len(steps) {
		return
	}
	s := steps[idx]

	// step ID
	if id := strings.TrimSpace(b.stepIDInput.Value()); id != "" {
		s.ID = id
	}

	exec := b.executorDD.Value()
	s.Executor = exec

	if b.modelEnabled {
		s.Model = b.modelDD.Value()
	} else {
		s.Model = ""
	}

	s.Prompt = b.promptInput.Value()
	s.Needs = b.needsDD.Selected()

	// retry
	maxStr := strings.TrimSpace(b.retryMaxInput.Value())
	intStr := strings.TrimSpace(b.retryIntervalInput.Value())
	if maxStr != "" || intStr != "" {
		if s.Retry == nil {
			s.Retry = &pipeline.RetryPolicy{}
		}
		if maxStr != "" {
			var n int
			fmt.Sscanf(maxStr, "%d", &n)
			s.Retry.MaxAttempts = n
		}
		if intStr != "" {
			// store as string; keep Duration zero if parse fails
			var dur pipeline.Duration
			if err := dur.UnmarshalYAML(nil); err == nil {
				s.Retry.Interval = dur
			}
		}
		s.Retry.On = b.retryOnDD.Value()
	} else {
		s.Retry = nil
	}

	s.ForEach = b.forEachInput.Value()
	s.OnFailure = b.onFailureDD.Value()
	s.Condition.If = b.condIfInput.Value()
	s.Condition.Then = b.condThenDD.Value()
	s.Condition.Else = b.condElseDD.Value()
	s.PublishTo = b.publishToInput.Value()

	// args
	if len(b.argsRows) > 0 {
		s.Args = make(map[string]any)
		for _, row := range b.argsRows {
			if row.key != "" {
				s.Args[row.key] = row.value
			}
		}
		if len(s.Args) == 0 {
			s.Args = nil
		}
	} else {
		s.Args = nil
	}
	s.Vars = nil //nolint:staticcheck // clear deprecated field

	b.inner.UpdateStep(idx, s)
}

// focusedInput returns the currently focused textinput (if any).
func (b *BubbleModel) focusedInput() *textinput.Model {
	switch b.activeGroup {
	case 0:
		fieldIdx := b.activeField
		if !b.modelEnabled {
			// field mapping: 0=executorDD, 1=promptInput
			if fieldIdx == 1 {
				return &b.promptInput
			}
		} else {
			if fieldIdx == 2 {
				return &b.promptInput
			}
		}
	case 1:
		switch b.activeField {
		case 1:
			return &b.retryMaxInput
		case 2:
			return &b.retryIntervalInput
		case 4:
			return &b.forEachInput
		}
	case 2:
		switch b.activeField {
		case 0:
			return &b.condIfInput
		case 3:
			return &b.publishToInput
		}
	}
	return nil
}

// activeFocusedInput returns the textinput that is currently focused (via inp.Focused()),
// or nil if no input is focused. This differs from focusedInput() which just checks
// the active field index.
func (b *BubbleModel) activeFocusedInput() *textinput.Model {
	inputs := []*textinput.Model{
		&b.promptInput,
		&b.retryMaxInput,
		&b.retryIntervalInput,
		&b.forEachInput,
		&b.condIfInput,
		&b.publishToInput,
	}
	for _, inp := range inputs {
		if inp.Focused() {
			return inp
		}
	}
	return nil
}

// isDropdownField returns true if the currently active field is a dropdown/multiselect.
func (b *BubbleModel) isDropdownField() bool {
	return b.focusedInput() == nil
}

// openDropdown opens the dropdown/multiselect for the current field (if applicable).
func (b *BubbleModel) openDropdown() {
	switch b.activeGroup {
	case 0:
		switch b.activeField {
		case 0:
			b.executorDD.Open()
		case 1:
			if b.modelEnabled {
				b.modelDD.Open()
			}
		}
	case 1:
		switch b.activeField {
		case 0:
			b.needsDD.Open()
		case 3:
			b.retryOnDD.Open()
		case 5:
			b.onFailureDD.Open()
		}
	case 2:
		switch b.activeField {
		case 1:
			b.condThenDD.Open()
		case 2:
			b.condElseDD.Open()
		}
	}
}

// anyDropdownOpen returns true if any dropdown is currently open.
func (b *BubbleModel) anyDropdownOpen() bool {
	return b.executorDD.IsOpen() ||
		b.modelDD.IsOpen() ||
		b.needsDD.IsOpen() ||
		b.retryOnDD.IsOpen() ||
		b.onFailureDD.IsOpen() ||
		b.condThenDD.IsOpen() ||
		b.condElseDD.IsOpen()
}

// routeToOpenDropdown routes a KeyMsg to whichever dropdown is open.
func (b *BubbleModel) routeToOpenDropdown(msg tea.KeyMsg) {
	if b.executorDD.IsOpen() {
		confirmed, changed := b.executorDD.Update(msg)
		if confirmed && changed {
			// executor changed: rebuild model dropdown, check if builtin
			exec := b.executorDD.Value()
			if isBuiltin(exec) {
				b.modelEnabled = false
				b.modelDD = NewDropdown(nil, nil)
				// pre-populate args rows
				if argKeys, ok := builtinArgKeys[exec]; ok && len(b.argsRows) == 0 {
					b.argsRows = nil
					for _, k := range argKeys {
						b.argsRows = append(b.argsRows, argsRow{key: k})
					}
				}
			} else {
				b.modelEnabled = true
				provIdx := -1
				for i, p := range b.providers {
					if p.ID == exec {
						provIdx = i
						break
					}
				}
				if provIdx >= 0 {
					b.modelDD = b.buildModelDD(provIdx)
				} else {
					b.modelDD = NewDropdown(nil, nil)
				}
				b.argsRows = nil
			}
			b.applyToStep()
		}
		return
	}
	if b.modelDD.IsOpen() {
		confirmed, changed := b.modelDD.Update(msg)
		if confirmed && changed {
			b.applyToStep()
		}
		return
	}
	if b.needsDD.IsOpen() {
		confirmed := b.needsDD.Update(msg)
		if confirmed {
			b.applyToStep()
		}
		return
	}
	if b.retryOnDD.IsOpen() {
		confirmed, changed := b.retryOnDD.Update(msg)
		if confirmed && changed {
			b.applyToStep()
		}
		return
	}
	if b.onFailureDD.IsOpen() {
		confirmed, changed := b.onFailureDD.Update(msg)
		if confirmed && changed {
			b.applyToStep()
		}
		return
	}
	if b.condThenDD.IsOpen() {
		confirmed, changed := b.condThenDD.Update(msg)
		if confirmed && changed {
			b.applyToStep()
		}
		return
	}
	if b.condElseDD.IsOpen() {
		confirmed, changed := b.condElseDD.Update(msg)
		if confirmed && changed {
			b.applyToStep()
		}
		return
	}
}

// blurAllInputs blurs every text input.
func (b *BubbleModel) blurAllInputs() {
	b.promptInput.Blur()
	b.retryMaxInput.Blur()
	b.retryIntervalInput.Blur()
	b.forEachInput.Blur()
	b.condIfInput.Blur()
	b.publishToInput.Blur()
}

// enterArgsEditing starts inline editing of the args row at idx.
func (b *BubbleModel) enterArgsEditing(idx int) {
	if idx < 0 || idx >= len(b.argsRows) {
		return
	}
	b.argsKeyInput.SetValue(b.argsRows[idx].key)
	b.argsValueInput.SetValue(b.argsRows[idx].value)
	b.argsEditingKey = true
	b.argsKeyInput.Focus()
	b.argsValueInput.Blur()
	b.argsEditing = true
}

// focusCurrentInput focuses the textinput for the current field, if any.
func (b *BubbleModel) focusCurrentInput() {
	b.blurAllInputs()
	if inp := b.focusedInput(); inp != nil {
		inp.Focus()
	}
}

// nextField moves to the next field within the active group (if expanded),
// or advances to the next group header when at the end or group is collapsed.
// This ensures Tab always cycles groups so collapsed groups are reachable.
func (b *BubbleModel) nextField() {
	b.blurAllInputs()
	if b.groupExpanded[b.activeGroup] {
		count := b.groupFieldCount(b.activeGroup)
		nextF := b.activeField + 1
		if nextF < count {
			b.activeField = nextF
			b.focusCurrentInput()
			b.autoScrollRightToField()
			return
		}
	}
	// Advance to next group (cycling), even if collapsed.
	b.activeGroup = (b.activeGroup + 1) % 3
	b.activeField = 0
	if b.groupExpanded[b.activeGroup] {
		b.focusCurrentInput()
	}
	b.autoScrollRightToField()
}

// prevField moves to the previous field within the active group (if expanded),
// or retreats to the previous group header when at the start or group is collapsed.
func (b *BubbleModel) prevField() {
	b.blurAllInputs()
	if b.groupExpanded[b.activeGroup] && b.activeField > 0 {
		b.activeField--
		b.focusCurrentInput()
		b.autoScrollRightToField()
		return
	}
	// Retreat to previous group (cycling), even if collapsed.
	b.activeGroup = (b.activeGroup - 1 + 3) % 3
	if b.groupExpanded[b.activeGroup] {
		b.activeField = b.groupFieldCount(b.activeGroup) - 1
		b.focusCurrentInput()
	} else {
		b.activeField = 0
	}
	b.autoScrollRightToField()
}

// autoScrollRightToField adjusts rightScroll so the active field is visible.
func (b *BubbleModel) autoScrollRightToField() {
	paneH := b.paneHeight()
	if paneH <= 0 {
		return
	}
	line := b.activeFieldLine()
	if line < b.rightScroll {
		b.rightScroll = line
	}
	if line >= b.rightScroll+paneH {
		b.rightScroll = line - paneH + 2
	}
}

// paneHeight returns the usable height of each pane (rows of content).
func (b *BubbleModel) paneHeight() int {
	h := b.height - 5
	if h < 1 {
		return 1
	}
	return h
}

// activeFieldLine returns the approximate line number of the active field in the
// rendered right pane content (0-indexed).
func (b *BubbleModel) activeFieldLine() int {
	line := 0
	for g := 0; g < 3; g++ {
		line++ // group header line
		if !b.groupExpanded[g] {
			continue
		}
		if g < b.activeGroup {
			line += b.groupFieldCount(g)
			// extra lines for dropdown overlays are not tracked here
		} else if g == b.activeGroup {
			line += b.activeField
			return line
		}
	}
	return line
}

// autoScrollLeft adjusts leftScroll so the selected step is visible.
func (b *BubbleModel) autoScrollLeft() {
	paneH := b.paneHeight()
	idx := b.inner.SelectedIndex()
	// +2 for STEPS header + separator
	line := idx + 2
	if line < b.leftScroll {
		b.leftScroll = line
	}
	if line >= b.leftScroll+paneH {
		b.leftScroll = line - paneH + 2
	}
}

// clearStatusMsg is sent after a brief delay to clear the status line.
type clearStatusMsg struct{}

func (b *BubbleModel) Init() tea.Cmd { return nil }

func (b *BubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clearStatusMsg:
		b.statusMsg = ""

	case tea.WindowSizeMsg:
		b.width = msg.Width
		b.height = msg.Height

	case tea.KeyMsg:
		// Dismiss help overlay on any key
		if b.showHelp {
			b.showHelp = false
			return b, nil
		}

		// Pipeline name editing mode
		if b.editingName {
			switch msg.Type {
			case tea.KeyEnter, tea.KeyEsc:
				b.inner.SetName(b.nameInput.Value())
				b.editingName = false
			default:
				var cmd tea.Cmd
				b.nameInput, cmd = b.nameInput.Update(msg)
				return b, cmd
			}
			return b, nil
		}

		// Step ID editing mode
		if b.editingStepID {
			switch msg.Type {
			case tea.KeyEnter, tea.KeyEsc:
				b.applyToStep()
				b.editingStepID = false
				b.stepIDInput.Blur()
			default:
				var cmd tea.Cmd
				b.stepIDInput, cmd = b.stepIDInput.Update(msg)
				return b, cmd
			}
			return b, nil
		}

		// Args row editing mode
		if b.argsEditing {
			switch msg.Type {
			case tea.KeyEnter:
				// commit and exit editing
				if b.argsSelected < len(b.argsRows) {
					b.argsRows[b.argsSelected].key = b.argsKeyInput.Value()
					b.argsRows[b.argsSelected].value = b.argsValueInput.Value()
				}
				b.argsEditing = false
				b.argsKeyInput.Blur()
				b.argsValueInput.Blur()
			case tea.KeyEsc:
				// cancel — discard edits
				b.argsEditing = false
				b.argsKeyInput.Blur()
				b.argsValueInput.Blur()
			case tea.KeyTab, tea.KeyShiftTab:
				// toggle between key and value halves
				if b.argsEditingKey {
					b.argsKeyInput.Blur()
					b.argsValueInput.Focus()
					b.argsEditingKey = false
				} else {
					b.argsValueInput.Blur()
					b.argsKeyInput.Focus()
					b.argsEditingKey = true
				}
			default:
				var cmd tea.Cmd
				if b.argsEditingKey {
					b.argsKeyInput, cmd = b.argsKeyInput.Update(msg)
				} else {
					b.argsValueInput, cmd = b.argsValueInput.Update(msg)
				}
				return b, cmd
			}
			return b, nil
		}

		// If any dropdown is open, route to it first
		if b.anyDropdownOpen() {
			b.routeToOpenDropdown(msg)
			return b, nil
		}

		// If a text input is actually focused, handle structural keys only
		if inp := b.activeFocusedInput(); inp != nil {
			switch msg.Type {
			case tea.KeyEsc:
				return b, tea.Quit
			case tea.KeyTab:
				b.applyToStep()
				b.nextField()
			case tea.KeyShiftTab:
				b.applyToStep()
				b.prevField()
			case tea.KeyUp:
				if len(b.inner.Steps()) > 0 {
					b.applyToStep()
					b.inner.SelectStep(b.inner.SelectedIndex() - 1)
					b.activeGroup = 0
					b.activeField = 0
					b.blurAllInputs()
					b.syncFromStep(b.inner.SelectedIndex())
					b.autoScrollLeft()
				}
			case tea.KeyDown:
				if len(b.inner.Steps()) > 0 {
					b.applyToStep()
					b.inner.SelectStep(b.inner.SelectedIndex() + 1)
					b.activeGroup = 0
					b.activeField = 0
					b.blurAllInputs()
					b.syncFromStep(b.inner.SelectedIndex())
					b.autoScrollLeft()
				}
			default:
				var cmd tea.Cmd
				*inp, cmd = inp.Update(msg)
				// eagerly write prompt to step
				if inp == &b.promptInput {
					idx := b.inner.SelectedIndex()
					steps := b.inner.Steps()
					if idx >= 0 && idx < len(steps) {
						s := steps[idx]
						s.Prompt = b.promptInput.Value()
						b.inner.UpdateStep(idx, s)
					}
				}
				return b, cmd
			}
			return b, nil
		}

		// Navigation and action keys
		switch {
		case key.Matches(msg, keys.Quit):
			return b, tea.Quit

		case key.Matches(msg, keys.Tab):
			if len(b.inner.Steps()) > 0 {
				b.nextField()
			}

		case key.Matches(msg, keys.ShiftTab):
			if len(b.inner.Steps()) > 0 {
				b.prevField()
			}

		case key.Matches(msg, keys.Up):
			if len(b.inner.Steps()) > 0 {
				b.applyToStep()
				b.inner.SelectStep(b.inner.SelectedIndex() - 1)
				b.activeGroup = 0
				b.activeField = 0
				b.blurAllInputs()
				b.syncFromStep(b.inner.SelectedIndex())
				b.autoScrollLeft()
			}

		case key.Matches(msg, keys.Down):
			if len(b.inner.Steps()) > 0 {
				b.applyToStep()
				b.inner.SelectStep(b.inner.SelectedIndex() + 1)
				b.activeGroup = 0
				b.activeField = 0
				b.blurAllInputs()
				b.syncFromStep(b.inner.SelectedIndex())
				b.autoScrollLeft()
			}

		case msg.Type == tea.KeyLeft:
			// collapse active group accordion
			if len(b.inner.Steps()) > 0 {
				b.groupExpanded[b.activeGroup] = false
			}

		case msg.Type == tea.KeyRight:
			// expand active group accordion
			if len(b.inner.Steps()) > 0 {
				b.groupExpanded[b.activeGroup] = true
			}

		case key.Matches(msg, keys.Open):
			if b.activeGroup == 2 && b.activeField == 4 && len(b.argsRows) > 0 {
				// Enter on an args row starts editing it
				b.enterArgsEditing(b.argsSelected)
			} else if len(b.inner.Steps()) > 0 && b.isDropdownField() {
				b.openDropdown()
			}

		case msg.Type == tea.KeyUp && b.activeGroup == 2 && b.activeField == 4:
			if b.argsSelected > 0 {
				b.argsSelected--
			}

		case msg.Type == tea.KeyDown && b.activeGroup == 2 && b.activeField == 4:
			if b.argsSelected < len(b.argsRows)-1 {
				b.argsSelected++
			}

		case msg.String() == "+":
			// args add (when in advanced group, args field) or global add step
			if b.activeGroup == 2 && b.activeField == 4 {
				b.argsRows = append(b.argsRows, argsRow{key: "", value: ""})
				b.argsSelected = len(b.argsRows) - 1
				b.enterArgsEditing(b.argsSelected)
			} else {
				id := fmt.Sprintf("step%d", len(b.inner.Steps())+1)
				exec := ""
				if len(b.providers) > 0 {
					exec = b.providers[0].ID
				}
				b.inner.AddStep(pipeline.Step{ID: id, Executor: exec})
				b.inner.SelectStep(len(b.inner.Steps()) - 1)
				b.activeGroup = 0
				b.activeField = 0
				b.blurAllInputs()
				b.syncFromStep(b.inner.SelectedIndex())
				b.autoScrollLeft()
			}

		case msg.String() == "d":
			// args delete (when in advanced group, args field)
			if b.activeGroup == 2 && b.activeField == 4 && len(b.argsRows) > 0 {
				b.argsRows = append(b.argsRows[:b.argsSelected], b.argsRows[b.argsSelected+1:]...)
				if b.argsSelected >= len(b.argsRows) && b.argsSelected > 0 {
					b.argsSelected--
				}
			}

		case key.Matches(msg, keys.Save):
			home, err := os.UserHomeDir()
			if err == nil {
				dir := filepath.Join(home, ".config", "glitch", "pipelines")
				os.MkdirAll(dir, 0o755) //nolint:errcheck
				path := filepath.Join(dir, b.inner.Name()+".pipeline.yaml")
				b.applyToStep()
				if saveErr := Save(b.inner, path); saveErr != nil {
					b.statusMsg = "  ✗ save failed: " + saveErr.Error()
				} else {
					b.statusMsg = "  ✓ saved → " + path
				}
				return b, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
					return clearStatusMsg{}
				})
			}

		case key.Matches(msg, keys.Help):
			b.showHelp = true

		case msg.String() == "n":
			b.nameInput.SetValue(b.inner.Name())
			b.nameInput.Focus()
			b.editingName = true

		case msg.String() == "i":
			if len(b.inner.Steps()) > 0 {
				idx := b.inner.SelectedIndex()
				b.stepIDInput.SetValue(b.inner.Steps()[idx].ID)
				b.stepIDInput.Focus()
				b.editingStepID = true
			}
		}
	}
	return b, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (b *BubbleModel) View() string {
	if b.width == 0 {
		return "Loading..."
	}
	if b.showHelp {
		return b.renderHelp()
	}

	p := b.palette
	leftW := 28
	// 3 = left border(1) + separator(1) + right border(1)
	rightW := b.width - leftW - 3
	if rightW < 10 {
		rightW = 10
	}
	paneH := b.paneHeight()

	// ── build left pane lines ──
	leftLines := b.buildLeftLines(p)
	leftVisible, leftAbove, leftBelow := applyScroll(leftLines, b.leftScroll, paneH)
	leftBlock := padLines(leftVisible, leftW, paneH, p, leftAbove, leftBelow)

	// ── build right pane lines ──
	rightLines := b.buildRightLines(p, rightW)
	rightVisible, rightAbove, rightBelow := applyScroll(rightLines, b.rightScroll, paneH)
	rightBlock := padLines(rightVisible, rightW, paneH, p, rightAbove, rightBelow)

	// ── header ──
	// ╔══ PIPELINE BUILDER ── NAME: x ══════════ [?] help ══╗
	var headerNamePart string
	if b.editingName {
		headerNamePart = b.nameInput.View()
	} else {
		pipelineName := b.inner.Name()
		if pipelineName == "" {
			pipelineName = "(unnamed)"
		}
		headerNamePart = p.pink + pipelineName + p.reset
	}
	headerPrefix := p.bold + " PIPELINE BUILDER ── NAME: " + p.reset
	helpHint := " [?] help "
	totalInner := b.width - 2 // inside ╔...╗
	// fill: totalInner - len(prefix) - len(namePart) - len(helpHint) dashes
	fillLen := totalInner - visLen(headerPrefix) - visLen(headerNamePart) - visLen(helpHint) - 1
	if fillLen < 0 {
		fillLen = 0
	}
	header := p.purple + "╔" + p.reset +
		headerPrefix + headerNamePart + " " +
		p.dim + strings.Repeat("─", fillLen) + p.reset +
		p.dim + helpHint + p.reset +
		p.purple + "╗" + p.reset

	// ── separator row: ╠══════╦══════════════════════════════╣ ──
	// leftW chars for left section, then separator, then rightW chars
	sepLeft := strings.Repeat("═", leftW)
	sepRight := strings.Repeat("═", rightW)
	separator := p.purple + "╠" + sepLeft + "╦" + sepRight + "╣" + p.reset

	// ── bottom separator ──
	// ╠══════════════════════════════════════════════════════╣
	bottomSep := p.purple + "╠" + strings.Repeat("═", b.width-2) + "╣" + p.reset

	// ── footer ──
	footInner := b.width - 2
	var footerContent string
	if b.statusMsg != "" {
		statusCol := p.pink // success
		if strings.Contains(b.statusMsg, "✗") {
			statusCol = "\x1b[38;5;196m" // red for error
		}
		footFill := footInner - visLen(b.statusMsg)
		if footFill < 0 {
			footFill = 0
		}
		footerContent = statusCol + b.statusMsg + strings.Repeat(" ", footFill) + p.reset
	} else {
		footerText := " [↑↓] steps  [tab] field  [→] expand group  [enter] open  [n] name  [i] step id  [s] save  [?] help "
		footFill := footInner - visLen(footerText)
		if footFill < 0 {
			footFill = 0
		}
		footerContent = p.dim + footerText + strings.Repeat(" ", footFill) + p.reset
	}
	footer := p.purple + "╚" + p.reset + footerContent + p.purple + "╝" + p.reset

	// ── assemble pane rows ──
	var sb strings.Builder
	sb.WriteString(header + "\n")
	sb.WriteString(separator + "\n")

	for i := 0; i < paneH; i++ {
		leftLine := ""
		if i < len(leftBlock) {
			leftLine = leftBlock[i]
		}
		rightLine := ""
		if i < len(rightBlock) {
			rightLine = rightBlock[i]
		}
		sb.WriteString(p.purple + "║" + p.reset + leftLine + p.purple + "│" + p.reset + rightLine + p.purple + "║" + p.reset + "\n")
	}

	sb.WriteString(bottomSep + "\n")
	sb.WriteString(footer)

	return sb.String()
}

// renderHelp renders a full-screen help overlay listing all keybindings.
func (b *BubbleModel) renderHelp() string {
	p := b.palette
	w := b.width
	if w < 40 {
		w = 40
	}
	h := b.height
	if h < 10 {
		h = 10
	}

	// Box inner width (leave 4 cols margin each side, min 40)
	boxW := w - 8
	if boxW < 40 {
		boxW = 40
	}
	inner := boxW - 2

	type entry struct{ key, desc string }
	sections := []struct {
		title   string
		entries []entry
	}{
		{"Navigation", []entry{
			{"↑ / ↓", "select step (left pane)"},
			{"tab / shift+tab", "next / prev field in expanded groups"},
			{"→  /  ←", "expand / collapse group"},
			{"space / enter", "open dropdown"},
			{"esc", "close dropdown / cancel name edit"},
		}},
		{"Pipeline", []entry{
			{"n", "edit pipeline name"},
			{"i", "rename selected step"},
			{"s", "save pipeline to disk"},
			{"+", "add step"},
		}},
		{"View", []entry{
			{"?", "toggle this help overlay"},
			{"ctrl+c  /  q", "quit without saving"},
		}},
	}

	pad := func(s string, width int) string {
		vl := visLen(s)
		if vl >= width {
			return s
		}
		return s + strings.Repeat(" ", width-vl)
	}
	hline := func() string {
		return p.purple + "├" + strings.Repeat("─", inner) + "┤" + p.reset
	}

	var lines []string
	lines = append(lines, p.purple+"┌"+strings.Repeat("─", inner)+"┐"+p.reset)
	title := " PIPELINE BUILDER — KEYBINDINGS "
	titleFill := inner - visLen(title)
	if titleFill < 0 {
		titleFill = 0
	}
	lines = append(lines, p.purple+"│"+p.reset+p.bold+title+p.reset+p.dim+strings.Repeat(" ", titleFill)+p.reset+p.purple+"│"+p.reset)

	for _, sec := range sections {
		lines = append(lines, hline())
		secHeader := "  " + sec.title
		lines = append(lines, p.purple+"│"+p.reset+p.blue+pad(secHeader, inner)+p.reset+p.purple+"│"+p.reset)
		for _, e := range sec.entries {
			keyPart := p.pink + "  " + e.key
			descPart := p.dim + "  " + e.desc + p.reset
			row := keyPart + descPart
			lines = append(lines, p.purple+"│"+p.reset+pad(row, inner)+p.purple+"│"+p.reset)
		}
	}

	lines = append(lines, hline())
	dismiss := "  press any key to dismiss"
	lines = append(lines, p.purple+"│"+p.reset+p.dim+pad(dismiss, inner)+p.reset+p.purple+"│"+p.reset)
	lines = append(lines, p.purple+"└"+strings.Repeat("─", inner)+"┘"+p.reset)

	// center vertically
	topPad := (h - len(lines)) / 2
	if topPad < 0 {
		topPad = 0
	}
	leftPad := strings.Repeat(" ", (w-boxW)/2)

	var sb strings.Builder
	for i := 0; i < topPad; i++ {
		sb.WriteByte('\n')
	}
	for _, l := range lines {
		sb.WriteString(leftPad + l + "\n")
	}
	return sb.String()
}

// buildLeftLines builds the lines for the left pane (step list).
func (b *BubbleModel) buildLeftLines(p palette) []string {
	var lines []string

	// Header
	lines = append(lines, p.bold+"STEPS"+p.reset)
	lines = append(lines, p.dim+strings.Repeat("─", 26)+p.reset)

	for i, s := range b.inner.Steps() {
		exec := s.Executor
		icon := "◆"
		if isBuiltin(exec) {
			icon = "⚙"
		}
		label := fmt.Sprintf("%s [%d] %s", icon, i+1, s.ID)
		if i == b.inner.SelectedIndex() {
			lines = append(lines, p.pink+"→ "+label+p.reset)
		} else {
			lines = append(lines, p.dim+"  "+label+p.reset)
		}
	}

	lines = append(lines, "")
	lines = append(lines, p.dim+"[+] add step"+p.reset)

	return lines
}

// buildRightLines builds the lines for the right pane (fields).
func (b *BubbleModel) buildRightLines(p palette, width int) []string {
	steps := b.inner.Steps()
	if len(steps) == 0 {
		return []string{
			"",
			p.dim + "  No steps yet." + p.reset,
			"",
			p.dim + "  Press [+] to add your first step." + p.reset,
			p.dim + "  Each step can use a provider or builtin executor." + p.reset,
			"",
			p.dim + "  Once a step is selected:" + p.reset,
			p.dim + "    [tab]    move between fields" + p.reset,
			p.dim + "    [enter]  open dropdown" + p.reset,
		}
	}

	idx := b.inner.SelectedIndex()
	sel := steps[idx]
	var lines []string

	// Step title — show editable ID inline when editing
	var stepIDPart string
	if b.editingStepID {
		stepIDPart = b.stepIDInput.View()
	} else {
		stepIDPart = p.pink + sel.ID + p.reset
	}
	stepTitle := p.bold + fmt.Sprintf("STEP %d — ", idx+1) + p.reset + stepIDPart
	if !b.editingStepID {
		stepTitle += p.dim + "  [i] rename" + p.reset
	}
	lines = append(lines, stepTitle)
	lines = append(lines, p.dim+strings.Repeat("─", width-2)+p.reset)

	// Core group
	lines = append(lines, b.renderGroupLines(p, width)...)

	return lines
}

// renderGroupLines renders all three groups' lines.
func (b *BubbleModel) renderGroupLines(p palette, width int) []string {
	var lines []string
	lines = append(lines, b.renderCore(p, width)...)
	lines = append(lines, b.renderExecution(p, width)...)
	lines = append(lines, b.renderAdvanced(p, width)...)
	return lines
}

// renderCore renders the Core group fields as a []string of lines.
func (b *BubbleModel) renderCore(p palette, width int) []string {
	var lines []string
	expanded := b.groupExpanded[0]
	active := b.activeGroup == 0

	// Group header
	lines = append(lines, b.groupHeader("CORE", 0, p, expanded, b.groupFieldCount(0), width))

	if !expanded {
		return lines
	}

	// field 0: executorDD
	ddView := b.executorDD.View("Executor", width, p)
	ddLines := strings.Split(ddView, "\n")
	for li, dl := range ddLines {
		if li == 0 {
			if active && b.activeField == 0 {
				lines = append(lines, p.pink+"▶ "+p.reset+dl)
			} else {
				lines = append(lines, "  "+dl)
			}
		} else {
			lines = append(lines, dl)
		}
	}

	// field 1: modelDD (if enabled)
	if b.modelEnabled {
		modelView := b.modelDD.View("Model", width, p)
		modelLines := strings.Split(modelView, "\n")
		for li, ml := range modelLines {
			if li == 0 {
				if active && b.activeField == 1 {
					lines = append(lines, p.pink+"▶ "+p.reset+ml)
				} else {
					lines = append(lines, "  "+ml)
				}
			} else {
				lines = append(lines, ml)
			}
		}
	} else {
		lines = append(lines, p.dim+"  Model: (n/a for builtin)"+p.reset)
	}

	// field 2 (or 1 if model disabled): promptInput
	promptFieldIdx := 2
	if !b.modelEnabled {
		promptFieldIdx = 1
	}
	if active && b.activeField == promptFieldIdx {
		lines = append(lines, p.pink+"▶ "+p.reset+p.blue+"Prompt: "+p.reset+b.promptInput.View())
	} else {
		pval := b.promptInput.Value()
		if pval == "" {
			pval = "(none)"
		}
		lines = append(lines, "  "+p.blue+"Prompt: "+p.reset+pval)
	}

	return lines
}

// renderExecution renders the Execution group fields as a []string of lines.
func (b *BubbleModel) renderExecution(p palette, width int) []string {
	var lines []string
	expanded := b.groupExpanded[1]
	active := b.activeGroup == 1
	fieldCount := b.groupFieldCount(1)

	lines = append(lines, b.groupHeader("EXECUTION", 1, p, expanded, fieldCount, width))

	if !expanded {
		return lines
	}

	fields := []struct {
		idx  int
		view string
	}{
		{0, b.needsDD.View("Needs", width, p)},
		{1, p.blue + "  RetryMax: " + p.reset + b.retryMaxInput.View()},
		{2, p.blue + "  RetryInterval: " + p.reset + b.retryIntervalInput.View()},
		{3, b.retryOnDD.View("RetryOn", width, p)},
		{4, p.blue + "  ForEach: " + p.reset + b.forEachInput.View()},
		{5, b.onFailureDD.View("OnFailure", width, p)},
	}

	for _, f := range fields {
		fieldLines := strings.Split(f.view, "\n")
		for li, fl := range fieldLines {
			if li == 0 {
				if active && b.activeField == f.idx {
					lines = append(lines, p.pink+"▶ "+p.reset+fl)
				} else {
					lines = append(lines, "  "+fl)
				}
			} else {
				lines = append(lines, fl)
			}
		}
	}

	return lines
}

// renderAdvanced renders the Advanced group fields as a []string of lines.
func (b *BubbleModel) renderAdvanced(p palette, width int) []string {
	var lines []string
	expanded := b.groupExpanded[2]
	active := b.activeGroup == 2
	fieldCount := b.groupFieldCount(2)

	lines = append(lines, b.groupHeader("ADVANCED", 2, p, expanded, fieldCount, width))

	if !expanded {
		return lines
	}

	fields := []struct {
		idx  int
		view string
	}{
		{0, p.blue + "  Cond.If: " + p.reset + b.condIfInput.View()},
		{1, b.condThenDD.View("Cond.Then", width, p)},
		{2, b.condElseDD.View("Cond.Else", width, p)},
		{3, p.blue + "  PublishTo: " + p.reset + b.publishToInput.View()},
	}

	for _, f := range fields {
		fieldLines := strings.Split(f.view, "\n")
		for li, fl := range fieldLines {
			if li == 0 {
				if active && b.activeField == f.idx {
					lines = append(lines, p.pink+"▶ "+p.reset+fl)
				} else {
					lines = append(lines, "  "+fl)
				}
			} else {
				lines = append(lines, fl)
			}
		}
	}

	// Args rows (field 4)
	argsHeader := p.blue + "  Args:" + p.reset
	if active && b.activeField == 4 {
		lines = append(lines, p.pink+"▶ "+p.reset+argsHeader)
	} else {
		lines = append(lines, "  "+argsHeader)
	}
	for i, row := range b.argsRows {
		isSelected := active && b.activeField == 4 && i == b.argsSelected
		if isSelected && b.argsEditing {
			// show live text inputs for key and value
			keyView := b.argsKeyInput.View()
			valView := b.argsValueInput.View()
			lines = append(lines, p.pink+"    ▶ "+p.reset+keyView+p.dim+" = "+p.reset+valView)
			lines = append(lines, p.dim+"      tab: key↔value  enter: confirm  esc: cancel"+p.reset)
		} else if isSelected {
			lines = append(lines, p.pink+"    ▶ "+row.key+p.dim+" = "+p.reset+p.pink+row.value+p.reset)
		} else {
			lines = append(lines, p.dim+"      "+row.key+" = "+row.value+p.reset)
		}
	}
	if active && b.activeField == 4 {
		hint := "  [+] add"
		if len(b.argsRows) > 0 {
			hint += "  [enter] edit  [d] delete  [↑↓] navigate"
		}
		lines = append(lines, p.dim+hint+p.reset)
	}

	return lines
}

// groupHeader renders a single group header line with accordion indicator.
func (b *BubbleModel) groupHeader(name string, groupIdx int, p palette, expanded bool, fieldCount int, width int) string {
	indicator := "▶"
	if expanded {
		indicator = "▼"
	}

	active := b.activeGroup == groupIdx
	col := p.dim
	if active {
		col = p.pink
	}

	var text string
	if expanded {
		// ▼ NAME ─────────────────────────────
		title := indicator + " " + name + " "
		fillLen := width - visLen(title) - 2
		if fillLen < 0 {
			fillLen = 0
		}
		text = col + title + strings.Repeat("─", fillLen) + p.reset
	} else {
		// ▶ NAME ──── N fields ────────────────
		countStr := fmt.Sprintf(" %d fields ", fieldCount)
		prefix := indicator + " " + name
		// total: prefix + " ─── " + countStr + " ───..."
		dashLeft := 3
		dashRight := width - visLen(prefix) - visLen(countStr) - dashLeft - 2
		if dashRight < 0 {
			dashRight = 0
		}
		text = col + prefix + " " + strings.Repeat("─", dashLeft) + countStr + strings.Repeat("─", dashRight) + p.reset
	}

	return text
}

// applyScroll returns the visible slice [scroll : scroll+height] plus above/below counts.
func applyScroll(lines []string, scroll, height int) (visible []string, above, below int) {
	total := len(lines)
	if scroll < 0 {
		scroll = 0
	}
	if scroll > total {
		scroll = total
	}
	end := scroll + height
	if end > total {
		end = total
	}
	above = scroll
	below = total - end
	visible = lines[scroll:end]
	return
}

// padLines pads/truncates each line to exactly `width` visible chars and pads to `height` lines.
// Prepends scroll indicators if above/below > 0.
func padLines(lines []string, width, height int, p palette, above, below int) []string {
	result := make([]string, 0, height)

	for i, line := range lines {
		isFirst := i == 0
		isLast := i == len(lines)-1

		// prepend scroll-up indicator on first line
		if isFirst && above > 0 {
			indLine := p.dim + fmt.Sprintf("  ▲ %d more", above) + p.reset
			result = append(result, padToWidth(indLine, width))
			continue
		}
		// prepend scroll-down indicator on last line
		if isLast && below > 0 {
			indLine := p.dim + fmt.Sprintf("  ▼ %d more", below) + p.reset
			result = append(result, padToWidth(indLine, width))
			continue
		}

		result = append(result, padToWidth(line, width))
	}

	// fill remaining lines with blanks
	for len(result) < height {
		result = append(result, strings.Repeat(" ", width))
	}
	return result
}

// padToWidth pads a line (which may contain ANSI escapes) to exactly `width` visible chars.
func padToWidth(line string, width int) string {
	vl := visLen(line)
	if vl < width {
		return line + strings.Repeat(" ", width-vl)
	}
	// truncate to width visible chars (preserve ANSI)
	return truncateANSI(line, width)
}

// visLen returns the number of visible (non-ANSI) characters in a string.
func visLen(s string) int {
	n := 0
	inEsc := false
	for i := 0; i < len(s); {
		if inEsc {
			if s[i] == 'm' {
				inEsc = false
			}
			i++
			continue
		}
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			inEsc = true
			i += 2
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		_ = r
		n++
		i += size
	}
	return n
}

// truncateANSI truncates string to at most `maxVis` visible chars while preserving ANSI escapes.
func truncateANSI(s string, maxVis int) string {
	var sb strings.Builder
	n := 0
	inEsc := false
	for i := 0; i < len(s); {
		if inEsc {
			sb.WriteByte(s[i])
			if s[i] == 'm' {
				inEsc = false
			}
			i++
			continue
		}
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			sb.WriteByte(s[i])
			inEsc = true
			i++
			continue
		}
		if n >= maxVis {
			break
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		sb.WriteRune(r)
		n++
		i += size
	}
	return sb.String()
}

// stepLabel returns a display label for a step.
func stepLabel(s pipeline.Step) string {
	if s.Executor != "" {
		return s.Executor
	}
	if s.Type != "" {
		return s.Type
	}
	return s.ID
}


// keep dimIf for any leftover references (it's a no-op)
func dimIf(_ bool, s string, _ palette) string {
	return s
}

// suppress unused import warnings
var (
	_ = stepLabel
	_ = dimIf
)
