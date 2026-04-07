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
// up real git/github/mattermost goroutines. It implements the
// production Collector interface (Name + Start(ctx, *esearch.Client)).
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
		builder := func(workspaceID string, _ *Config) []Collector {
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
			func(string, *Config) []Collector { return nil })
		if err := mgr.StartPod(""); err == nil {
			t.Error("want error for empty workspace id")
		}
	})

	t.Run("duplicate start errors", func(t *testing.T) {
		withTempHome(t)
		_ = WriteWorkspaceConfig("ws-dup", "git:\n  repos: []\n")
		fc := newFakeCollector("c")
		mgr := NewPodManager(context.Background(), nil,
			func(string, *Config) []Collector { return []Collector{fc} })
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
			func(string, *Config) []Collector { return nil })
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
			func(string, *Config) []Collector {
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
			func(workspaceID string, _ *Config) []Collector {
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

func TestPodManager_ParentContextCancel(t *testing.T) {
	withTempHome(t)
	_ = WriteWorkspaceConfig("ws-cancel", "git:\n  repos: []\n")

	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	fc := newFakeCollector("cancellable")
	mgr := NewPodManager(parent, nil,
		func(string, *Config) []Collector { return []Collector{fc} })

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
	t.Run("empty config returns just the always-on collectors", func(t *testing.T) {
		out := BuildCollectorsFromConfig("ws", &Config{})
		// directories + pipeline always run, plus claude+copilot
		// because their default Enabled is true (mirrors the global
		// behaviour). The Config{} literal here doesn't go through
		// defaultConfig() so claude/copilot will be false in this
		// path — we expect just directories + pipeline.
		if len(out) != 2 {
			t.Errorf("collectors = %d, want 2 (directories + pipeline)", len(out))
		}
	})

	t.Run("populated config returns expected collectors", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.Git.Repos = []string{"/tmp/repo"}
		cfg.GitHub.Repos = []string{"org/repo"}
		cfg.Mattermost.URL = "https://mm.example.com"
		cfg.Mattermost.Token = "tok"
		// Per the opt-in default rule, claude/copilot are off in
		// defaultConfig — flip them on here so this test exercises
		// the full set of collectors.
		cfg.Claude.Enabled = true
		cfg.Copilot.Enabled = true

		out := BuildCollectorsFromConfig("ws-x", cfg)

		// Expected: git, claude, claude-projects, copilot, github,
		// mattermost, directories, pipeline = 8.
		if len(out) != 8 {
			t.Errorf("collectors = %d, want 8: names=%v", len(out), names(out))
		}

		// Every collector with a WorkspaceID field must hold "ws-x".
		for _, c := range out {
			if got := workspaceIDOf(c); got != "ws-x" {
				t.Errorf("collector %q WorkspaceID = %q, want ws-x", c.Name(), got)
			}
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
	case *GitCollector:
		return v.WorkspaceID
	case *GitHubCollector:
		return v.WorkspaceID
	case *ClaudeCollector:
		return v.WorkspaceID
	case *ClaudeProjectCollector:
		return v.WorkspaceID
	case *CopilotCollector:
		return v.WorkspaceID
	case *MattermostCollector:
		return v.WorkspaceID
	case *DirectoryCollector:
		return v.WorkspaceID
	case *PipelineIndexer:
		return v.WorkspaceID
	default:
		return ""
	}
}
