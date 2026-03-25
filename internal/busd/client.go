package busd

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
)

// registrationFrame is the JSON frame a widget sends on connect.
type registrationFrame struct {
	Name      string   `json:"name"`
	Subscribe []string `json:"subscribe"`
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
		conn: conn,
		name: reg.Name,
		subs: reg.Subscribe,
		disc: make(chan struct{}),
	}

	// Start a goroutine that drains any further reads so we detect disconnect.
	go func() {
		defer close(c.disc)
		buf := make([]byte, 256)
		for {
			if _, err := conn.Read(buf); err != nil {
				return
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

// send writes a pre-encoded newline-terminated frame to the client.
// Returns an error if the write fails (caller should prune the client).
func (c *client) send(frame []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return net.ErrClosed
	}
	_, err := c.conn.Write(frame)
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
