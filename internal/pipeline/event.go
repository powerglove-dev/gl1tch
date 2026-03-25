package pipeline

import "context"

// EventPublisher publishes pipeline lifecycle events to an event bus.
// Implementations must be safe to call concurrently.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// NoopPublisher is a nil-safe EventPublisher that discards all events.
// Use it when no bus is available or in tests that don't need event assertions.
type NoopPublisher struct{}

func (NoopPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
