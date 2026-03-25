package pipeline

import (
	"bytes"
	"context"
	"fmt"

	"github.com/adam-stokes/orcai/internal/plugin"
)

// Run executes a pipeline against the given plugin manager.
// userInput is the initial value injected for the first input step.
// publisher receives lifecycle events; pass NoopPublisher{} when not needed.
// Returns the final output string.
func Run(ctx context.Context, p *Pipeline, mgr *plugin.Manager, userInput string, publisher EventPublisher) (string, error) {
	if publisher == nil {
		publisher = NoopPublisher{}
	}

	ec := NewExecutionContext()

	// Seed pipeline-level vars into context under "param".
	for k, v := range p.Vars {
		ec.Set("param."+k, v)
	}

	// Index steps by ID for branch lookups.
	byID := make(map[string]*Step, len(p.Steps))
	order := make([]string, 0, len(p.Steps))
	for i := range p.Steps {
		byID[p.Steps[i].ID] = &p.Steps[i]
		order = append(order, p.Steps[i].ID)
	}

	visited := make(map[string]bool)
	queue := append([]string(nil), order...)

	// lastOutput tracks the most recent step output for use as defaultInput
	// in the next plugin step. Seeded with userInput so the first plugin step
	// receives it if it has no explicit prompt/input.
	lastOutput := userInput
	var lastPluginOutput string

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		if visited[id] {
			continue
		}
		visited[id] = true

		step, ok := byID[id]
		if !ok {
			return "", fmt.Errorf("pipeline: unknown step id %q", id)
		}

		switch step.Type {
		case "input":
			ec.Set(step.ID+".out", userInput)
			lastOutput = userInput

		case "output":
			return lastPluginOutput, nil

		default:
			output, err := executeStep(ctx, step, ec, mgr, lastOutput)
			if err != nil {
				return "", fmt.Errorf("pipeline: step %q: %w", step.ID, err)
			}
			ec.Set(step.ID+".out", output)
			lastPluginOutput = output
			lastOutput = output

			// Evaluate branch condition if present.
			if step.Condition.If != "" {
				condVars := ec.Snapshot()
				condVars["_output"] = output
				if EvalCondition(step.Condition.If, condVars) {
					if step.Condition.Then != "" {
						queue = append([]string{step.Condition.Then}, filterOut(queue, step.Condition.Else)...)
					}
				} else {
					if step.Condition.Else != "" {
						queue = append([]string{step.Condition.Else}, filterOut(queue, step.Condition.Then)...)
					}
				}
			}
		}
	}

	return lastPluginOutput, nil
}

// executeStep runs a single plugin step and returns its string output.
// defaultInput is used when the step has no explicit prompt or input.
func executeStep(ctx context.Context, step *Step, ec *ExecutionContext, mgr *plugin.Manager, defaultInput string) (string, error) {
	// Resolve plugin name: prefer Executor, fall back to Plugin.
	pluginName := step.Executor
	if pluginName == "" {
		pluginName = step.Plugin
	}

	pl, ok := mgr.Get(pluginName)
	if !ok {
		return "", fmt.Errorf("plugin %q not found", pluginName)
	}

	snap := ec.Snapshot()

	raw := step.Prompt + step.Input
	if raw == "" {
		raw = defaultInput
	}
	promptOrInput := Interpolate(raw, snap)

	stepVars := ec.FlatStrings()
	stepVars["model"] = step.Model

	var buf bytes.Buffer
	if err := pl.Execute(ctx, promptOrInput, stepVars, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// filterOut removes a single value from a slice.
func filterOut(ss []string, remove string) []string {
	if remove == "" {
		return ss
	}
	out := ss[:0:len(ss)]
	for _, s := range ss {
		if s != remove {
			out = append(out, s)
		}
	}
	return out
}
