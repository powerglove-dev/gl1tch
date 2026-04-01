package apmmanager

import (
	"context"
	"fmt"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// AgentCapabilityProvider lets glitch executors request agent capabilities at
// runtime without knowing whether the required agent is already installed.
//
// GetAgent returns the Agent immediately from the in-memory registry (or false).
// RequireAgent blocks until the agent is installed and its executor is registered,
// then returns it. If installation fails, it returns a descriptive error.
//
// Plugins that need a capability should call RequireAgent at point-of-use:
//
//	agent, err := provider.RequireAgent(ctx, "api-architect")
//	if err != nil {
//	    return fmt.Errorf("need api-architect agent: %w", err)
//	}
//	// agent.ExecutorID is now registered in the executor.Manager
type AgentCapabilityProvider interface {
	// GetAgent returns the Agent for id if it is already installed.
	GetAgent(id string) (Agent, bool)
	// RequireAgent ensures the agent is installed and returns it.
	// Blocks until installation completes or ctx is cancelled.
	RequireAgent(ctx context.Context, id string) (Agent, error)
}

// DefaultProvider implements AgentCapabilityProvider by synchronising with the
// Model's in-memory agent list and delegating installs to installAndWrap.
//
// Construct one via NewDefaultProvider and pass it to executors that need
// on-demand agent capabilities. The send function, if non-nil, is called with
// progress messages (AgentInstallStartMsg / AgentInstallDoneMsg /
// AgentInstallErrMsg) so the TUI can track install state.
type DefaultProvider struct {
	mu          sync.RWMutex
	agents      map[string]Agent  // keyed by Agent.ID
	projectRoot string
	send        func(tea.Msg) // may be nil; used to broadcast progress to the TUI
	model       *Model        // non-nil when provider is wired directly to the model
}

// NewDefaultProvider creates a DefaultProvider backed by the given Model.
// send is optional; if non-nil it is called with install progress messages.
func NewDefaultProvider(m *Model, send func(tea.Msg)) *DefaultProvider {
	p := &DefaultProvider{
		agents:      make(map[string]Agent),
		projectRoot: m.projectRoot,
		send:        send,
		model:       m,
	}
	// Seed from the model's current agent list.
	for _, a := range m.agents {
		p.agents[a.ID] = a
	}
	return p
}

// Sync refreshes the provider's in-memory copy from the Model's current agent
// list. Call this whenever the Model processes an AgentInstallDoneMsg.
func (p *DefaultProvider) Sync(agents []Agent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, a := range agents {
		p.agents[a.ID] = a
	}
}

// GetAgent returns the Agent for id if it is known and installed.
func (p *DefaultProvider) GetAgent(id string) (Agent, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	a, ok := p.agents[id]
	if !ok || a.InstallState != StateInstalled {
		return Agent{}, false
	}
	return a, true
}

// RequireAgent ensures the agent identified by id is installed and its
// CliAdapter is registered in the executor.Manager.
//
// If the agent is already installed it returns immediately.
// If it is not installed, RequireAgent runs apm install synchronously (within
// ctx) and registers the resulting CliAdapter before returning.
func (p *DefaultProvider) RequireAgent(ctx context.Context, id string) (Agent, error) {
	// Fast path: already installed.
	if a, ok := p.GetAgent(id); ok {
		return a, nil
	}

	// Lock while we check-and-set to avoid duplicate concurrent installs.
	p.mu.Lock()
	// Re-check under write lock.
	if a, ok := p.agents[id]; ok && a.InstallState == StateInstalled {
		p.mu.Unlock()
		return a, nil
	}
	// Mark as installing so concurrent callers wait on the same slot.
	stub := p.agents[id]
	stub.ID = id
	stub.InstallState = StateInstalling
	p.agents[id] = stub
	executorMgr := p.model.executorMgr
	projectRoot := p.projectRoot
	p.mu.Unlock()

	// Broadcast start to TUI (best-effort).
	p.emit(AgentInstallStartMsg{AgentID: id})

	// Build a minimal Agent struct for installAndWrap.
	a := Agent{
		ID:   id,
		Name: agentBaseName(id),
	}

	_, adapter, err := installAndWrap(ctx, projectRoot, a, executorMgr)
	if err != nil {
		p.mu.Lock()
		entry := p.agents[id]
		entry.InstallState = StateError
		entry.ErrMsg = err.Error()
		p.agents[id] = entry
		p.mu.Unlock()

		p.emit(AgentInstallErrMsg{AgentID: id, Err: err})
		return Agent{}, fmt.Errorf("require agent %s: %w", id, err)
	}

	// Locate the newly deployed .agent.md to enrich the Agent struct.
	agentMDPath, _ := findDeployedAgentMD(projectRoot, id)
	installed := parseAgentMD(agentMDPath)
	if installed.ID == "" {
		installed.ID = id
	}
	installed.InstallState = StateInstalled
	installed.AgentMDPath = agentMDPath
	installed.ExecutorID = adapter.Name()

	p.mu.Lock()
	p.agents[id] = installed
	p.mu.Unlock()

	p.emit(AgentInstallDoneMsg{
		AgentID:  id,
		ExecutorID: adapter.Name(),
		Adapter:  adapter,
	})

	return installed, nil
}

// emit calls p.send if it is non-nil.
func (p *DefaultProvider) emit(msg tea.Msg) {
	if p.send != nil {
		p.send(msg)
	}
}

// agentBaseName derives a human-readable name from an APM agent ID.
// e.g. "anthropics/skills/agents/api-architect" → "api-architect"
func agentBaseName(id string) string {
	segs := splitLast(id, "/")
	if segs[1] != "" {
		return segs[1]
	}
	return id
}

// splitLast splits s on the last occurrence of sep, returning a two-element
// array [before, after]. If sep is not found, [s, ""] is returned.
func splitLast(s, sep string) [2]string {
	idx := -1
	for i := len(s) - len(sep); i >= 0; i-- {
		if s[i:i+len(sep)] == sep {
			idx = i
			break
		}
	}
	if idx < 0 {
		return [2]string{s, ""}
	}
	return [2]string{s[:idx], s[idx+len(sep):]}
}
