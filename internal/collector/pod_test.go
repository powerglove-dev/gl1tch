package collector

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// fakeCollector is a test double that records its lifecycle so the
// pod manager tests can verify start/stop semantics without spinning
// up real git/github goroutines. It implements the production
// Collector interface (Name + Start(ctx, *esearch.Client)).
type fakeCollector struct {
	name    string
	started atomic.Int32
	stopped atomic.Int32
	// release is closed by Start so tests can wait for the goroutine
	// to actually run before asserting on counters.
	release chan struct{}
	once    sync.Once
}

func newFakeCollector(name string) *fakeCollector {
	return &fakeCollector{name: name, release: make(chan struct{})}
}

func (f *fakeCollector) Name() string { return f.name }

func (f *fakeCollector) Start(ctx context.Context, _ *esearch.Client) error {
	f.started.Add(1)
	f.once.Do(func() { close(f.release) })
	<-ctx.Done()
	f.stopped.Add(1)
	return ctx.Err()
}

func TestPodManager_StartStop(t *testing.T) {
	t.Run("starts each collector in its own goroutine", func(t *testing.T) {
		withTempHome(t)
		_ = WriteWorkspaceConfig("ws-1", "git:\n  repos: []\n")

		fc1 := newFakeCollector("c1")
		fc2 := newFakeCollector("c2")
		builder := func(workspaceID string, _ *Config, _ []string) []Collector {
			if workspaceID != "ws-1" {
				t.Errorf("builder got %q, want ws-1", workspaceID)
			}
			return []Collector{fc1, fc2}
		}
		mgr := NewPodManager(context.Background(), nil, builder)

		if err := mgr.StartPod("ws-1"); err != nil {
			t.Fatalf("StartPod: %v", err)
		}
		<-fc1.release
		<-fc2.release

		if fc1.started.Load() != 1 || fc2.started.Load() != 1 {
			t.Errorf("collectors not all started: c1=%d c2=%d",
				fc1.started.Load(), fc2.started.Load())
		}
		active := mgr.ActiveWorkspaces()
		if len(active) != 1 || active[0] != "ws-1" {
			t.Errorf("ActiveWorkspaces = %v, want [ws-1]", active)
		}

		if err := mgr.StopPod("ws-1"); err != nil {
			t.Fatalf("StopPod: %v", err)
		}
		if fc1.stopped.Load() != 1 || fc2.stopped.Load() != 1 {
			t.Errorf("collectors not all stopped after StopPod: c1=%d c2=%d",
				fc1.stopped.Load(), fc2.stopped.Load())
		}
		if len(mgr.ActiveWorkspaces()) != 0 {
			t.Errorf("workspace not removed from active list")
		}
	})

	t.Run("rejects empty workspace id", func(t *testing.T) {
		mgr := NewPodManager(context.Background(), nil,
			func(string, *Config, []string) []Collector { return nil })
		if err := mgr.StartPod(""); err == nil {
			t.Error("want error for empty workspace id")
		}
	})

	t.Run("duplicate start errors", func(t *testing.T) {
		withTempHome(t)
		_ = WriteWorkspaceConfig("ws-dup", "git:\n  repos: []\n")
		fc := newFakeCollector("c")
		mgr := NewPodManager(context.Background(), nil,
			func(string, *Config, []string) []Collector { return []Collector{fc} })
		if err := mgr.StartPod("ws-dup"); err != nil {
			t.Fatalf("first start: %v", err)
		}
		<-fc.release
		if err := mgr.StartPod("ws-dup"); err == nil {
			t.Error("want error for duplicate start")
		}
		_ = mgr.StopPod("ws-dup")
	})

	t.Run("stopping a missing pod is a no-op", func(t *testing.T) {
		mgr := NewPodManager(context.Background(), nil,
			func(string, *Config, []string) []Collector { return nil })
		if err := mgr.StopPod("never-started"); err != nil {
			t.Errorf("stop missing should be no-op, got: %v", err)
		}
	})

	t.Run("restart cycles a pod", func(t *testing.T) {
		withTempHome(t)
		_ = WriteWorkspaceConfig("ws-restart", "git:\n  repos: []\n")

		var builds atomic.Int32
		var mu sync.Mutex
		var fakes []*fakeCollector
		mgr := NewPodManager(context.Background(), nil,
			func(string, *Config, []string) []Collector {
				builds.Add(1)
				f := newFakeCollector("restartable")
				mu.Lock()
				fakes = append(fakes, f)
				mu.Unlock()
				return []Collector{f}
			})

		if err := mgr.StartPod("ws-restart"); err != nil {
			t.Fatalf("start: %v", err)
		}
		mu.Lock()
		first := fakes[0]
		mu.Unlock()
		<-first.release

		if err := mgr.RestartPod("ws-restart"); err != nil {
			t.Fatalf("restart: %v", err)
		}
		if builds.Load() != 2 {
			t.Errorf("builds = %d, want 2", builds.Load())
		}
		mu.Lock()
		second := fakes[1]
		mu.Unlock()
		<-second.release
		if first.stopped.Load() != 1 {
			t.Errorf("first instance not stopped during restart")
		}
		_ = mgr.StopPod("ws-restart")
	})

	t.Run("StopAll tears down every pod", func(t *testing.T) {
		withTempHome(t)
		for _, id := range []string{"ws-1", "ws-2", "ws-3"} {
			_ = WriteWorkspaceConfig(id, "git:\n  repos: []\n")
		}

		var fakes []*fakeCollector
		var mu sync.Mutex
		mgr := NewPodManager(context.Background(), nil,
			func(workspaceID string, _ *Config, _ []string) []Collector {
				f := newFakeCollector(workspaceID + "-c")
				mu.Lock()
				fakes = append(fakes, f)
				mu.Unlock()
				return []Collector{f}
			})

		for _, id := range []string{"ws-1", "ws-2", "ws-3"} {
			if err := mgr.StartPod(id); err != nil {
				t.Fatalf("start %s: %v", id, err)
			}
		}

		mu.Lock()
		all := append([]*fakeCollector{}, fakes...)
		mu.Unlock()
		for _, f := range all {
			<-f.release
		}

		mgr.StopAll()

		if got := len(mgr.ActiveWorkspaces()); got != 0 {
			t.Errorf("ActiveWorkspaces after StopAll = %d, want 0", got)
		}
		for _, f := range all {
			if f.stopped.Load() != 1 {
				t.Errorf("collector %s not stopped: %d", f.name, f.stopped.Load())
			}
		}
	})
}

