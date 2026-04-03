// Package busd implements glitch's Unix socket event bus daemon.
//
// Widget binaries connect to the socket, send a registration JSON frame, and
// then receive newline-delimited JSON event frames for topics they subscribed
// to.
//
// Wire format (server → client):
//
//	{"event":"theme.changed","payload":{...}}\n
//
// Registration frame (client → server, sent once on connect):
//
//	{"name":"weather","subscribe":["theme.changed","session.*"]}\n
//
// Publish frame (client → server, after registration):
//
//	{"action":"publish","event":"theme.changed","payload":{...}}\n
//
// The daemon broadcasts publish frames to all subscribed clients, allowing
// any connected process to emit events without holding a Daemon reference.
// Use [PublishEvent] as a convenience wrapper.
package busd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// SocketPath returns the path to the Unix domain socket.
// Primary:  $XDG_RUNTIME_DIR/glitch/bus.sock
// Fallback: $XDG_CACHE_HOME/glitch/bus.sock  (via os.UserCacheDir)
func SocketPath() (string, error) {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "glitch", "bus.sock"), nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("busd: cannot determine socket path: %w", err)
	}
	return filepath.Join(cache, "glitch", "bus.sock"), nil
}

// Event is the decoded form of a server-to-client wire frame. It is exported
// so test helpers can unmarshal received frames into a typed value.
type Event struct {
	Event       string `json:"event"`
	Payload     any    `json:"payload"`
	Traceparent string `json:"traceparent,omitempty"`
}

// eventFrame is the wire format sent from the server to connected clients.
type eventFrame struct {
	Event       string `json:"event"`
	Payload     any    `json:"payload"`
	Traceparent string `json:"traceparent,omitempty"`
}

// ExtractContext returns a context with the trace context extracted from event's
// traceparent field. If the field is empty, ctx is returned unchanged.
func ExtractContext(ctx context.Context, evt Event) context.Context {
	if evt.Traceparent == "" {
		return ctx
	}
	carrier := propagation.MapCarrier{"traceparent": evt.Traceparent}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// injectTraceparent extracts the current span's traceparent from ctx and returns it.
func injectTraceparent(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

// matchTopic returns true when pattern matches topic.
// Supports exact match and wildcard suffix ("session.*" matches "session.started").
func matchTopic(pattern, topic string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == topic {
		return true
	}
	if prefix, ok := strings.CutSuffix(pattern, ".*"); ok {
		return strings.HasPrefix(topic, prefix+".")
	}
	return false
}

// Daemon is a Unix socket event bus.
type Daemon struct {
	mu       sync.RWMutex
	clients  map[*client]struct{}
	listener net.Listener
	done     chan struct{}
}

// New creates a new Daemon. Call Start() to begin accepting connections.
func New() *Daemon {
	return &Daemon{
		clients: make(map[*client]struct{}),
		done:    make(chan struct{}),
	}
}

// ClientCount returns the number of currently registered clients.
// Exported for testing only.
func (d *Daemon) ClientCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.clients)
}

// Start begins listening on the Unix socket resolved by SocketPath(). It is
// non-blocking — the accept loop runs in a background goroutine.
func (d *Daemon) Start() error {
	sockPath, err := SocketPath()
	if err != nil {
		return err
	}
	return d.StartAt(sockPath)
}

// StartAt begins listening on the given Unix socket path. It is non-blocking —
// the accept loop runs in a background goroutine. Callers can use this instead
// of Start() to specify a custom socket path (useful in tests where the temp
// directory would otherwise exceed the OS limit for socket path lengths).
func (d *Daemon) StartAt(sockPath string) error {
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(sockPath), 0o700); err != nil {
		return fmt.Errorf("busd: mkdir socket dir: %w", err)
	}

	// Remove a stale socket file if one exists.
	_ = os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("busd: listen %s: %w", sockPath, err)
	}
	d.listener = ln

	go d.acceptLoop()
	return nil
}

// Stop closes all client connections, removes the socket file, and waits for
// the accept goroutine to exit.
func (d *Daemon) Stop() {
	if d.listener != nil {
		sockPath := d.listener.Addr().String()
		d.listener.Close()
		<-d.done
		_ = os.Remove(sockPath)
	}

	d.mu.Lock()
	for c := range d.clients {
		c.close()
	}
	d.clients = make(map[*client]struct{})
	d.mu.Unlock()
}

// PublishCtx delivers an event to all clients subscribed to topic, injecting
// the current span's traceparent into the wire frame.
func (d *Daemon) PublishCtx(ctx context.Context, topic string, payload any) error {
	tp := injectTraceparent(ctx)
	frame, err := json.Marshal(eventFrame{Event: topic, Payload: payload, Traceparent: tp})
	if err != nil {
		return fmt.Errorf("busd: marshal event: %w", err)
	}
	frame = append(frame, '\n')

	_, span := otel.Tracer("busd").Start(ctx, "bus.publish",
		trace.WithAttributes(
			attribute.String("bus.topic", topic),
			attribute.Int("bus.payload_bytes", len(frame)),
		),
	)
	defer span.End()

	d.mu.RLock()
	var targets []*client
	for c := range d.clients {
		if c.matches(topic) {
			targets = append(targets, c)
		}
	}
	d.mu.RUnlock()

	var dead []*client
	for _, c := range targets {
		if err := c.send(frame); err != nil {
			dead = append(dead, c)
		}
	}

	if len(dead) > 0 {
		d.mu.Lock()
		for _, c := range dead {
			delete(d.clients, c)
			c.close()
		}
		d.mu.Unlock()
	}
	return nil
}

