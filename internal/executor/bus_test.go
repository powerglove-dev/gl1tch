package executor_test

import (
	"context"
	"testing"

	"github.com/adam-stokes/orcai/internal/executor"
)

// noopBusClientForTest satisfies executor.BusClient without importing busd,
// avoiding a test-only import cycle.
type noopBusClientForTest struct{}

func (noopBusClientForTest) Publish(_ context.Context, _ string, _ []byte) error { return nil }

// busAwareStub is a StubExecutor that also implements BusAwareExecutor.
type busAwareStub struct {
	executor.StubExecutor
	client executor.BusClient
}

func (b *busAwareStub) SetBusClient(c executor.BusClient) { b.client = c }

func TestBusAwareExecutor_ReceivesClientOnSetBusClient(t *testing.T) {
	mgr := executor.NewManager()
	e := &busAwareStub{StubExecutor: executor.StubExecutor{ExecutorName: "test"}}
	if err := mgr.Register(e); err != nil {
		t.Fatalf("Register: %v", err)
	}

	noop := noopBusClientForTest{}
	mgr.SetBusClient(noop)

	if e.client == nil {
		t.Fatal("expected client to be set after SetBusClient")
	}

	// Publish should not panic and should return nil.
	err := e.client.Publish(context.Background(), "test.topic", []byte("{}"))
	if err != nil {
		t.Fatalf("unexpected error from noop Publish: %v", err)
	}
}

func TestBusAwareExecutor_ReceivesClientOnRegister(t *testing.T) {
	mgr := executor.NewManager()

	// Set the bus client before registering any executor.
	noop := noopBusClientForTest{}
	mgr.SetBusClient(noop)

	// Executor registered after SetBusClient should also receive the client.
	e := &busAwareStub{StubExecutor: executor.StubExecutor{ExecutorName: "late"}}
	if err := mgr.Register(e); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if e.client == nil {
		t.Fatal("expected client to be injected during Register")
	}
}

func TestNonBusAwareExecutor_UnaffectedBySetBusClient(t *testing.T) {
	mgr := executor.NewManager()
	plain := &executor.StubExecutor{ExecutorName: "plain"}
	if err := mgr.Register(plain); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Should not panic — plain executor doesn't implement BusAwareExecutor.
	mgr.SetBusClient(noopBusClientForTest{})

	if _, ok := mgr.Get("plain"); !ok {
		t.Error("plain executor should still be registered")
	}
}
