package executor

import (
	"context"
	"io"
)

// BusClient is the interface a BusAwareExecutor uses to interact with the event bus.
// It is satisfied by *busd.ConnectedClient or NoopBusClient.
// Defined here (not in busd) to avoid import cycles — the executor package must
// not import busd.
type BusClient interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// BusAwareExecutor is an optional interface a Tier 1 executor may implement to
// receive a BusClient on startup. The base Executor interface is unchanged —
// existing executors need no modification.
type BusAwareExecutor interface {
	Executor
	SetBusClient(c BusClient)
}

// Executor is the universal interface all orcai executors implement,
// regardless of whether they are native go-plugins (Tier 1) or CLI wrappers (Tier 2).
//
// Execute parameters:
//   - input: the primary data payload / stdin for this invocation (prompt text or raw content)
//   - vars: string metadata passed as environment/template variables — not structured data;
//     for typed structured data use ExecuteRequest.Args (see proto/orcai/v1/executor.proto)
type Executor interface {
	Name() string
	Description() string
	Capabilities() []Capability
	Execute(ctx context.Context, input string, vars map[string]string, w io.Writer) error
	Close() error
}

// Capability describes one thing an executor can do.
type Capability struct {
	Name         string
	InputSchema  string
	OutputSchema string
}

// StubExecutor is a test double that satisfies the Executor interface.
type StubExecutor struct {
	ExecutorName string
	ExecutorDesc string
	ExecutorCaps []Capability
	ExecuteFn    func(ctx context.Context, input string, vars map[string]string, w io.Writer) error
}

func (s *StubExecutor) Name() string               { return s.ExecutorName }
func (s *StubExecutor) Description() string         { return s.ExecutorDesc }
func (s *StubExecutor) Capabilities() []Capability  { return s.ExecutorCaps }
func (s *StubExecutor) Close() error                { return nil }
func (s *StubExecutor) Execute(ctx context.Context, input string, vars map[string]string, w io.Writer) error {
	if s.ExecuteFn != nil {
		return s.ExecuteFn(ctx, input, vars, w)
	}
	return nil
}
