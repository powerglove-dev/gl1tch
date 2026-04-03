package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/activity"
	"github.com/8op-org/gl1tch/internal/busd/topics"
	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/supervisor"
)

// AgentLoopConfig holds the parsed configuration from a sidecar YAML that has
// agent_loop: true.
type AgentLoopConfig struct {
	PluginName    string
	ExecutorName  string
	MaxIterations int
	LoopSleep     time.Duration
}

// AgentLoopHandler drives a single plugin in an autonomous loop:
// execute → ask reasoning model → repeat up to MaxIterations.
type AgentLoopHandler struct {
	cfg     AgentLoopConfig
	execMgr *executor.Manager
	pub     EventPublisher
}

// NewAgentLoopHandler creates an AgentLoopHandler for the given plugin config.
func NewAgentLoopHandler(cfg AgentLoopConfig, execMgr *executor.Manager, pub EventPublisher) *AgentLoopHandler {
	return &AgentLoopHandler{cfg: cfg, execMgr: execMgr, pub: pub}
}

// Name implements supervisor.Handler.
func (a *AgentLoopHandler) Name() string { return "agent_loop:" + a.cfg.PluginName }

// Topics implements supervisor.Handler. Agent loop handlers don't react to bus
// events — they are driven by the supervisor on startup. Return an empty list
// so the supervisor doesn't subscribe them to any topics.
func (a *AgentLoopHandler) Topics() []string { return nil }

// Handle is called once at supervisor startup (synthetic event with empty topic).
func (a *AgentLoopHandler) Handle(ctx context.Context, _ supervisor.Event, model supervisor.ResolvedModel) error {
	return a.run(ctx, model)
}

// run executes the autonomous loop for this plugin.
func (a *AgentLoopHandler) run(ctx context.Context, model supervisor.ResolvedModel) error {
	maxIter := a.cfg.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}
	sleep := a.cfg.LoopSleep
	if sleep == 0 {
		sleep = 5 * time.Second
	}

	var summary strings.Builder
	input := "" // initial prompt is empty; the plugin drives itself

loop:
	for i := 0; i < maxIter; i++ {
		select {
		case <-ctx.Done():
			fmt.Fprintf(&summary, "\nLoop cancelled after %d iterations.", i)
			a.publishSummary(ctx, summary.String())
			return nil
		default:
		}

		// Execute the plugin.
		var pluginBuf bytes.Buffer
		if a.execMgr != nil {
			if err := a.execMgr.Execute(ctx, a.cfg.ExecutorName, input, nil, &pluginBuf); err != nil {
				slog.Warn("agent_loop: plugin execution failed",
					"plugin", a.cfg.PluginName,
					"iter", i,
					"err", err)
				break loop
			}
		}
		pluginOutput := pluginBuf.String()
		fmt.Fprintf(&summary, "[iter %d] plugin output: %s\n", i+1, truncateStr(pluginOutput, 200))

		// Ask the reasoning model what to do next.
		if a.execMgr != nil && model.ProviderID != "" && model.ModelID != "" {
			reasoningPrompt := fmt.Sprintf(
				"You are an autonomous agent controller.\n"+
					"Plugin %q just produced this output:\n%s\n\n"+
					"Iteration %d of %d.\n"+
					"Reply with the next input to send to the plugin, or reply exactly \"DONE\" to stop.",
				a.cfg.PluginName, pluginOutput, i+1, maxIter,
			)
			var reasonBuf bytes.Buffer
			vars := map[string]string{"model": model.ModelID}
			if err := a.execMgr.Execute(ctx, model.ProviderID, reasoningPrompt, vars, &reasonBuf); err != nil {
				slog.Warn("agent_loop: reasoning model failed",
					"model", model.ModelID,
					"err", err)
				break loop
			}
			nextInput := strings.TrimSpace(reasonBuf.String())
			if strings.EqualFold(nextInput, "done") {
				fmt.Fprintf(&summary, "[iter %d] reasoning model said DONE.\n", i+1)
				break loop
			}
			input = nextInput
		}

		// Sleep between iterations.
		select {
		case <-ctx.Done():
			break loop
		case <-time.After(sleep):
		}
	}

	a.publishSummary(ctx, summary.String())
	return nil
}

// publishSummary emits the loop completion notification and writes to activity feed.
func (a *AgentLoopHandler) publishSummary(ctx context.Context, summary string) {
	if a.pub != nil {
		note := NotificationPayload{
			Session:  "",
			Title:    "Agent loop completed: " + a.cfg.PluginName,
			Body:     summary,
			Severity: "warning",
		}
		noteBytes, err := json.Marshal(note)
		if err == nil {
			_ = a.pub.Publish(ctx, topics.NotificationAgentLoopComplete, noteBytes)
		}
	}

	_ = activity.AppendEvent(activity.DefaultPath(), activity.ActivityEvent{
		TS:     time.Now().UTC().Format(time.RFC3339),
		Kind:   "agent_loop",
		Agent:  a.cfg.PluginName,
		Label:  truncateStr(summary, 60),
		Status: "done",
	})
}

// truncateStr shortens s to at most n runes.
func truncateStr(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// ScanAgentLoopSidecars scans wrappersDir for sidecar YAML files that declare
// agent_loop: true and returns one AgentLoopConfig per matching file.
func ScanAgentLoopSidecars(wrappersDir string) []AgentLoopConfig {
	entries, err := os.ReadDir(wrappersDir)
	if err != nil {
		return nil
	}
	var out []AgentLoopConfig
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		adapter, err := executor.NewCliAdapterFromSidecar(filepath.Join(wrappersDir, e.Name()))
		if err != nil {
			continue
		}
		schema := adapter.Schema()
		if !schema.AgentLoop {
			continue
		}
		sleep := 5 * time.Second
		if schema.LoopSleep != "" {
			if d, err := time.ParseDuration(schema.LoopSleep); err == nil {
				sleep = d
			}
		}
		maxIter := schema.MaxIterations
		if maxIter <= 0 {
			maxIter = 10
		}
		out = append(out, AgentLoopConfig{
			PluginName:    schema.Name,
			ExecutorName:  schema.Name,
			MaxIterations: maxIter,
			LoopSleep:     sleep,
		})
	}
	return out
}
