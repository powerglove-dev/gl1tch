// Package glitchd exposes gl1tch backend services for embedding in the
// desktop GUI. This is the public API boundary — the desktop app imports
// this instead of internal/ packages directly.
package glitchd

import (
	"context"
	"sync"

	"github.com/8op-org/gl1tch/internal/bootstrap"
	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/observer"
	"github.com/8op-org/gl1tch/internal/store"
)

// RunBackend starts all background services (busd, supervisor, collectors,
// brain, cron) as goroutines. Blocks until ctx is cancelled.
func RunBackend(ctx context.Context) error {
	return bootstrap.RunHeadless(ctx)
}

// QueryEngine creates an observer query engine connected to Elasticsearch.
func QueryEngine() (*observer.QueryEngine, error) {
	cfg, err := collector.LoadConfig()
	if err != nil {
		return nil, err
	}

	es, err := esearch.New(cfg.Elasticsearch.Address)
	if err != nil {
		return nil, err
	}

	if err := es.Ping(context.Background()); err != nil {
		return nil, err
	}

	return observer.NewQueryEngine(es, cfg.Model), nil
}

// StreamAnswer queries the observer and streams tokens to the channel.
func StreamAnswer(ctx context.Context, question string, tokenCh chan<- string) error {
	qe, err := QueryEngine()
	if err != nil {
		return err
	}
	return qe.Stream(ctx, question, tokenCh)
}

// StreamAnswerScoped queries the observer scoped to specific repo names.
// repos should be directory basenames (e.g. ["gl1tch", "ensemble"]).
func StreamAnswerScoped(ctx context.Context, question string, repos []string, tokenCh chan<- string) error {
	qe, err := QueryEngine()
	if err != nil {
		return err
	}
	return qe.StreamScoped(ctx, question, repos, tokenCh)
}

// SaveMessage persists a workspace message via the store.
func SaveMessage(ctx context.Context, id, workspaceID, role, blocksJSON string, timestamp int64) error {
	st, err := OpenStore()
	if err != nil {
		return err
	}
	return st.SaveWorkspaceMessage(ctx, store.WorkspaceMessage{
		ID:          id,
		WorkspaceID: workspaceID,
		Role:        role,
		BlocksJSON:  blocksJSON,
		Timestamp:   timestamp,
	})
}

// ── Store singleton ────────────────────────────────────────────────────────

var (
	storeOnce     sync.Once
	storeInstance *store.Store
	storeErr      error
)

// OpenStore returns a singleton handle to the SQLite store.
func OpenStore() (*store.Store, error) {
	storeOnce.Do(func() {
		storeInstance, storeErr = store.Open()
	})
	return storeInstance, storeErr
}
