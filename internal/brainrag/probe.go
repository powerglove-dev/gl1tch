package brainrag

import (
	"sync/atomic"
)

// QueryHit is an in-process counter of RAGStore read calls. Smoke tests enable
// it to prove that a `glitch ask` run actually consulted the vector index
// (i.e. the code-index capability's output was read during the query), rather
// than only trusting that the prompt looked plausible.
//
// Production code pays the cost of one atomic add per vector query; disabled
// instrumentation returns without doing even that.
var queryHitCounter atomic.Int64

// probeEnabled is flipped by tests via EnableQueryProbe. When false the hook
// is effectively free.
var probeEnabled atomic.Bool

// EnableQueryProbe starts counting vector query calls and resets the counter.
// Returns a reset+disable func so tests can defer cleanup:
//
//	defer brainrag.EnableQueryProbe()()
func EnableQueryProbe() func() {
	queryHitCounter.Store(0)
	probeEnabled.Store(true)
	return func() {
		probeEnabled.Store(false)
		queryHitCounter.Store(0)
	}
}

// QueryProbeHits returns the number of vector queries observed since the
// probe was enabled. Returns 0 when the probe is disabled.
func QueryProbeHits() int64 {
	return queryHitCounter.Load()
}

// recordQueryHit is called by RAGStore.Query / QueryWithText on every call.
// It is a no-op unless a test enabled the probe.
func recordQueryHit() {
	if probeEnabled.Load() {
		queryHitCounter.Add(1)
	}
}
