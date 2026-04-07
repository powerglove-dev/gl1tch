// glitch-backfill is a one-shot helper that runs the desktop-style
// backend (SkipGlobalCollectors:true) plus the per-workspace pod
// manager for a configurable duration so collectors can re-ingest
// into a freshly-dropped glitch-events index.
//
// Use case: you've made a mapping change, dropped the index by hand
// (`curl -X DELETE http://localhost:9200/glitch-events`), and now
// want to repopulate it from the live sources without launching the
// full desktop app. Per the no-migrations-pre-1.0 rule this is the
// supported "migration" path: drop the index, run the pods, let the
// idempotent collectors re-walk their sources.
//
// Run with:
//
//	go run ./cmd/glitch-backfill
//	BACKFILL_SECONDS=300 go run ./cmd/glitch-backfill
//
// All output goes to stderr via the standard slog handler so the
// pod manager's progress logs are visible inline.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/8op-org/gl1tch/pkg/glitchd"
)

func main() {
	dur := 120 * time.Second
	if v := os.Getenv("BACKFILL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			dur = time.Duration(n) * time.Second
		}
	}

	bgCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := glitchd.RunBackendWithOptions(bgCtx, glitchd.BackendOptions{
			SkipGlobalCollectors: true,
		})
		if err != nil {
			log.Printf("backend: %v", err)
		}
	}()

	glitchd.InitPodManager(bgCtx)
	go glitchd.StartAllWorkspacePods(bgCtx)
	// Tool pod runs copilot + mattermost ONCE under
	// glitchd.WorkspaceIDTools — same wiring as glitch-desktop's
	// app.go startup hook. Without this the backfill would only
	// repopulate per-workspace data and the brain popover's tool
	// rows (copilot, mattermost) would stay at zero.
	go func() {
		if err := glitchd.StartToolPod(); err != nil {
			log.Printf("backfill: start tool pod: %v", err)
		}
	}()

	fmt.Fprintf(os.Stderr, "glitch-backfill: backend + pods + tool pod started, ingesting for %s...\n", dur)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-time.After(dur):
	case <-sigCh:
	}

	fmt.Fprintln(os.Stderr, "glitch-backfill: shutting down pods")
	glitchd.StopAllWorkspacePods()
	_ = glitchd.StopToolPod()
	cancel()
	time.Sleep(2 * time.Second)
	fmt.Fprintln(os.Stderr, "glitch-backfill: done")
}
