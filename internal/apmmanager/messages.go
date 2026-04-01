// Package apmmanager provides a BubbleTea component for browsing, installing,
// and activating APM (Agent Package Manager) agents within the glitch TUI.
// It also exposes [AgentCapabilityProvider], an interface glitch executors use to
// request and receive agent capabilities at runtime.
package apmmanager

import "github.com/powerglove-dev/gl1tch/internal/executor"

// ── Public tea.Msg types ────────────────────────────────────────────────────
// These are exported because parent components and executors need to handle them.

// AgentListLoadedMsg is dispatched when the initial agent manifest scan completes.
// If Err is non-nil the list is empty and the error is displayed in the TUI.
type AgentListLoadedMsg struct {
	Agents []Agent
	Err    error
}

// AgentInstallStartMsg signals that the ApmManager has begun installing an agent.
// Parents can use this to show a global progress indicator.
type AgentInstallStartMsg struct {
	AgentID string
}

// AgentInstallDoneMsg is dispatched when an agent installation succeeds.
// ExecutorID is the name under which the new CliAdapter was registered in
// the executor.Manager, and Adapter is the live adapter instance.
type AgentInstallDoneMsg struct {
	AgentID  string
	ExecutorID string
	Adapter  *executor.CliAdapter
}

// AgentInstallErrMsg is dispatched when an agent installation fails.
type AgentInstallErrMsg struct {
	AgentID string
	Err     error
}

// AgentActivatedMsg signals to the parent that an agent is now live and its
// executor is registered. The parent should route tasks to ExecutorID going forward.
type AgentActivatedMsg struct {
	AgentID  string
	ExecutorID string
}

// AgentUninstallDoneMsg signals that an agent was removed from disk and its
// executor deregistered.
type AgentUninstallDoneMsg struct {
	AgentID string
}

// ── Internal tea.Msg types ──────────────────────────────────────────────────
// These are unexported; only the apmmanager package dispatches them.

// agentInstallResultMsg carries the outcome of a background install command.
// It is converted to AgentInstallDoneMsg or AgentInstallErrMsg in Update.
type agentInstallResultMsg struct {
	agentID  string
	executorID string
	adapter  *executor.CliAdapter
	err      error
}

// agentScanResultMsg carries the raw scan results from loadAgentsCmd.
type agentScanResultMsg struct {
	agents []Agent
	err    error
}
