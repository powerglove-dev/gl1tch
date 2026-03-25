package plugin

import (
	"context"
	"io"
)

// Plugin is the universal interface all orcai plugins implement,
// regardless of whether they are native go-plugins (Tier 1) or CLI wrappers (Tier 2).
//
// Execute parameters:
//   - input: the primary data payload / stdin for this invocation (prompt text or raw content)
//   - vars: string metadata passed as environment/template variables — not structured data;
//     for typed structured data use ExecuteRequest.Args (see proto/orcai/v1/plugin.proto)
type Plugin interface {
	Name() string
	Description() string
	Capabilities() []Capability
	Execute(ctx context.Context, input string, vars map[string]string, w io.Writer) error
	Close() error
}

// Capability describes one thing a plugin can do.
type Capability struct {
	Name         string
	InputSchema  string
	OutputSchema string
}

// StubPlugin is a test double that satisfies the Plugin interface.
type StubPlugin struct {
	PluginName string
	PluginDesc string
	PluginCaps []Capability
	ExecuteFn  func(ctx context.Context, input string, vars map[string]string, w io.Writer) error
}

func (s *StubPlugin) Name() string              { return s.PluginName }
func (s *StubPlugin) Description() string        { return s.PluginDesc }
func (s *StubPlugin) Capabilities() []Capability { return s.PluginCaps }
func (s *StubPlugin) Close() error               { return nil }
func (s *StubPlugin) Execute(ctx context.Context, input string, vars map[string]string, w io.Writer) error {
	if s.ExecuteFn != nil {
		return s.ExecuteFn(ctx, input, vars, w)
	}
	return nil
}
