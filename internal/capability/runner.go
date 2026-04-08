package capability

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"
)

// Indexer is the minimal interface the runner needs from esearch.Client. It
// exists so the runner can be tested without a real Elasticsearch and so the
// capability package doesn't have to import esearch directly. *esearch.Client
// satisfies this implicitly via its existing BulkIndex method.
type Indexer interface {
	BulkIndex(ctx context.Context, index string, docs []any) error
}

// AfterInvokeHook is called once per completed invocation, regardless of
// trigger mode. It exists so the bootstrap layer can feed the brain popover
// heartbeat (collector.RecordRun) without coupling the capability package to
// the collector package. err is non-nil only when the runner-level routing
// failed (Invoke error or BulkIndex error); per-event EventError values are
// logged but do not propagate here.
type AfterInvokeHook func(name string, dur time.Duration, indexed int, err error)

// Runner drives capabilities. It schedules every interval-trigger capability
// in its registry on a background goroutine and routes events to the indexer
// (Doc events) or to the invocation caller (Stream events). On-demand
// capabilities are not started by Start — they wait for explicit Invoke calls
// from the assistant or the user.
//
// One Runner per process. The same Runner handles every capability regardless
// of trigger mode, which is the whole point of unifying the two systems.
type Runner struct {
	reg          *Registry
	indexer      Indexer
	defaultIndex string

	mu          sync.Mutex
	cancels     []context.CancelFunc
	wg          sync.WaitGroup
	afterInvoke AfterInvokeHook
}

// NewRunner constructs a Runner that pulls capabilities from reg and routes
// Doc events to indexer. Pass nil indexer to disable Doc routing entirely
// (useful for tests and for headless on-demand-only setups).
func NewRunner(reg *Registry, indexer Indexer) *Runner {
	return &Runner{
		reg:          reg,
		indexer:      indexer,
		defaultIndex: "glitch-events",
	}
}

// SetDefaultIndex overrides the index used when a Doc event has no explicit
// index and the capability's Invocation.Index is also empty.
func (r *Runner) SetDefaultIndex(idx string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultIndex = idx
}

// SetAfterInvoke installs a hook called after every invocation completes.
// Only one hook is supported; subsequent calls replace the previous hook.
func (r *Runner) SetAfterInvoke(h AfterInvokeHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.afterInvoke = h
}

// Start spawns a background goroutine for every scheduled capability in the
// registry. Interval-trigger capabilities tick on the manifest's cadence;
// daemon-trigger capabilities are invoked once and run until ctx is
// cancelled. On-demand capabilities are not started here — they wait for
// explicit Invoke calls.
//
// Capabilities registered after Start is called are not picked up — call
// Start again or use a wrapper that re-scans. In production the registry is
// built once at boot.
//
// The passed context is the parent for every scheduled invocation;
// cancelling it stops every loop. Stop also works.
func (r *Runner) Start(ctx context.Context) {
	for _, c := range r.reg.List() {
		m := c.Manifest()
		switch m.Trigger.Mode {
		case TriggerInterval:
			childCtx, cancel := context.WithCancel(ctx)
			r.mu.Lock()
			r.cancels = append(r.cancels, cancel)
			r.mu.Unlock()
			r.wg.Add(1)
			go func(cap Capability) {
				defer r.wg.Done()
				r.runInterval(childCtx, cap)
			}(c)
		case TriggerDaemon:
			childCtx, cancel := context.WithCancel(ctx)
			r.mu.Lock()
			r.cancels = append(r.cancels, cancel)
			r.mu.Unlock()
			r.wg.Add(1)
			go func(cap Capability) {
				defer r.wg.Done()
				r.runDaemon(childCtx, cap)
			}(c)
		}
	}
}

// Stop cancels every scheduled interval goroutine and waits for them to exit.
// Safe to call multiple times.
func (r *Runner) Stop() {
	r.mu.Lock()
	cancels := r.cancels
	r.cancels = nil
	r.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	r.wg.Wait()
}

// Invoke calls a registered capability by name. Stream events are written to
// stream (pass io.Discard to drop them; pass nil to use io.Discard). Doc
// events are routed to the indexer regardless of caller. Returns an error if
// the capability is not found, the capability's Invoke fails, or the bulk
// index fails. Errors emitted as EventError mid-stream are logged but do not
// fail the invocation.
//
// This is the entry point the assistant uses for on-demand capabilities and
// the entry point the interval scheduler uses internally for indexers. One
// path, two callers.
func (r *Runner) Invoke(ctx context.Context, name string, in Input, stream io.Writer) error {
	c, ok := r.reg.Get(name)
	if !ok {
		return &NotFoundError{Name: name}
	}
	return r.runOnce(ctx, c, in, stream)
}

