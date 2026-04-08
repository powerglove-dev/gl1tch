package capability

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// withTempHome redirects os.UserHomeDir to a t.TempDir for the duration of
// the test so WriteWorkspaceConfig / LoadWorkspaceConfig touch a throwaway
// filesystem instead of the real ~/.config/glitch. Every pod manager test
// calls this as its first line because StartPod unconditionally loads the
// workspace's collectors.yaml and would otherwise read the user's real
// config.
func withTempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
}

// daemonCap is a test capability that records its lifecycle so the pod
// manager tests can verify start/stop semantics without spinning up real
// polling loops. It declares TriggerDaemon so the runner calls Invoke once
// and blocks until ctx is cancelled — the same shape the pod manager needs
// to exercise.
type daemonCap struct {
	name    string
	started atomic.Int32
	stopped atomic.Int32
	release chan struct{}
	once    sync.Once
}

func newDaemonCap(name string) *daemonCap {
	return &daemonCap{name: name, release: make(chan struct{})}
}

func (d *daemonCap) Manifest() Manifest {
	return Manifest{
		Name:    d.name,
		Trigger: Trigger{Mode: TriggerDaemon},
	}
}

func (d *daemonCap) Invoke(ctx context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event)
	go func() {
		defer close(ch)
		d.started.Add(1)
		d.once.Do(func() { close(d.release) })
		<-ctx.Done()
		d.stopped.Add(1)
	}()
	return ch, nil
}

func writeBareWorkspaceConfig(t *testing.T, workspaceID string) {
	t.Helper()
	if err := WriteWorkspaceConfig(workspaceID, "git:\n  repos: []\n"); err != nil {
		t.Fatalf("WriteWorkspaceConfig: %v", err)
	}
}

