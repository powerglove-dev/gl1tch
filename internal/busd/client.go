package busd

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
	"time"
)

// registrationFrame is the JSON frame a widget sends on connect.
type registrationFrame struct {
	Name      string   `json:"name"`
	Subscribe []string `json:"subscribe"`
}

// incomingFrame is a frame sent by a client after registration.
// When Action is "publish", the daemon re-broadcasts the event to all
// subscribers. Other actions are silently ignored.
type incomingFrame struct {
	Action  string          `json:"action"`  // "publish"
	Event   string          `json:"event"`   // topic
	Payload json.RawMessage `json:"payload"` // arbitrary JSON
}

// client represents a connected widget binary.
type client struct {
	conn   net.Conn
	name   string
	subs   []string
	mu     sync.Mutex
	closed bool
	// disc is closed when the client's read loop ends (disconnect detected).
	disc chan struct{}
	// publishCh receives incomingFrames with action=="publish" for relay.
	publishCh chan incomingFrame
}

// newClient reads the registration frame from conn and returns a ready client.
// Returns an error if the frame is missing, malformed, or has no name.
func newClient(conn net.Conn) (*client, error) {
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		err := scanner.Err()
		if err == nil {
			err = net.ErrClosed
		}
		return nil, err
	}

	var reg registrationFrame
	if err := json.Unmarshal(scanner.Bytes(), &reg); err != nil {
		return nil, err
	}
	if reg.Name == "" {
		reg.Name = "unknown"
	}

	c := &client{
		conn:      conn,
		name:      reg.Name,
		subs:      reg.Subscribe,
		disc:      make(chan struct{}),
		publishCh: make(chan incomingFrame, 16),
	}

	// Read post-registration frames. Reuse the same scanner so that any
	// bytes already buffered (e.g. a publish frame written back-to-back with
	// the registration frame by PublishEvent) are not lost. A fresh scanner
	// would read from conn directly, missing data already in the buffer.
	go func() {
		defer close(c.disc)
		for scanner.Scan() {
			var f incomingFrame
			if err := json.Unmarshal(scanner.Bytes(), &f); err != nil {
				continue
			}
			if f.Action == "publish" && f.Event != "" {
				select {
				case c.publishCh <- f:
				default: // drop if channel full
				}
			}
		}
	}()

	return c, nil
}

// matches reports whether any of the client's subscriptions cover topic.
func (c *client) matches(topic string) bool {
	for _, sub := range c.subs {
		if matchTopic(sub, topic) {
			return true
		}
	}
	return false
}

// sendDeadline is the maximum time allowed for a single write to a widget client.
// Slow clients that cannot drain their socket buffer within this window are pruned.
const sendDeadline = 5 * time.Millisecond

// send writes a pre-encoded newline-terminated frame to the client.
// A short write deadline ensures Publish never blocks on a slow client.
// Returns an error if the write fails or times out (caller should prune the client).
func (c *client) send(frame []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return net.ErrClosed
	}
	c.conn.SetWriteDeadline(time.Now().Add(sendDeadline)) //nolint:errcheck
	_, err := c.conn.Write(frame)
	c.conn.SetWriteDeadline(time.Time{}) //nolint:errcheck — reset deadline
	return err
}

// close tears down the underlying connection.
func (c *client) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		c.conn.Close()
	}
}

// wait blocks until the client's read goroutine exits (i.e. the client
// disconnected).
func (c *client) wait() {
	<-c.disc
}