// runDaemon invokes a capability once and waits for it to complete.
// Daemon-mode capabilities own their own internal scheduling — they block
// inside Invoke until ctx is cancelled. The legacy collector wrapper is the
// canonical example. Errors from runOnce are logged; the daemon is not
// restarted on exit (intentional — if a legacy collector dies, the brain
// popover surfaces it via RecordRun and the user gets to investigate).
func (r *Runner) runDaemon(ctx context.Context, c Capability) {
	m := c.Manifest()
	slog.Info("capability: daemon started", "name", m.Name)
	if err := r.runOnce(ctx, c, Input{}, io.Discard); err != nil && ctx.Err() == nil {
		slog.Warn("capability: daemon exited with error", "name", m.Name, "err", err)
	}
	slog.Info("capability: daemon stopped", "name", m.Name)
}

func (r *Runner) runInterval(ctx context.Context, c Capability) {
	m := c.Manifest()
	every := m.Trigger.Every
	if every == 0 {
		every = 5 * time.Minute
	}

	slog.Info("capability: interval started", "name", m.Name, "every", every)

	// Run once immediately so the first indexed batch lands without waiting
	// a full interval. This matches the existing collector behaviour where
	// the user expects fresh data shortly after `glitch serve` boots.
	if err := r.runOnce(ctx, c, Input{}, io.Discard); err != nil && ctx.Err() == nil {
		slog.Warn("capability: initial run failed", "name", m.Name, "err", err)
	}

	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("capability: interval stopped", "name", m.Name)
			return
		case <-ticker.C:
			if err := r.runOnce(ctx, c, Input{}, io.Discard); err != nil && ctx.Err() == nil {
				slog.Warn("capability: tick failed", "name", m.Name, "err", err)
			}
		}
	}
}

// runOnce drains one Invoke call's event channel, routing each event by kind
// and bulk-indexing accumulated Doc events at the end. Centralised so the
// scheduled and on-demand paths share identical sink semantics. Reports the
// completed invocation to the AfterInvoke hook (if set) so heartbeat
// consumers like the brain popover can show "ran 12s ago, indexed 3."
func (r *Runner) runOnce(ctx context.Context, c Capability, in Input, stream io.Writer) (err error) {
	if stream == nil {
		stream = io.Discard
	}
	m := c.Manifest()
	start := time.Now()
	indexed := 0
	defer func() {
		r.mu.Lock()
		hook := r.afterInvoke
		r.mu.Unlock()
		if hook != nil {
			hook(m.Name, time.Since(start), indexed, err)
		}
	}()

	ch, err := c.Invoke(ctx, in)
	if err != nil {
		return err
	}

	// Group docs by destination index so a single capability invocation can
	// emit into multiple indices if it wants. The common case is one index.
	docsByIndex := make(map[string][]any)
	for ev := range ch {
		switch ev.Kind {
		case EventDoc:
			if !m.Sink.Index {
				continue
			}
			idx := r.resolveIndex(m, ev.Index)
			docsByIndex[idx] = append(docsByIndex[idx], ev.Doc)
			indexed++
		case EventStream:
			// Always forward stream events when a writer is provided. The
			// Sink.Stream flag is advisory metadata for the assistant
			// (so it knows which capabilities can stream back to the
			// user); the runner does not gate on it.
			if _, werr := io.WriteString(stream, ev.Text); werr != nil {
				slog.Warn("capability: stream write failed", "name", m.Name, "err", werr)
			}
		case EventError:
			slog.Warn("capability: event error", "name", m.Name, "err", ev.Err)
		}
	}

	if r.indexer == nil {
		return nil
	}
	for idx, docs := range docsByIndex {
		if len(docs) == 0 {
			continue
		}
		if err := r.indexer.BulkIndex(ctx, idx, docs); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) resolveIndex(m Manifest, override string) string {
	if override != "" {
		return override
	}
	if m.Invocation.Index != "" {
		return m.Invocation.Index
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.defaultIndex
}

// NotFoundError is returned by Invoke when no capability is registered under
// the requested name. Exposed as a typed error so callers (the assistant
// router) can distinguish "you asked for something that doesn't exist" from
// "the capability ran but failed."
type NotFoundError struct {
	Name string
}

func (e *NotFoundError) Error() string { return "capability: not found: " + e.Name }
