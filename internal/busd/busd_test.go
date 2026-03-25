package busd_test

import (
	"bufio"
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
// returns the connection and a line scanner ready for reading events.
func dialSocket(t *testing.T, sockPath string, name string, subscribe []string) (net.Conn, *bufio.Scanner) {
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

	sc := bufio.NewScanner(conn)
	return conn, sc
}

// readEvent waits up to timeout for a newline-terminated JSON event frame.
func readEvent(t *testing.T, sc *bufio.Scanner, timeout time.Duration) map[string]any {
	t.Helper()
	done := make(chan map[string]any, 1)
	go func() {
		if sc.Scan() {
			var m map[string]any
			_ = json.Unmarshal(sc.Bytes(), &m)
			done <- m
		} else {
			done <- nil
		}
	}()
	select {
	case m := <-done:
		return m
	case <-time.After(timeout):
		t.Fatal("timed out waiting for event")
		return nil
	}
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

	_, sc := dialSocket(t, sockPath, "weather", []string{"theme.changed"})

	// Brief pause to let the daemon register the client.
	time.Sleep(20 * time.Millisecond)

	if err := d.Publish("theme.changed", map[string]string{"theme": "dracula"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	evt := readEvent(t, sc, 2*time.Second)
	if evt == nil {
		t.Fatal("expected event, got nil")
	}
	if got, ok := evt["event"].(string); !ok || got != "theme.changed" {
		t.Errorf("event[\"event\"] = %v; want \"theme.changed\"", evt["event"])
	}
}

// TestPubSub_WildcardSubscription verifies wildcard patterns are routed correctly.
func TestPubSub_WildcardSubscription(t *testing.T) {
	d, sockPath := startDaemon(t)

	_, sc := dialSocket(t, sockPath, "sessionwatcher", []string{"session.*"})

	time.Sleep(20 * time.Millisecond)

	if err := d.Publish("session.started", map[string]string{"id": "abc"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	evt := readEvent(t, sc, 2*time.Second)
	if evt == nil {
		t.Fatal("expected wildcard event, got nil")
	}
	if got := evt["event"].(string); got != "session.started" {
		t.Errorf("event[\"event\"] = %q; want \"session.started\"", got)
	}
}

// TestPubSub_NonMatchingSubscription verifies a client does NOT receive events
// it didn't subscribe to.
func TestPubSub_NonMatchingSubscription(t *testing.T) {
	d, sockPath := startDaemon(t)

	conn, _ := dialSocket(t, sockPath, "weather", []string{"theme.changed"})
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)) //nolint:errcheck

	time.Sleep(20 * time.Millisecond)

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

	conn, _ := dialSocket(t, sockPath, "quitter", []string{"theme.changed"})

	time.Sleep(20 * time.Millisecond)

	// Disconnect the client abruptly.
	conn.Close()

	// Give the daemon a moment to detect the closure.
	time.Sleep(20 * time.Millisecond)

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
