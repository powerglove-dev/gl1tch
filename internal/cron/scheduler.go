package cron

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/activity"
	"github.com/8op-org/gl1tch/internal/busd/topics"
	"github.com/charmbracelet/log"
	"github.com/fsnotify/fsnotify"
	robfigcron "github.com/robfig/cron/v3"
)

// EventPublisher publishes cron lifecycle events to the event bus.
// Implementations must be safe to call concurrently.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// NoopPublisher discards all events.
type NoopPublisher struct{}

func (NoopPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }

// Option is a functional option for configuring a Scheduler.
type Option func(*Scheduler)

// WithPublisher sets the EventPublisher used to emit cron lifecycle events.
func WithPublisher(p EventPublisher) Option {
	return func(s *Scheduler) { s.publisher = p }
}

// Scheduler wraps robfig/cron and adds hot-reload via fsnotify and optional
// event publishing for run lifecycle events.
type Scheduler struct {
	c         *robfigcron.Cron
	logger    *log.Logger
	publisher EventPublisher
	mu        sync.Mutex
	entries   []Entry
	cronIDs   []robfigcron.EntryID
	done      chan struct{}
}

// New creates a new Scheduler. The logger may be nil; the scheduler will
// operate without logging in that case. Pass functional options to configure
// additional behaviour such as event publishing.
func New(logger *log.Logger, opts ...Option) *Scheduler {
	sc := &Scheduler{
		c:         robfigcron.New(),
		logger:    logger,
		publisher: NoopPublisher{},
		done:      make(chan struct{}),
	}
	for _, o := range opts {
		o(sc)
	}
	return sc
}

// Start begins the cron daemon. It loads the config, registers all entries,
// starts the underlying cron runner, and launches a goroutine that watches
// ~/.config/glitch/cron.yaml for changes and hot-reloads on write/create.
func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.loadAndRegister(); err != nil {
		return err
	}
	s.c.Start()
	go s.watchConfig(ctx)
	return nil
}

// Stop halts the cron daemon and signals the fsnotify watcher to exit.
func (s *Scheduler) Stop() {
	s.c.Stop()
	select {
	case <-s.done:
		// already closed
	default:
		close(s.done)
	}
}

// Entries returns a copy of the currently registered schedule entries.
func (s *Scheduler) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]Entry, len(s.entries))
	copy(cp, s.entries)
	return cp
}

// RunNow executes entry immediately in a goroutine, outside its schedule.
// Output is written to the same logger as scheduled runs.
func (s *Scheduler) RunNow(entry Entry) {
	go s.runEntry(entry)
}

// loadAndRegister reads the config file and registers all entries with the
// underlying cron instance. It replaces any previously registered entries.
func (s *Scheduler) loadAndRegister() error {
	entries, err := LoadConfig()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove previously registered entries.
	for _, id := range s.cronIDs {
		s.c.Remove(id)
	}
	s.cronIDs = s.cronIDs[:0]
	s.entries = entries

	registered := 0
	for _, e := range entries {
		entry := e // capture loop var
		id, err := s.c.AddFunc(entry.Schedule, func() {
			s.runEntry(entry)
		})
		if err != nil {
			s.logError("cron: invalid schedule for entry, skipping",
				"name", entry.Name, "schedule", entry.Schedule, "err", err)
			continue
		}
		s.cronIDs = append(s.cronIDs, id)
		registered++
	}

	s.logInfo("cron: entries loaded", "total", len(entries), "registered", registered)
	return nil
}

// reload re-reads the config file and re-registers all entries. Called on
// fsnotify events after a debounce delay.
func (s *Scheduler) reload() {
	s.logInfo("cron: reloading config")
	if err := s.loadAndRegister(); err != nil {
		s.logError("cron: reload failed", "err", err)
	}
}

// runEntry executes a single scheduled entry as a subprocess, publishing
// cron.job.started before spawn and cron.job.completed after exit.
func (s *Scheduler) runEntry(entry Entry) {
	// Determine subprocess args.
	var args []string
	switch entry.Kind {
	case "pipeline", "agent":
		args = []string{"pipeline", "run", entry.Target}
	default:
		s.logError("cron: unknown entry kind", "name", entry.Name, "kind", entry.Kind)
		return
	}

	// Build context with optional timeout.
	ctx := context.Background()
	var cancel context.CancelFunc
	if entry.Timeout != "" {
		d, err := time.ParseDuration(entry.Timeout)
		if err != nil {
			s.logError("cron: invalid timeout, running without deadline",
				"name", entry.Name, "timeout", entry.Timeout, "err", err)
		} else {
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}
	}

	// Publish cron.job.started before spawning subprocess.
	startedAt := time.Now()
	startPayload, _ := json.Marshal(map[string]any{
		"job":          entry.Name,
		"target":       entry.Target,
		"schedule":     entry.Schedule,
		"triggered_at": startedAt.Format(time.RFC3339),
	})
	_ = s.publisher.Publish(context.Background(), topics.CronJobStarted, startPayload)
	_ = activity.AppendEvent(activity.DefaultPath(), activity.Now(
		"schedule_fired", entry.Name, entry.Target, "scheduled",
	))

	// Resolve the glitch binary (same binary as current process).
	self, err := os.Executable()
	if err != nil {
		self = "glitch"
	}

	cmd := exec.CommandContext(ctx, self, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	runErr := cmd.Run()

	exitStatus := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitStatus = ee.ExitCode()
		} else {
			exitStatus = 1
		}
		s.logError("cron: entry failed", "name", entry.Name, "exit", exitStatus, "err", runErr)
	} else {
		s.logInfo("cron: entry completed", "name", entry.Name)
	}

	// Publish cron.job.completed after subprocess exits.
	durationMs := time.Since(startedAt).Milliseconds()
	donePayload, _ := json.Marshal(map[string]any{
		"job":         entry.Name,
		"target":      entry.Target,
		"exit_status": exitStatus,
		"duration_ms": durationMs,
		"finished_at": time.Now().Format(time.RFC3339),
	})
	_ = s.publisher.Publish(context.Background(), topics.CronJobCompleted, donePayload)
}

// watchConfig uses fsnotify to watch the cron config file for changes,
// debouncing rapid events by 500 ms before calling reload.
func (s *Scheduler) watchConfig(ctx context.Context) {
	home, err := os.UserHomeDir()
	if err != nil {
		s.logError("cron: watchConfig: cannot determine home dir", "err", err)
		return
	}
	configFile := filepath.Join(home, ".config", "glitch", "cron.yaml")
	configDir := filepath.Dir(configFile)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		s.logError("cron: fsnotify.NewWatcher failed", "err", err)
		return
	}
	defer watcher.Close()

	// Watch the directory so we catch file creation events too.
	if err := watcher.Add(configDir); err != nil {
		s.logError("cron: watcher.Add failed", "dir", configDir, "err", err)
		return
	}

	var debounce *time.Timer

	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Only react to events on the cron.yaml file itself.
			if filepath.Clean(event.Name) != filepath.Clean(configFile) {
				continue
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(500*time.Millisecond, func() {
					s.reload()
				})
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			s.logError("cron: watcher error", "err", err)
		}
	}
}

// logInfo logs at Info level when a logger is configured.
func (s *Scheduler) logInfo(msg string, kv ...any) {
	if s.logger != nil {
		s.logger.Info(msg, kv...)
	}
}

// logError logs at Error level when a logger is configured.
func (s *Scheduler) logError(msg string, kv ...any) {
	if s.logger != nil {
		s.logger.Error(msg, kv...)
	}
}
