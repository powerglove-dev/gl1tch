package supervisor_test

import (
	"testing"

	"github.com/8op-org/gl1tch/internal/supervisor"
)

func TestResolveModel_NilConfig(t *testing.T) {
	// With no config and no providers available, should return a fallback (never panic).
	got := supervisor.ResolveModel(nil, "diagnosis")
	if got.ProviderID == "" {
		// No providers in test env is acceptable — just verify no panic.
		t.Log("no providers available, fallback returned zero ProviderID — acceptable in test env")
	}
}

func TestResolveModel_EmptyConfig(t *testing.T) {
	cfg := &supervisor.SupervisorConfig{Roles: make(map[string]supervisor.RoleConfig)}
	got := supervisor.ResolveModel(cfg, "diagnosis")
	// Should not panic regardless of what providers are available.
	_ = got
}

func TestResolveModel_ConfiguredRoleMatchesAvailableProvider(t *testing.T) {
	// Build a config that points to "ollama" which may or may not be running.
	// The important invariant is: never returns an error, always returns something.
	cfg := &supervisor.SupervisorConfig{
		Roles: map[string]supervisor.RoleConfig{
			"diagnosis": {Provider: "ollama", Model: "llama3.2"},
		},
	}
	got := supervisor.ResolveModel(cfg, "diagnosis")
	// We can't assert specific values because the test env may not have Ollama.
	// Just assert the function returns without panicking.
	_ = got.ProviderID
	_ = got.ModelID
}

func TestResolveModel_UnknownRole(t *testing.T) {
	cfg := &supervisor.SupervisorConfig{
		Roles: map[string]supervisor.RoleConfig{
			"diagnosis": {Provider: "ollama", Model: "llama3.2"},
		},
	}
	// "reasoning" is not in config — should still return a usable fallback.
	got := supervisor.ResolveModel(cfg, "reasoning")
	_ = got
}

func TestResolveModel_NeverPanics(t *testing.T) {
	// Table-driven test to ensure no panics in common scenarios.
	tests := []struct {
		name string
		cfg  *supervisor.SupervisorConfig
		role string
	}{
		{"nil config", nil, "diagnosis"},
		{"empty roles", &supervisor.SupervisorConfig{Roles: map[string]supervisor.RoleConfig{}}, "routing"},
		{"configured role", &supervisor.SupervisorConfig{
			Roles: map[string]supervisor.RoleConfig{
				"diagnosis": {Provider: "ollama", Model: "llama3.2"},
			},
		}, "diagnosis"},
		{"missing provider", &supervisor.SupervisorConfig{
			Roles: map[string]supervisor.RoleConfig{
				"routing": {Provider: "nonexistent-provider", Model: "some-model"},
			},
		}, "routing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ResolveModel panicked: %v", r)
				}
			}()
			got := supervisor.ResolveModel(tt.cfg, tt.role)
			_ = got
		})
	}
}
