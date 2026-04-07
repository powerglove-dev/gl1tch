package collector

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// startTickSpan creates a short-lived child span under the long-lived
// collector.run parent so each tick is individually queryable in
// Kibana Discover and the Elastic APM Transactions UI.
//
// Why this helper exists: the original `collector.run` span wraps the
// entire goroutine lifetime — it's one span per (workspace, collector,
// pod-start) that only ends when the goroutine exits. That's fine for
// "did the collector launch" queries but useless for "did the last
// tick finish" or latency histograms, because the BatchSpanProcessor
// only exports spans AFTER `span.End()` is called. Wrapping each
// tick in its own child span gives us:
//
//   - A row in glitch-traces per tick (sortable by start_time so
//     "last 10 git ticks for workspace X" is a single query)
//   - A transaction in APM per tick so apm-server builds the latency
//     histogram Kibana's APM UI wants
//   - An error-grouped entry when a tick errors, correlated to the
//     parent collector.run span via the trace ID
//
// Every tickSpan carries the workspace_id, collector name, and tick
// duration as attributes so dashboards can group by workspace AND
// collector without re-parsing the span name.
//
// Usage:
//
//	case <-ticker.C:
//	    tickCtx, done := startTickSpan(ctx, "git", g.WorkspaceID)
//	    indexed, err := g.doTick(tickCtx, ...)
//	    done(indexed, err)
//
// The returned context is the one the tick body should pass to any
// downstream tracers / ES calls so their spans become grandchildren
// of collector.poll (and therefore great-grandchildren of
// collector.run).
func startTickSpan(ctx context.Context, collectorName, workspaceID string) (context.Context, func(indexed int, err error)) {
	start := time.Now()
	ctx, span := podTracer.Start(ctx, "collector.poll",
		oteltrace.WithAttributes(
			attribute.String("workspace_id", workspaceID),
			attribute.String("collector", collectorName),
		),
	)
	done := func(indexed int, err error) {
		span.SetAttributes(
			attribute.Int("indexed", indexed),
			attribute.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
		if err != nil {
			span.SetStatus(codes.Error, "tick error")
			span.RecordError(err)
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}
	return ctx, done
}
