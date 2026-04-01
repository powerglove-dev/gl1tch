package pipeline_test

import (
	"context"
	"testing"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

func TestNoopPublisher_Publish(t *testing.T) {
	pub := pipeline.NoopPublisher{}
	if err := pub.Publish(context.Background(), "test.topic", []byte("payload")); err != nil {
		t.Errorf("NoopPublisher.Publish returned error: %v", err)
	}
}

func TestNoopPublisher_NilPayload(t *testing.T) {
	pub := pipeline.NoopPublisher{}
	if err := pub.Publish(context.Background(), "test.topic", nil); err != nil {
		t.Errorf("NoopPublisher.Publish with nil payload returned error: %v", err)
	}
}

// capturePublisher records published events for test assertions.
type capturePublisher struct {
	events []capturedEvent
}

type capturedEvent struct {
	topic   string
	payload []byte
}

func (c *capturePublisher) Publish(_ context.Context, topic string, payload []byte) error {
	c.events = append(c.events, capturedEvent{topic: topic, payload: payload})
	return nil
}

func TestEventPublisher_Interface(t *testing.T) {
	// Verify capturePublisher satisfies EventPublisher.
	var pub pipeline.EventPublisher = &capturePublisher{}
	if err := pub.Publish(context.Background(), "a.b", []byte("data")); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
