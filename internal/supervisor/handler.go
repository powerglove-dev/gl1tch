package supervisor

import "context"

// Event carries a decoded busd event received by the supervisor.
type Event struct {
	Topic   string
	Payload []byte
}

// Handler is implemented by anything that can react to busd events.
type Handler interface {
	// Name returns a short human-readable identifier for logging.
	Name() string
	// Topics returns the busd topic patterns this handler reacts to.
	// Patterns follow the same wildcard rules as busd subscriptions
	// (e.g. "pipeline.run.*", "notification.*").
	Topics() []string
	// Handle is called for each matching event. model is pre-resolved for
	// the handler's role so implementations don't need to call ResolveModel.
	Handle(ctx context.Context, evt Event, model ResolvedModel) error
}