// Publish delivers an event to all clients subscribed to topic. It is
// non-blocking — slow or disconnected clients are silently pruned.
func (d *Daemon) Publish(topic string, payload any) error {
	frame, err := json.Marshal(eventFrame{Event: topic, Payload: payload})
	if err != nil {
		return fmt.Errorf("busd: marshal event: %w", err)
	}
	frame = append(frame, '\n')

	// Snapshot matching clients under a read lock so concurrent Publish calls
	// and handleConn registrations are not serialized behind a write lock.
	d.mu.RLock()
	var targets []*client
	for c := range d.clients {
		if c.matches(topic) {
			targets = append(targets, c)
		}
	}
	d.mu.RUnlock()

	// Write to each client without holding any lock. client.send is
	// internally synchronized, so concurrent Publish calls are safe.
	var dead []*client
	for _, c := range targets {
		if err := c.send(frame); err != nil {
			dead = append(dead, c)
		}
	}

	// Prune dead clients under a single write lock.
	if len(dead) > 0 {
		d.mu.Lock()
		for _, c := range dead {
			delete(d.clients, c)
			c.close()
		}
		d.mu.Unlock()
	}
	return nil
}

func (d *Daemon) acceptLoop() {
	defer close(d.done)
	for {
		conn, err := d.listener.Accept()
		if err != nil {
			// Listener was closed — normal shutdown.
			return
		}
		go d.handleConn(conn)
	}
}

func (d *Daemon) handleConn(conn net.Conn) {
	c, err := newClient(conn)
	if err != nil {
		// Registration failed — close and discard.
		conn.Close()
		return
	}

	d.mu.Lock()
	d.clients[c] = struct{}{}
	d.mu.Unlock()

	// Relay any publish frames forwarded by this client to all subscribers.
	go func() {
		for f := range c.publishCh {
			var payload any
			if len(f.Payload) > 0 {
				_ = json.Unmarshal(f.Payload, &payload)
			}
			_ = d.Publish(f.Event, payload)
		}
	}()

	// Block until the client disconnects (reads EOF or error).
	c.wait()

	// Invariant: if Stop() already ran it replaced d.clients with a fresh map,
	// so the delete below is a harmless no-op. c.close() is idempotent (guarded
	// by c.closed), so calling it here after Stop() has already closed it is
	// also safe. No special ordering between handleConn and Stop is required.
	d.mu.Lock()
	delete(d.clients, c)
	d.mu.Unlock()
	c.close()
}

// publishClientFrame is the wire frame sent by PublishEvent to the daemon.
type publishClientFrame struct {
	Action  string `json:"action"`
	Event   string `json:"event"`
	Payload any    `json:"payload"`
}

// publishClientFrameWithTrace is the wire frame sent by PublishEventCtx, carrying trace context.
type publishClientFrameWithTrace struct {
	Action      string `json:"action"`
	Event       string `json:"event"`
	Payload     any    `json:"payload"`
	Traceparent string `json:"traceparent,omitempty"`
}

// PublishEvent dials the busd daemon at sockPath, sends a publish frame for
// topic with payload, and closes the connection. The daemon broadcasts the
// event to all subscribers.
//
// This is the primary mechanism for out-of-process event publishing — callers
// that do not hold a *Daemon reference (e.g. the deck subprocess) use
// this to emit events onto the bus.
//
// Returns nil if the daemon is not running (dial fails), so callers can safely
// call this without checking whether busd is available.
func PublishEvent(sockPath, topic string, payload any) error {
	return PublishEventCtx(context.Background(), sockPath, topic, payload)
}

// PublishEventCtx is like PublishEvent but injects the current span's traceparent
// into the wire frame so subscribers can continue the trace.
func PublishEventCtx(ctx context.Context, sockPath, topic string, payload any) error {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		// Daemon not running — degrade silently.
		return nil
	}
	defer conn.Close()

	// Registration frame — subscribe to nothing, just here to publish.
	reg := registrationFrame{Name: "publisher", Subscribe: nil}
	regBytes, _ := json.Marshal(reg)
	regBytes = append(regBytes, '\n')
	if _, err := conn.Write(regBytes); err != nil {
		return fmt.Errorf("busd: write registration: %w", err)
	}

	// Publish frame with traceparent.
	pub := publishClientFrameWithTrace{
		Action:      "publish",
		Event:       topic,
		Payload:     payload,
		Traceparent: injectTraceparent(ctx),
	}
	pubBytes, err := json.Marshal(pub)
	if err != nil {
		return fmt.Errorf("busd: marshal publish frame: %w", err)
	}
	pubBytes = append(pubBytes, '\n')
	if _, err := conn.Write(pubBytes); err != nil {
		return fmt.Errorf("busd: write publish frame: %w", err)
	}
	return nil
}
