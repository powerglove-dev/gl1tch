// Package collector defines the interface for background observation
// collectors. Each collector watches an external source (git, GitHub, CI,
// Claude Code, Copilot, etc.) and pushes events into Elasticsearch.
//
// Collectors are registered as individual supervisor.Service instances
// via the CollectorService adapter in internal/supervisor/handlers/.
package collector

import (
	"context"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// Collector is a background observation source.
type Collector interface {
	// Name returns the collector identifier (e.g. "git").
	Name() string
	// Start begins collecting in the background. It should block until ctx is
	// cancelled or a fatal error occurs.
	Start(ctx context.Context, es *esearch.Client) error
}
