package cmd

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/8op-org/gl1tch/internal/busd"
)

// busPublisher wraps busd.PublishEvent as a pipeline.EventPublisher.
// It resolves the socket path once and uses PublishEvent (which dials per-call).
// If the socket is unavailable, Publish silently returns nil.
type busPublisher struct {
	sockPath string
}

// newBusPublisher attempts to find the busd socket. Returns nil if unavailable.
func newBusPublisher() *busPublisher {
	sockPath, err := busd.SocketPath()
	if err != nil {
		return nil
	}
	// Quick reachability check — don't fail if bus is down.
	conn, err := net.DialTimeout("unix", sockPath, 100*time.Millisecond)
	if err != nil {
		return nil
	}
	conn.Close()
	return &busPublisher{sockPath: sockPath}
}

// Publish implements pipeline.EventPublisher. It forwards the pre-marshaled
// payload to the busd daemon as a json.RawMessage so it is not double-marshaled.
func (b *busPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	// PublishEvent degrades silently when the daemon is unreachable.
	_ = busd.PublishEvent(b.sockPath, topic, json.RawMessage(payload))
	return nil
}