func TestPodManager_StartStop(t *testing.T) {
	withTempHome(t)
	writeBareWorkspaceConfig(t, "ws-1")

	d1 := newDaemonCap("fake1")
	d2 := newDaemonCap("fake2")

	mgr := NewPodManager(context.Background(), nil)
	mgr.SetCapabilityBuilder(func(id string, _ *Config, _ []string) []Capability {
		if id != "ws-1" {
			t.Errorf("builder got %q, want ws-1", id)
		}
		return []Capability{d1, d2}
	})

	if err := mgr.StartPod("ws-1"); err != nil {
		t.Fatalf("StartPod: %v", err)
	}

	<-d1.release
	<-d2.release

	if d1.started.Load() != 1 || d2.started.Load() != 1 {
		t.Errorf("not all started: d1=%d d2=%d",
			d1.started.Load(), d2.started.Load())
	}
	if active := mgr.ActiveWorkspaces(); len(active) != 1 || active[0] != "ws-1" {
		t.Errorf("ActiveWorkspaces = %v, want [ws-1]", active)
	}

	if err := mgr.StopPod("ws-1"); err != nil {
		t.Fatalf("StopPod: %v", err)
	}

	// Give the runner goroutines a moment to settle after cancel.
	deadline := time.After(2 * time.Second)
	for d1.stopped.Load() == 0 || d2.stopped.Load() == 0 {
		select {
		case <-deadline:
			t.Fatalf("capabilities did not stop: d1=%d d2=%d",
				d1.stopped.Load(), d2.stopped.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if len(mgr.ActiveWorkspaces()) != 0 {
		t.Errorf("workspace not removed from active list")
	}
}

func TestPodManager_StartPodRejectsEmptyID(t *testing.T) {
	mgr := NewPodManager(context.Background(), nil)
	if err := mgr.StartPod(""); err == nil {
		t.Error("expected error for empty workspace id")
	}
}

func TestPodManager_DuplicateStart(t *testing.T) {
	withTempHome(t)
	writeBareWorkspaceConfig(t, "ws-dup")

	d := newDaemonCap("one")
	mgr := NewPodManager(context.Background(), nil)
	mgr.SetCapabilityBuilder(func(string, *Config, []string) []Capability {
		return []Capability{d}
	})

	if err := mgr.StartPod("ws-dup"); err != nil {
		t.Fatalf("first start: %v", err)
	}
	<-d.release

	if err := mgr.StartPod("ws-dup"); err == nil {
		t.Error("expected duplicate start to error")
	}

	_ = mgr.StopPod("ws-dup")
}

func TestPodManager_Restart(t *testing.T) {
	withTempHome(t)
	writeBareWorkspaceConfig(t, "ws-r")

	// Restart should produce a fresh capability instance each time, so
	// we build a new one per builder invocation rather than reusing a
	// single fixture across calls (the old instance's release channel
	// would already be closed).
	var calls atomic.Int32
	var current *daemonCap
	var curMu sync.Mutex
	mgr := NewPodManager(context.Background(), nil)
	mgr.SetCapabilityBuilder(func(string, *Config, []string) []Capability {
		calls.Add(1)
		d := newDaemonCap("restartable")
		curMu.Lock()
		current = d
		curMu.Unlock()
		return []Capability{d}
	})

	if err := mgr.StartPod("ws-r"); err != nil {
		t.Fatalf("StartPod: %v", err)
	}
	curMu.Lock()
	first := current
	curMu.Unlock()
	<-first.release

	if err := mgr.RestartPod("ws-r"); err != nil {
		t.Fatalf("RestartPod: %v", err)
	}
	curMu.Lock()
	second := current
	curMu.Unlock()
	<-second.release

	if calls.Load() < 2 {
		t.Errorf("builder called %d times, want >= 2", calls.Load())
	}
	if first == second {
		t.Error("restart reused the same capability instance")
	}

	// Wait for first to have stopped (restart cancels it).
	deadline := time.After(2 * time.Second)
	for first.stopped.Load() == 0 {
		select {
		case <-deadline:
			t.Fatalf("first capability did not stop after restart")
		case <-time.After(10 * time.Millisecond):
		}
	}

	_ = mgr.StopPod("ws-r")
}

func TestPodManager_StopAll(t *testing.T) {
	withTempHome(t)
	writeBareWorkspaceConfig(t, "ws-a")
	writeBareWorkspaceConfig(t, "ws-b")

	aCap := newDaemonCap("a")
	bCap := newDaemonCap("b")

	mgr := NewPodManager(context.Background(), nil)
	mgr.SetCapabilityBuilder(func(id string, _ *Config, _ []string) []Capability {
		switch id {
		case "ws-a":
			return []Capability{aCap}
		case "ws-b":
			return []Capability{bCap}
		}
		return nil
	})

	if err := mgr.StartPod("ws-a"); err != nil {
		t.Fatalf("StartPod a: %v", err)
	}
	if err := mgr.StartPod("ws-b"); err != nil {
		t.Fatalf("StartPod b: %v", err)
	}
	<-aCap.release
	<-bCap.release

	if got := len(mgr.ActiveWorkspaces()); got != 2 {
		t.Errorf("active = %d, want 2", got)
	}

	mgr.StopAll()

	if got := len(mgr.ActiveWorkspaces()); got != 0 {
		t.Errorf("active after StopAll = %d, want 0", got)
	}

	deadline := time.After(2 * time.Second)
	for aCap.stopped.Load() == 0 || bCap.stopped.Load() == 0 {
		select {
		case <-deadline:
			t.Fatalf("not all stopped: a=%d b=%d",
				aCap.stopped.Load(), bCap.stopped.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestPodManager_ParentContextCancelTearsDown(t *testing.T) {
	withTempHome(t)
	writeBareWorkspaceConfig(t, "ws-cancel")

	d := newDaemonCap("cancellable")
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewPodManager(parent, nil)
	mgr.SetCapabilityBuilder(func(string, *Config, []string) []Capability {
		return []Capability{d}
	})

	if err := mgr.StartPod("ws-cancel"); err != nil {
		t.Fatalf("StartPod: %v", err)
	}
	<-d.release

	cancel() // parent cancel must propagate to the pod runner

	deadline := time.After(2 * time.Second)
	for d.stopped.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("capability did not stop within 2s of parent cancel")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestPodManager_ToolPodStartStop(t *testing.T) {
	// LoadConfig (used by StartToolPod) reads from ~/.config/glitch —
	// redirect HOME so it reads our temp dir instead.
	withTempHome(t)

	// Seed an empty config file so LoadConfig returns defaults instead
	// of surfacing the missing-file path error.
	home, _ := os.UserHomeDir()
	_ = os.MkdirAll(home+"/.config/glitch", 0o755)
	_ = os.WriteFile(home+"/.config/glitch/observer.yaml", []byte(""), 0o644)

	d := newDaemonCap("toolcap")
	mgr := NewPodManager(context.Background(), nil)
	mgr.SetToolCapabilityBuilder(func(id string, _ *Config, _ []string) []Capability {
		if id != WorkspaceIDTools {
			t.Errorf("tool builder got %q, want %q", id, WorkspaceIDTools)
		}
		return []Capability{d}
	})

	if err := mgr.StartToolPod(); err != nil {
		t.Fatalf("StartToolPod: %v", err)
	}
	<-d.release

	if err := mgr.StartToolPod(); err == nil {
		t.Error("expected second StartToolPod to error")
	}

	if err := mgr.StopToolPod(); err != nil {
		t.Fatalf("StopToolPod: %v", err)
	}

	// Idempotent: second stop is fine.
	if err := mgr.StopToolPod(); err != nil {
		t.Fatalf("second StopToolPod: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for d.stopped.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("tool capability did not stop")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestPodManager_StopUnknownIsNoOp(t *testing.T) {
	mgr := NewPodManager(context.Background(), nil)
	if err := mgr.StopPod("never-started"); err != nil {
		t.Errorf("StopPod on unknown workspace returned %v, want nil", err)
	}
	if err := mgr.StopPod(""); err != nil {
		t.Errorf("StopPod on empty id returned %v, want nil", err)
	}
}
