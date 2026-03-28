package busd_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adam-stokes/orcai/internal/busd"
)

// ---- helpers ----------------------------------------------------------------

// testSockCounter generates unique short socket paths under /tmp to stay well
// within the macOS 104-byte Unix socket path limit.
var testSockCounter atomic.Int64

func tempSockPath(t *testing.T) string {
	t.Helper()
	n := testSockCounter.Add(1)
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("busd%d", n))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "bus.sock")
}

// startDaemon starts a Daemon at a short socket path and returns the daemon
// and path. The daemon is stopped on test cleanup.
func startDaemon(t *testing.T) (*busd.Daemon, string) {
	t.Helper()
	sockPath := tempSockPath(t)
	d := busd.New()
	if err := d.StartAt(sockPath); err != nil {
		t.Fatalf("daemon StartAt: %v", err)
	}
	t.Cleanup(d.Stop)
	return d, sockPath
}

// dialSocket opens a connection to sockPath, sends a registration frame, and
// returns the connection.
func dialSocket(t *testing.T, sockPath string, name string, subscribe []string) net.Conn {
	t.Helper()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial %s: %v", sockPath, err)
	}
	t.Cleanup(func() { conn.Close() })

	reg := map[string]any{"name": name, "subscribe": subscribe}
	b, _ := json.Marshal(reg)
	b = append(b, '\n')
	if _, err := conn.Write(b); err != nil {
		t.Fatalf("write registration: %v", err)
	}

	return conn
}

// readEvent waits up to 2 seconds for a newline-terminated JSON event frame.
// A read deadline is set on the connection before scanning so the goroutine
// unblocks when the connection is closed by test cleanup (preventing races).
func readEvent(t *testing.T, conn net.Conn) busd.Event {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck

	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1)
	for {
		_, err := conn.Read(tmp)
		if err != nil {
			t.Fatalf("readEvent: read error: %v", err)
		}
		if tmp[0] == '\n' {
			break
		}
		buf = append(buf, tmp[0])
	}

	conn.SetReadDeadline(time.Time{}) //nolint:errcheck

	var ev busd.Event
	if err := json.Unmarshal(buf, &ev); err != nil {
		t.Fatalf("readEvent: unmarshal error: %v", err)
	}
	return ev
}

// waitFor polls cond every 2ms until it returns true or timeout elapses.
func waitFor(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

// ---- tests ------------------------------------------------------------------

// TestSocketPath_XDG verifies that SocketPath uses $XDG_RUNTIME_DIR when set.
func TestSocketPath_XDG(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	got, err := busd.SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	want := "/run/user/1000/orcai/bus.sock"
	if got != want {
		t.Errorf("SocketPath() = %q; want %q", got, want)
	}
}

// TestSocketPath_Fallback verifies the os.UserCacheDir() fallback.
func TestSocketPath_Fallback(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	got, err := busd.SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "/orcai/bus.sock") {
		t.Errorf("SocketPath() fallback = %q; expected suffix /orcai/bus.sock", got)
	}
	// Must not reference /run when XDG_RUNTIME_DIR is unset.
	if strings.Contains(got, "/run/user") {
		t.Errorf("SocketPath() fallback = %q; should not use XDG path", got)
	}
}

