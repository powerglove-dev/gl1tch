package handlers

import (
	"context"
	"log/slog"

	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/cron"
)

// CronService runs the cron scheduler as a supervisor-managed service.
// Replaces the tmux-based cron daemon.
type CronService struct{}

func (c *CronService) Name() string { return "cron" }

func (c *CronService) Start(ctx context.Context) error {
	logger, err := cron.NewLogger()
	if err != nil {
		slog.Warn("cron: logger setup error", "err", err)
	}

	sockPath, _ := busd.SocketPath()
	pub := NewBusPublisher(sockPath)

	sched := cron.New(logger, cron.WithPublisher(pub))
	if err := sched.Start(ctx); err != nil {
		slog.Warn("cron: start error", "err", err)
		return nil // non-fatal
	}

	<-ctx.Done()
	sched.Stop()
	return nil
}
