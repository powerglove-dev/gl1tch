//go:build ignore

// Package apmmanager — example_executor_patch.go
//
// This file is a living example showing how an existing glitch executor (or pipeline
// builtin step) calls RequireAgent to unlock a capability it needs at runtime.
// Build tag "ignore" keeps it out of the normal build; it is documentation-as-code.
//
// The pattern:
//  1. Accept an AgentCapabilityProvider at construction time.
//  2. Call RequireAgent(ctx, id) at point-of-use — not at startup.
//  3. Use the returned Agent.ExecutorID to look up the registered CliAdapter in the
//     executor.Manager and dispatch the user's input.
//
// This lets executors self-enhance on first use: if the needed agent isn't installed,
// ApmManager installs it on demand and the executor continues with the new capability.

package apmmanager

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/powerglove-dev/gl1tch/internal/executor"
)

// ── Example 1: a pipeline builtin step ──────────────────────────────────────

// AgentStepArgs are the args a pipeline author writes in their .pipeline.yaml.
//
//	steps:
//	  - id: summarize
//	    executor: builtin.agent_step
//	    args:
//	      agent_id: api-architect
//	      prompt: "Review the following API surface and suggest improvements"
type AgentStepArgs struct {
	AgentID string `json:"agent_id"`
	Prompt  string `json:"prompt"`
}

// AgentBuiltinStep is a pipeline BuiltinFunc that delegates to an APM agent.
// It is wired into builtinRegistry in pipeline/builtin.go as "builtin.agent_step".
//
// Before this change, the pipeline step could only call pre-registered CLI
// adapters. After this change it can install any APM agent on demand.
//
// Patch in pipeline/builtin.go:
//
//	import "github.com/powerglove-dev/gl1tch/internal/apmmanager"
//
//	func NewBuiltinRegistry(provider apmmanager.AgentCapabilityProvider) map[string]BuiltinFunc {
//	    return map[string]BuiltinFunc{
//	        // … existing builtins …
//	        "builtin.agent_step": AgentBuiltinStep(provider),
//	    }
//	}
func AgentBuiltinStep(provider AgentCapabilityProvider) func(ctx context.Context, args map[string]any, w io.Writer) (map[string]any, error) {
	return func(ctx context.Context, args map[string]any, w io.Writer) (map[string]any, error) {
		agentID, _ := args["agent_id"].(string)
		prompt, _ := args["prompt"].(string)

		if agentID == "" {
			return nil, fmt.Errorf("builtin.agent_step: agent_id is required")
		}

		// RequireAgent installs the agent if not already present, then returns it.
		// This blocks until the install completes or ctx is cancelled.
		agent, err := provider.RequireAgent(ctx, agentID)
		if err != nil {
			return nil, fmt.Errorf("builtin.agent_step: %w", err)
		}

		// The agent's CliAdapter is now registered under agent.ExecutorID.
		// Here we call its Execute method directly to stream output to w.
		// In a real wiring, you'd retrieve the adapter from executor.Manager.
		_ = agent
		fmt.Fprintf(w, "[agent:%s] %s\n", agent.ExecutorID, prompt)

		return map[string]any{"agent_id": agent.ID, "executor_id": agent.ExecutorID}, nil
	}
}

// ── Example 2: a CliAdapter-backed executor that self-enhances ────────────────

// ReviewerExecutor wraps a static CLI tool but can escalate to an APM code-review
// agent when the user's input contains a Go file path.
//
// This demonstrates the "self-enhance" pattern: the executor starts as a thin
// wrapper and calls RequireAgent only when it detects the heavier capability
// is needed — avoiding the install cost for simple inputs.
type ReviewerExecutor struct {
	provider    AgentCapabilityProvider
	executorMgr *executor.Manager
	base        *executor.CliAdapter // fast path: static linter
}

func NewReviewerExecutor(base *executor.CliAdapter, provider AgentCapabilityProvider, mgr *executor.Manager) *ReviewerExecutor {
	return &ReviewerExecutor{provider: provider, executorMgr: mgr, base: base}
}

func (r *ReviewerExecutor) Name() string               { return "reviewer" }
func (r *ReviewerExecutor) Description() string         { return "Code reviewer — static linter + optional APM agent escalation" }
func (r *ReviewerExecutor) Capabilities() []executor.Capability { return r.base.Capabilities() }
func (r *ReviewerExecutor) Close() error                { return nil }

// Execute runs the static linter for quick feedback, but if the input mentions
// a Go file it escalates to the APM "go-reviewer" agent for deep analysis.
func (r *ReviewerExecutor) Execute(ctx context.Context, input string, vars map[string]string, w io.Writer) error {
	// Fast path: delegate to the base static linter.
	if !strings.Contains(input, ".go") {
		return r.base.Execute(ctx, input, vars, w)
	}

	// Escalation path: require the APM agent for Go-specific deep review.
	agent, err := r.provider.RequireAgent(ctx, "go-reviewer")
	if err != nil {
		// Graceful degradation: fall back to the static linter if the agent
		// is unavailable (no network, APM not installed, etc.).
		fmt.Fprintf(w, "[reviewer] go-reviewer agent unavailable (%v); using static linter\n", err)
		return r.base.Execute(ctx, input, vars, w)
	}

	// Retrieve the registered CliAdapter from the executor.Manager and execute.
	agentExec, ok := r.executorMgr.Get(agent.ExecutorID)
	if !ok {
		return fmt.Errorf("reviewer: executor %s not found after RequireAgent", agent.ExecutorID)
	}
	return agentExec.Execute(ctx, input, vars, w)
}
