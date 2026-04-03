package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/executor"
)

// Supervisor subscribes to busd and dispatches registered handlers.
type Supervisor struct {
	cfgDir   string
	cfgPath  string
	handlers []Handler
	busPath  string
	execMgr  *executor.Manager
	mu       sync.RWMutex
	cfg      *SupervisorConfig
	cfgMtime time.Time
	cancel   context.CancelFunc
}

// New creates a new Supervisor. Call Start to begin event dispatch.
func New(cfgDir string, execMgr *executor.Manager) *Supervisor {
	cfg, _ := LoadConfig(cfgDir)
	if cfg == nil {
		cfg = &SupervisorConfig{Roles: make(map[string]RoleConfig)}
	}
	return &Supervisor{
		cfgDir:  cfgDir,
		cfgPath: DefaultConfigPath(cfgDir),
		execMgr: execMgr,
		cfg:     cfg,
	}
}

// RegisterHandler adds a handler. Must be called before Start.
func (s *Supervisor) RegisterHandler(h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers = append(s.handlers, h)
}

// Start dials busd, registers all topic subscriptions, and begins the dispatch
// loop. If busd is not running, Start returns nil (not an error) — the
// supervisor is non-critical and must never crash the caller.
func (s *Supervisor) Start(ctx context.Context) error {
	sockPath, err := busd.SocketPath()
	if err != nil {
		slog.Warn("supervisor: cannot determine busd socket path", "err", err)
		return nil
	}
	s.busPath = sockPath

	conn, err := net.DialTimeout("unix", sockPath, 500*time.Millisecond)
	if err != nil {
		slog.Warn("supervisor: busd not running, supervisor disabled", "err", err)
		return nil
	}

	// Collect all topics from all handlers.
	topicSet := make(map[string]struct{})
	s.mu.RLock()
	for _, h := range s.handlers {
		for _, t := range h.Topics() {
			topicSet[t] = struct{}{}
		}
	}
	s.mu.RUnlock()

	topics := make([]string, 0, len(topicSet))
	for t := range topicSet {
		topics = append(topics, t)
	}

	reg, _ := json.Marshal(map[string]any{
		"name":      "supervisor",
		"subscribe": topics,
	})
	if _, err := conn.Write(append(reg, '\n')); err != nil {
		conn.Close()
		slog.Warn("supervisor: failed to register with busd", "err", err)
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	go s.readLoop(ctx, conn)
	return nil
}

// Stop cancels the supervisor's context, closing the bus connection.
func (s *Supervisor) Stop() {
	s.mu.RLock()
	cancel := s.cancel
	s.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
}

// readLoop reads events from the busd connection and dispatches handlers.
func (s *Supervisor) readLoop(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	// Close the connection when context is cancelled so the scanner unblocks.
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var frame struct {
			Event   string          `json:"event"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			continue
		}

		// Config hot-reload: check mtime.
		s.maybeReloadConfig()

		evt := Event{
			Topic:   frame.Event,
			Payload: []byte(frame.Payload),
		}

		s.mu.RLock()
		cfg := s.cfg
		handlers := s.handlers
		s.mu.RUnlock()

		for _, h := range handlers {
			if matchesAny(h.Topics(), frame.Event) {
				model := ResolveModel(cfg, handlerRole(h))
				go func(handler Handler, e Event, m ResolvedModel) {
					if err := handler.Handle(ctx, e, m); err != nil {
						slog.Warn("supervisor: handler error",
							"handler", handler.Name(),
							"topic", e.Topic,
							"err", err)
					}
				}(h, evt, model)
			}
		}
	}
}

// maybeReloadConfig checks if the config file has been modified since the last
// load and reloads it if so.
func (s *Supervisor) maybeReloadConfig() {
	info, err := os.Stat(s.cfgPath)
	if err != nil {
		return
	}
	s.mu.RLock()
	mtime := s.cfgMtime
	s.mu.RUnlock()

	if info.ModTime().After(mtime) {
		cfg, err := LoadConfig(s.cfgDir)
		if err != nil {
			slog.Warn("supervisor: config reload failed", "err", err)
			return
		}
		s.mu.Lock()
		s.cfg = cfg
		s.cfgMtime = info.ModTime()
		s.mu.Unlock()
		slog.Info("supervisor: config reloaded")
	}
}

// matchesAny returns true if topic matches any of the patterns.
func matchesAny(patterns []string, topic string) bool {
	for _, pat := range patterns {
		if matchTopic(pat, topic) {
			return true
		}
	}
	return false
}

// matchTopic matches a busd topic pattern against a concrete topic.
// Supports wildcard suffix: "session.*" matches "session.foo".
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

// handlerRole attempts to derive a role name from the handler's name.
// Convention: handler names are like "diagnosis", "agent_loop", "router".
func handlerRole(h Handler) string {
	return h.Name()
}
