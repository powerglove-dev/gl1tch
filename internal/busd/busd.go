// Package busd implements orcai's Unix socket event bus daemon.
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
package busd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SocketPath returns the path to the Unix domain socket.
// Primary:  $XDG_RUNTIME_DIR/orcai/bus.sock
// Fallback: $XDG_CACHE_HOME/orcai/bus.sock  (via os.UserCacheDir)
func SocketPath() (string, error) {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "orcai", "bus.sock"), nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("busd: cannot determine socket path: %w", err)
	}
	return filepath.Join(cache, "orcai", "bus.sock"), nil
}

// eventFrame is the wire format sent from the server to connected clients.
type eventFrame struct {
	Event   string `json:"event"`
	Payload any    `json:"payload"`
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

// Publish delivers an event to all clients subscribed to topic. It is
// non-blocking — slow or disconnected clients are silently pruned.
func (d *Daemon) Publish(topic string, payload any) error {
	frame, err := json.Marshal(eventFrame{Event: topic, Payload: payload})
	if err != nil {
		return fmt.Errorf("busd: marshal event: %w", err)
	}
	frame = append(frame, '\n')

	d.mu.Lock()
	defer d.mu.Unlock()

	var dead []*client
	for c := range d.clients {
		if !c.matches(topic) {
			continue
		}
		if err := c.send(frame); err != nil {
			dead = append(dead, c)
		}
	}
	for _, c := range dead {
		c.close()
		delete(d.clients, c)
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

	// Block until the client disconnects (reads EOF or error).
	c.wait()

	d.mu.Lock()
	delete(d.clients, c)
	d.mu.Unlock()
}