func TestPodManager_ConcurrentRestart(t *testing.T) {
	// Two concurrent RestartPod calls for the same workspace must
	// serialize via the per-workspace lock so the end state is
	// exactly one set of running collectors. Without the lock the
	// second restart would race with the first's Start and either
	// drop one operation or leak goroutines.
	withTempHome(t)
	_ = WriteWorkspaceConfig("ws-race", "git:\n  repos: []\n")

	var totalBuilds atomic.Int32
	var mu sync.Mutex
	var fakes []*fakeCollector

	mgr := NewPodManager(context.Background(), nil, func(string, *Config, []string) []Collector {
		totalBuilds.Add(1)
		f := newFakeCollector("racer")
		mu.Lock()
		fakes = append(fakes, f)
		mu.Unlock()
		return []Collector{f}
	})

	// Initial pod so the restarts have something to stop.
	if err := mgr.StartPod("ws-race"); err != nil {
		t.Fatalf("initial start: %v", err)
	}

	// Fire 5 concurrent restarts. Each should stop the previous
	// pod and start a fresh one. With the per-workspace lock these
	// serialize, so total builds = 1 (initial) + 5 (each restart)
	// and exactly one pod is running at the end.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := mgr.RestartPod("ws-race"); err != nil {
				t.Errorf("restart: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := totalBuilds.Load(); got != 6 {
		t.Errorf("totalBuilds = %d, want 6 (1 initial + 5 restarts)", got)
	}
	if active := mgr.ActiveWorkspaces(); len(active) != 1 || active[0] != "ws-race" {
		t.Errorf("ActiveWorkspaces = %v, want [ws-race]", active)
	}

	// Every fake except the most recent must be stopped.
	mu.Lock()
	all := append([]*fakeCollector{}, fakes...)
	mu.Unlock()
	if len(all) != 6 {
		t.Fatalf("expected 6 fake collectors, got %d", len(all))
	}
	for i, f := range all[:5] {
		if f.stopped.Load() != 1 {
			t.Errorf("fake %d not stopped: stopped=%d", i, f.stopped.Load())
		}
	}
	// Last one should still be running (no Stop called yet).
	if all[5].stopped.Load() != 0 {
		t.Errorf("most recent fake should still be running, got stopped=%d", all[5].stopped.Load())
	}

	_ = mgr.StopPod("ws-race")
}

