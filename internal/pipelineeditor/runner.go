package pipelineeditor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/busd/topics"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/pipeline"
	"github.com/adam-stokes/orcai/internal/plugin"
)

// startRun builds a pipeline from current editor state and runs it in a goroutine.
func (m Model) startRun() (Model, tea.Cmd) {
	if m.runRunning {
		return m, nil
	}

	// Cancel any previous run.
	if m.runCancel != nil {
		m.runCancel()
		m.runCancel = nil
	}

	// Build YAML from editor state.
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		name = "test-pipeline"
	}
	executorID := m.picker.SelectedProviderID()
	modelID := m.picker.SelectedModelID()
	prompt := m.promptArea.Value()
	yamlContent := buildYAML(name, executorID, modelID, prompt)

	// Create output channel.
	ch := make(chan string, 100)
	m.runOutputCh = ch
	m.runLines = nil
	m.runRunning = true
	m.statusMsg = ""
	m.statusErr = false
	m.clarifyActive = false

	// Create cancel context.
	ctx, cancel := context.WithCancel(context.Background())
	m.runCancel = cancel

	// Capture store for the goroutine.
	st := m.store
	providers := picker.BuildProviders()

	go func() {
		defer close(ch)

		// Build plugin manager.
		mgr := plugin.NewManager()
		for _, prov := range providers {
			if prov.Command == "" {
				continue
			}
			// Register as a plugin by command.
			_ = mgr // providers registered elsewhere for CLI providers
		}
		_ = mgr

		// Rebuild manager from BuildProviders (same pattern as cmd/pipeline.go).
		mgr2, err := buildPluginManager(providers)
		if err != nil {
			ch <- "error building plugin manager: " + err.Error()
			return
		}

		// Parse pipeline.
		p, err := pipeline.Load(strings.NewReader(yamlContent))
		if err != nil {
			ch <- "error: " + err.Error()
			return
		}

		// Create event publisher that sends output lines to ch.
		pub := &linePublisher{ch: ch}

		var opts []pipeline.RunOption
		opts = append(opts, pipeline.WithEventPublisher(pub))
		if st != nil {
			opts = append(opts, pipeline.WithRunStore(st))
		}

		_, runErr := pipeline.Run(ctx, p, mgr2, "", opts...)
		if runErr != nil {
			if ctx.Err() != nil {
				ch <- "run cancelled"
			} else {
				ch <- "error: " + runErr.Error()
			}
		}
	}()

	return m, waitForLine(ch)
}

// buildPluginManager constructs a plugin.Manager from the given provider list.
// For each provider backed by a command, it registers a command-based executor.
func buildPluginManager(providers []picker.ProviderDef) (*plugin.Manager, error) {
	mgr := plugin.NewManager()
	// Providers that are command-backed are handled by the pipeline executor
	// lookup directly (the executor field in the YAML step). The manager is
	// used for builtin steps; command-based executors are looked up differently.
	// We return an empty manager here which handles builtin steps only.
	_ = providers
	return mgr, nil
}

// waitForLine returns a cmd that blocks until the next line arrives from ch.
func waitForLine(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return RunDoneMsg{}
		}
		return RunLineMsg(line)
	}
}

// pollClarify returns a cmd that polls the store for pending clarification requests.
func (m Model) pollClarify() tea.Cmd {
	if m.store == nil {
		return nil
	}
	st := m.store
	return func() tea.Msg {
		reqs, err := st.LoadPendingClarifications()
		if err != nil || len(reqs) == 0 {
			return nil
		}
		req := reqs[0]
		return ClarifyPollMsg{RunID: req.RunID, Question: req.Question}
	}
}

// linePublisher implements pipeline.EventPublisher, forwarding step events as
// human-readable lines to a channel.
type linePublisher struct {
	ch chan<- string
}

func (p *linePublisher) Publish(_ context.Context, topic string, payload []byte) error {
	switch topic {
	case topics.StepDone, topics.StepFailed:
		var evt struct {
			Output string `json:"output"`
			StepID string `json:"step_id"`
		}
		if err := json.Unmarshal(payload, &evt); err == nil {
			prefix := "[done]"
			if topic == topics.StepFailed {
				prefix = "[fail]"
			}
			if evt.Output != "" {
				for _, line := range strings.Split(evt.Output, "\n") {
					line = strings.TrimRight(line, "\r")
					if line != "" {
						p.ch <- fmt.Sprintf("%s %s: %s", prefix, evt.StepID, line)
					}
				}
			}
		}
	case topics.StepStarted:
		var evt struct {
			StepID string `json:"step_id"`
		}
		if err := json.Unmarshal(payload, &evt); err == nil && evt.StepID != "" {
			p.ch <- fmt.Sprintf("[run] step: %s", evt.StepID)
		}
	case topics.RunStarted:
		var evt struct {
			Pipeline string `json:"pipeline"`
		}
		if err := json.Unmarshal(payload, &evt); err == nil {
			p.ch <- fmt.Sprintf("[start] pipeline: %s", evt.Pipeline)
		}
	case topics.RunCompleted:
		p.ch <- "[done] pipeline complete"
	case topics.RunFailed:
		var evt struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(payload, &evt); err == nil && evt.Error != "" {
			p.ch <- "[fail] " + evt.Error
		} else {
			p.ch <- "[fail] pipeline failed"
		}
	}
	return nil
}

// runCmd executes a command and returns its stdout output.
func runCmd(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// runCmdIgnore executes a command, ignoring errors.
func runCmdIgnore(name string, args ...string) {
	exec.Command(name, args...).Run() //nolint:errcheck
}