// TestPubSub_MatchingSubscription verifies a client receives an event it subscribed to.
func TestPubSub_MatchingSubscription(t *testing.T) {
	d, sockPath := startDaemon(t)

	conn := dialSocket(t, sockPath, "weather", []string{"theme.changed"})

	// Wait until the daemon has registered the client before publishing.
	waitFor(t, func() bool { return d.ClientCount() > 0 }, 500*time.Millisecond)

	if err := d.Publish("theme.changed", map[string]string{"theme": "dracula"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	ev := readEvent(t, conn)
	if ev.Event != "theme.changed" {
		t.Errorf("event.Event = %q; want \"theme.changed\"", ev.Event)
	}
}

// TestPubSub_WildcardSubscription verifies wildcard patterns are routed correctly.
func TestPubSub_WildcardSubscription(t *testing.T) {
	d, sockPath := startDaemon(t)

	conn := dialSocket(t, sockPath, "sessionwatcher", []string{"session.*"})

	waitFor(t, func() bool { return d.ClientCount() > 0 }, 500*time.Millisecond)

	if err := d.Publish("session.started", map[string]string{"id": "abc"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	ev := readEvent(t, conn)
	if ev.Event != "session.started" {
		t.Errorf("event.Event = %q; want \"session.started\"", ev.Event)
	}
}

// TestPubSub_NonMatchingSubscription verifies a client does NOT receive events
// it didn't subscribe to.
func TestPubSub_NonMatchingSubscription(t *testing.T) {
	d, sockPath := startDaemon(t)

	conn := dialSocket(t, sockPath, "weather", []string{"theme.changed"})
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)) //nolint:errcheck

	waitFor(t, func() bool { return d.ClientCount() > 0 }, 500*time.Millisecond)

	// Publish an event the client is NOT subscribed to.
	if err := d.Publish("session.started", nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Expect no data within the deadline.
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if n > 0 {
		t.Errorf("client received %d unexpected bytes: %s", n, buf[:n])
	}
	if err == nil {
		t.Error("expected deadline/EOF error, got nil")
	}
}

// TestPubSub_DisconnectedClientPruned verifies that a disconnected client is
// pruned silently on the next Publish (no panic).
func TestPubSub_DisconnectedClientPruned(t *testing.T) {
	d, sockPath := startDaemon(t)

	conn := dialSocket(t, sockPath, "quitter", []string{"theme.changed"})

	waitFor(t, func() bool { return d.ClientCount() > 0 }, 500*time.Millisecond)

	// Disconnect the client abruptly.
	conn.Close()

	// Wait for the daemon to detect the closure (client count back to zero).
	waitFor(t, func() bool { return d.ClientCount() == 0 }, 500*time.Millisecond)

	// Publishing must not panic and should succeed (client pruned silently).
	if err := d.Publish("theme.changed", nil); err != nil {
		t.Errorf("Publish after disconnect: %v", err)
	}
}

// TestStop_RemovesSocketFile verifies that Stop() removes the socket file.
func TestStop_RemovesSocketFile(t *testing.T) {
	sockPath := tempSockPath(t)
	d := busd.New()
	if err := d.StartAt(sockPath); err != nil {
		t.Fatalf("StartAt: %v", err)
	}

	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("socket file should exist before Stop: %v", err)
	}

	d.Stop()

	if _, err := os.Stat(sockPath); err == nil {
		t.Error("socket file should be removed after Stop()")
	}
}

// TestPublishEvent_DeliveredToSubscriber verifies that PublishEvent (client-side
// publish) delivers the event to a connected subscriber. This specifically tests
// the scanner-reuse fix: PublishEvent writes both the registration frame and the
// publish frame in one burst; without reusing the scanner the publish frame is
// lost because the first scanner already buffered it.
func TestPublishEvent_DeliveredToSubscriber(t *testing.T) {
	_, sockPath := startDaemon(t)

	// Connect a subscriber before publishing.
	sub := dialSocket(t, sockPath, "listener", []string{"theme.changed"})

	// Wait for subscriber to be registered.
	waitFor(t, func() bool {
		// We can't use ClientCount here since we don't have d in scope.
		// Instead just give the server a moment to process the registration.
		return true
	}, 50*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	// Publish via the client-side PublishEvent (not Daemon.Publish).
	if err := busd.PublishEvent(sockPath, "theme.changed", map[string]string{"name": "dracula"}); err != nil {
		t.Fatalf("PublishEvent: %v", err)
	}

	ev := readEvent(t, sub)
	if ev.Event != "theme.changed" {
		t.Errorf("event.Event = %q; want \"theme.changed\"", ev.Event)
	}
}

// TestPublishEvent_NoDaemon verifies graceful degradation when the daemon is absent.
func TestPublishEvent_NoDaemon(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "noexist.sock")
	if err := busd.PublishEvent(sockPath, "theme.changed", nil); err != nil {
		t.Errorf("PublishEvent with no daemon should return nil, got: %v", err)
	}
}
