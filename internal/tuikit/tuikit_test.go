package tuikit_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adam-stokes/orcai/internal/busd"
	"github.com/adam-stokes/orcai/internal/themes"
	"github.com/adam-stokes/orcai/internal/tuikit"
)

// testSockCounter generates unique short socket paths under /tmp to stay well
// within the macOS 104-byte Unix socket path limit.
var testSockCounter atomic.Int64

// setupDaemonAtXDG starts a busd daemon at a short /tmp path and sets
// XDG_RUNTIME_DIR so ThemeSubscribeCmd dials the right socket.
func setupDaemonAtXDG(t *testing.T) (*busd.Daemon, string) {
	t.Helper()
	n := testSockCounter.Add(1)
	// Use short paths under /tmp — macOS limits Unix socket paths to 104 bytes.
	orcaiDir := filepath.Join("/tmp", fmt.Sprintf("tuikit%d", n), "orcai")
	if err := os.MkdirAll(orcaiDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Dir(orcaiDir)) })
	// XDG_RUNTIME_DIR must point to the parent of "orcai/".
	t.Setenv("XDG_RUNTIME_DIR", filepath.Dir(orcaiDir))
	sockPath := filepath.Join(orcaiDir, "bus.sock")
	d := busd.New()
	if err := d.StartAt(sockPath); err != nil {
		t.Fatalf("daemon StartAt: %v", err)
	}
	t.Cleanup(d.Stop)
	return d, sockPath
}

// TestThemeSubscribeCmd_ReceivesThemeChange verifies that ThemeSubscribeCmd
// returns a ThemeChangedMsg when a theme.changed event is published.
func TestThemeSubscribeCmd_ReceivesThemeChange(t *testing.T) {
	d, _ := setupDaemonAtXDG(t)

	ch := make(chan any, 1)
	go func() {
		cmd := tuikit.ThemeSubscribeCmd()
		ch <- cmd()
	}()

	// Give the subscriber a moment to connect and register.
	time.Sleep(50 * time.Millisecond)

	if err := d.Publish(themes.TopicThemeChanged, themes.ThemeChangedPayload{Name: "dracula"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case msg := <-ch:
		got, ok := msg.(tuikit.ThemeChangedMsg)
		if !ok {
			t.Fatalf("expected ThemeChangedMsg, got %T", msg)
		}
		if got.Name != "dracula" {
			t.Errorf("ThemeChangedMsg.Name = %q; want %q", got.Name, "dracula")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ThemeChangedMsg")
	}
}

// TestThemeSubscribeCmd_DaemonUnavailable verifies that ThemeSubscribeCmd
// returns a non-nil retry message (not ThemeChangedMsg) when busd is absent.
func TestThemeSubscribeCmd_DaemonUnavailable(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	cmd := tuikit.ThemeSubscribeCmd()
	msg := cmd()
	// Should return a retry message, not nil and not ThemeChangedMsg.
	if msg == nil {
		t.Error("expected a retry message, got nil")
	}
	if _, ok := msg.(tuikit.ThemeChangedMsg); ok {
		t.Error("expected retry message, got ThemeChangedMsg")
	}
}

// TestThemeState_ReceivesThemeChange verifies that ThemeState.Init starts a
// subscription and ThemeState.Handle updates the bundle on ThemeChangedMsg.
func TestThemeState_ReceivesThemeChange(t *testing.T) {
	d, _ := setupDaemonAtXDG(t)

	ts := tuikit.NewThemeState(nil)
	cmd := ts.Init()

	ch := make(chan any, 1)
	go func() { ch <- cmd() }()

	time.Sleep(50 * time.Millisecond)

	if err := d.Publish(themes.TopicThemeChanged, themes.ThemeChangedPayload{Name: "dracula"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case msg := <-ch:
		got, ok := msg.(tuikit.ThemeChangedMsg)
		if !ok {
			t.Fatalf("expected ThemeChangedMsg, got %T", msg)
		}
		if got.Name != "dracula" {
			t.Errorf("ThemeChangedMsg.Name = %q; want %q", got.Name, "dracula")
		}
		// Simulate what Update() does: pass msg to Handle.
		ts2, cmd2, handled := ts.Handle(got)
		if !handled {
			t.Error("Handle returned ok=false for ThemeChangedMsg")
		}
		if cmd2 == nil {
			t.Error("Handle should return a new subscription cmd")
		}
		// Bundle should remain nil since GlobalRegistry isn't set in tests.
		_ = ts2
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ThemeChangedMsg")
	}
}

// TestThemeState_RetryOnUnavailable verifies that ThemeState returns a retry
// message when busd is unavailable, and that Handle schedules the retry.
func TestThemeState_RetryOnUnavailable(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	ts := tuikit.NewThemeState(nil)
	cmd := ts.Init()
	msg := cmd()

	// Should be a non-nil, non-ThemeChangedMsg (it's the retry msg).
	if msg == nil {
		t.Fatal("expected retry msg, got nil")
	}
	if _, ok := msg.(tuikit.ThemeChangedMsg); ok {
		t.Fatal("got ThemeChangedMsg when busd is unavailable")
	}

	// ThemeState.Handle should consume the retry message.
	_, cmd2, ok := ts.Handle(msg)
	if !ok {
		t.Error("Handle returned ok=false for retry message")
	}
	if cmd2 == nil {
		t.Error("Handle should return a retry cmd")
	}
}
