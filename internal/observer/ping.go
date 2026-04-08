package observer

import (
	"context"

	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
)

// Ping checks ES connectivity and returns a connected client without starting
// collectors. Used by status commands that just need to query.
func Ping(ctx context.Context) (*esearch.Client, error) {
	cfg, err := capability.LoadConfig()
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

	return es, nil
}

// QueryOnly creates a query engine without starting collectors. For CLI
// commands that just need to ask questions.
func QueryOnly(ctx context.Context) (*QueryEngine, *esearch.Client, error) {
	cfg, err := capability.LoadConfig()
	if err != nil {
		return nil, nil, err
	}

	es, err := esearch.New(cfg.Elasticsearch.Address)
	if err != nil {
		return nil, nil, err
	}

	if err := es.Ping(ctx); err != nil {
		return nil, nil, err
	}

	if err := es.EnsureIndices(ctx); err != nil {
		return nil, nil, err
	}

	model := cfg.Model
	if model == "" {
		model = capability.DefaultLocalModel
	}

	return NewQueryEngine(es, model), es, nil
}