func TestPodManager_ParentContextCancel(t *testing.T) {
	withTempHome(t)
	_ = WriteWorkspaceConfig("ws-cancel", "git:\n  repos: []\n")

	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	fc := newFakeCollector("cancellable")
	mgr := NewPodManager(parent, nil,
		func(string, *Config, []string) []Collector { return []Collector{fc} })

	if err := mgr.StartPod("ws-cancel"); err != nil {
		t.Fatalf("StartPod: %v", err)
	}
	<-fc.release

	// Cancelling the parent context tears down the collector even
	// without an explicit StopPod call.
	cancel()

	deadline := time.After(2 * time.Second)
	for fc.stopped.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("collector did not stop within 2s of parent cancel")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestBuildCollectorsFromConfig(t *testing.T) {
	t.Run("empty config + no dirs returns just pipeline", func(t *testing.T) {
		out := BuildCollectorsFromConfig("ws", &Config{}, nil)
		// With no dirs the unified WorkspaceCollector is not spawned
		// (no work to do), claude/copilot are off by default in a
		// bare Config literal, so the only thing left is the
		// always-on PipelineIndexer.
		if len(out) != 1 {
			t.Errorf("collectors = %d, want 1 (pipeline only): names=%v", len(out), names(out))
		}
	})

	t.Run("populated config + dirs returns workspace + claude + pipeline", func(t *testing.T) {
		cfg := defaultConfig()
		// Per the opt-in default rule, claude is off in
		// defaultConfig — flip it on here so the test exercises the
		// full per-workspace set.
		cfg.Claude.Enabled = true
		cfg.Copilot.Enabled = true

		out := BuildCollectorsFromConfig("ws-x", cfg, []string{"/tmp/repo"})

		// Expected: workspace (unified directories+git+github),
		// claude, claude-projects, pipeline = 4. Copilot is
		// intentionally excluded from per-workspace pods because its
		// data source is global / shared across workspaces; it lives
		// in the tool pod path (BuildToolCollectorsFromConfig).
		if len(out) != 4 {
			t.Errorf("collectors = %d, want 4: names=%v", len(out), names(out))
		}

		// Every collector with a WorkspaceID field must hold "ws-x".
		for _, c := range out {
			if got := workspaceIDOf(c); got != "ws-x" {
				t.Errorf("collector %q WorkspaceID = %q, want ws-x", c.Name(), got)
			}
		}
	})
}

// TestBuildToolCollectorsFromConfig locks down the tool-pod
// builder: copilot is included when enabled, and every collector
// returned must be stamped with WorkspaceIDTools (not the empty
// string and not any per-workspace id) so the OR-include query in
// QueryCollectorActivityScoped can find them.
func TestBuildToolCollectorsFromConfig(t *testing.T) {
	t.Run("nil config returns empty slice", func(t *testing.T) {
		out := BuildToolCollectorsFromConfig(nil)
		if len(out) != 0 {
			t.Errorf("collectors = %d, want 0", len(out))
		}
	})

	t.Run("empty config skips copilot", func(t *testing.T) {
		out := BuildToolCollectorsFromConfig(&Config{})
		if len(out) != 0 {
			t.Errorf("collectors = %d, want 0: names=%v", len(out), names(out))
		}
	})

	t.Run("copilot enabled", func(t *testing.T) {
		cfg := &Config{}
		cfg.Copilot.Enabled = true
		out := BuildToolCollectorsFromConfig(cfg)
		if len(out) != 1 {
			t.Fatalf("collectors = %d, want 1: names=%v", len(out), names(out))
		}
		if got := workspaceIDOf(out[0]); got != WorkspaceIDTools {
			t.Errorf("copilot WorkspaceID = %q, want %q",
				got, WorkspaceIDTools)
		}
	})
}

func names(cs []Collector) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name()
	}
	return out
}

// workspaceIDOf reads the WorkspaceID field off any concrete
// collector type. Each one has it as a public string field.
func workspaceIDOf(c Collector) string {
	switch v := c.(type) {
	case *WorkspaceCollector:
		return v.WorkspaceID
	case *ClaudeCollector:
		return v.WorkspaceID
	case *ClaudeProjectCollector:
		return v.WorkspaceID
	case *CopilotCollector:
		return v.WorkspaceID
	case *PipelineIndexer:
		return v.WorkspaceID
	default:
		return ""
	}
}
