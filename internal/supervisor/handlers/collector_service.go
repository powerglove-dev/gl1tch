package handlers

import (
	"context"
	"log/slog"

	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/esearch"
)

// CollectorService wraps a collector.Collector as a supervisor.Service.
// Each collector gets its own ES client (lazy init from config).
// Non-fatal if ES is unreachable — logs a warning and returns nil.
type CollectorService struct {
	c collector.Collector
}

// NewCollectorService wraps a collector as a supervised service.
func NewCollectorService(c collector.Collector) *CollectorService {
	return &CollectorService{c: c}
}

func (s *CollectorService) Name() string { return "collector:" + s.c.Name() }

func (s *CollectorService) Start(ctx context.Context) error {
	cfg, err := collector.LoadConfig()
	if err != nil {
		slog.Warn("collector: config error", "name", s.c.Name(), "err", err)
		return nil
	}

	es, err := esearch.New(cfg.Elasticsearch.Address)
	if err != nil {
		slog.Info("collector: ES unavailable", "name", s.c.Name(), "err", err)
		return nil
	}

	if err := es.Ping(ctx); err != nil {
		slog.Info("collector: ES not reachable", "name", s.c.Name(), "err", err)
		return nil
	}

	if err := es.EnsureIndices(ctx); err != nil {
		slog.Warn("collector: index setup failed", "name", s.c.Name(), "err", err)
		return nil
	}

	return s.c.Start(ctx, es)
}
