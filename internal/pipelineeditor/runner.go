package pipelineeditor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/powerglove-dev/gl1tch/internal/buildershared"
	"github.com/powerglove-dev/gl1tch/internal/busd/topics"
	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/picker"
	"github.com/powerglove-dev/gl1tch/internal/pipeline"
	"github.com/powerglove-dev/gl1tch/internal/systemprompts"
)


// startRun invokes the selected AI provider to generate pipeline YAML from the
// user's description. Output streams via the shared RunnerPanel.
func (m Model) startRun() (Model, tea.Cmd) {
	if m.runner.IsRunning() {
		return m, nil
	}

	prompt := strings.TrimSpace(m.editor.Content())
	if prompt == "" {
		m.statusMsg = "enter a description in the PROMPT field first"
		m.statusErr = true
		return m, nil
	}

	executorID := m.editor.SelectedProviderID()
	if executorID == "" {
		executorID = "claude"
	}
	modelID := m.editor.SelectedModelID()

	generationPrompt := systemprompts.Load(systemprompts.PipelineGenerator) + prompt
	yamlContent := buildYAML("generate-pipeline", executorID, modelID, generationPrompt)

	ch := make(chan string, 200)
	ctx, cancel := context.WithCancel(context.Background())

	st := m.store
	providers := picker.BuildProviders()

	go func() {
		defer close(ch)

		mgr, err := buildExecutorManager(providers)
		if err != nil {
			ch <- "error building executor manager: " + err.Error()
			return
		}

		p, err := pipeline.Load(strings.NewReader(yamlContent))
		if err != nil {
			ch <- "error: " + err.Error()
			return
		}

		pub := &linePublisher{ch: ch}

		var opts []pipeline.RunOption
		opts = append(opts, pipeline.WithEventPublisher(pub))
		if st != nil {
			opts = append(opts, pipeline.WithRunStore(st))
		}

		_, runErr := pipeline.Run(ctx, p, mgr, "", opts...)
		if runErr != nil {
			if ctx.Err() != nil {
				ch <- "cancelled"
			} else {
				ch <- "error: " + runErr.Error()
			}
		}
	}()

	m.runner, _ = m.runner.StartRun(ch, cancel)
	m.focus = FocusRunner
	return m, buildershared.WaitForLine(ch)
}

// startRunWithPrompt runs with an explicit prompt string (for chat input / ctrl+r).
func (m Model) startRunWithPrompt(prompt string) (Model, tea.Cmd) {
	if m.runner.IsRunning() {
		return m, nil
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return m, nil
	}

	executorID := m.editor.SelectedProviderID()
	if executorID == "" {
		executorID = "claude"
	}
	modelID := m.editor.SelectedModelID()

	generationPrompt := systemprompts.Load(systemprompts.PipelineGenerator) + prompt
	yamlContent := buildYAML("generate-pipeline", executorID, modelID, generationPrompt)

	ch := make(chan string, 200)
	ctx, cancel := context.WithCancel(context.Background())

	st := m.store
	providers := picker.BuildProviders()

	go func() {
		defer close(ch)

		mgr, err := buildExecutorManager(providers)
		if err != nil {
			ch <- "error building executor manager: " + err.Error()
			return
		}

		p, err := pipeline.Load(strings.NewReader(yamlContent))
		if err != nil {
			ch <- "error: " + err.Error()
			return
		}

		pub := &linePublisher{ch: ch}

		var opts []pipeline.RunOption
		opts = append(opts, pipeline.WithEventPublisher(pub))
		if st != nil {
			opts = append(opts, pipeline.WithRunStore(st))
		}

		_, runErr := pipeline.Run(ctx, p, mgr, "", opts...)
		if runErr != nil {
			if ctx.Err() != nil {
				ch <- "cancelled"
			} else {
				ch <- "error: " + runErr.Error()
			}
		}
	}()

	m.runner, _ = m.runner.StartRun(ch, cancel)
	m.focus = FocusRunner
	return m, buildershared.WaitForLine(ch)
}

// buildExecutorManager constructs a executor.Manager from the provider list,
// matching the registration pattern used in cmd/pipeline.go.
func buildExecutorManager(providers []picker.ProviderDef) (*executor.Manager, error) {
	mgr := executor.NewManager()
	for _, prov := range providers {
		// Sidecar-backed providers are registered by LoadWrappersFromDir.
		if prov.SidecarPath != "" {
			continue
		}
		binary := prov.Command
		if binary == "" {
			binary = prov.ID
		}
		if err := mgr.Register(executor.NewCliAdapter(prov.ID, prov.Label+" CLI adapter", binary, prov.PipelineArgs...)); err != nil {
			// Non-fatal: provider just won't be available.
			_ = err
		}
	}

	// Load sidecar plugins from ~/.config/glitch/wrappers/.
	configDir := picker.GlitchConfigDir()
	if configDir != "" {
		wrappersDir := filepath.Join(configDir, "wrappers")
		_ = mgr.LoadWrappersFromDir(wrappersDir) // non-fatal
	}

	return mgr, nil
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
			if topic == topics.StepFailed {
				p.ch <- fmt.Sprintf("[fail] %s", evt.StepID)
			}
			if evt.Output != "" {
				for _, line := range strings.Split(evt.Output, "\n") {
					line = strings.TrimRight(line, "\r")
					if line != "" {
						p.ch <- line
					}
				}
			}
		}
	case topics.StepStarted:
		var evt struct {
			StepID string `json:"step_id"`
		}
		if err := json.Unmarshal(payload, &evt); err == nil && evt.StepID != "" {
			p.ch <- fmt.Sprintf("[generating via %s…]", evt.StepID)
		}
	case topics.RunCompleted:
		p.ch <- "[done]"
	case topics.RunFailed:
		var evt struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(payload, &evt); err == nil && evt.Error != "" {
			p.ch <- "[fail] " + evt.Error
		} else {
			p.ch <- "[fail] generation failed"
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

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
