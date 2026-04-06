package supervisor

import "context"

// Service is a long-running background process managed by the supervisor.
// Unlike Handler (which reacts to busd events), a Service runs continuously
// in its own goroutine until the context is cancelled.
type Service interface {
	// Name returns a short human-readable identifier for logging.
	Name() string
	// Start runs the service. It should block until ctx is cancelled or a
	// fatal error occurs. The supervisor calls this in a goroutine.
	Start(ctx context.Context) error
}
