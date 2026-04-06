package observer

import (
	"context"
	"log/slog"

	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/esearch"
)

// Service manages the ES connection and query engine. Collectors are now
// registered as independent supervisor services — this service only owns
// the Elasticsearch lifecycle and QueryEngine.
type Service struct {
	ES     *esearch.Client
	Engine *QueryEngine
}

// Start connects to ES, ensures indices, and creates the query engine.
// Blocks until ctx is cancelled. If ES is not reachable, returns an error
// (callers should treat this as non-fatal).
func Start(ctx context.Context) (*Service, error) {
	cfg, err := collector.LoadConfig()
	if err != nil {
		return nil, err
	}

	es, err := esearch.New(cfg.Elasticsearch.Address)
	if err != nil {
		return nil, err
	}

	if err := es.Ping(ctx); err != nil {
		return nil, err
	}

	if err := es.EnsureIndices(ctx); err != nil {
		return nil, err
	}

	model := cfg.Model
	if model == "" {
		model = "llama3.2"
	}

	svc := &Service{
		ES:     es,
		Engine: NewQueryEngine(es, model),
	}

	slog.Info("observer: started", "es", cfg.Elasticsearch.Address)
	return svc, nil
}
