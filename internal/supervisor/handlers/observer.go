package handlers

import (
	"context"
	"log/slog"

	"github.com/8op-org/gl1tch/internal/observer"
)

// ObserverService wraps the observer lifecycle (ES connection + query engine)
// as a supervisor-managed service. Collectors are registered separately.
type ObserverService struct {
	Engine *observer.QueryEngine
}

func NewObserverService() *ObserverService {
	return &ObserverService{}
}

func (o *ObserverService) Name() string { return "observer" }

func (o *ObserverService) Start(ctx context.Context) error {
	svc, err := observer.Start(ctx)
	if err != nil {
		slog.Info("observer: not available (start ES with 'docker compose up -d')", "err", err)
		return nil
	}
	o.Engine = svc.Engine

	<-ctx.Done()
	return nil
}
