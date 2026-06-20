package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type Runner struct {
	interval time.Duration
	logger   *slog.Logger
}

func New(interval time.Duration, logger *slog.Logger) *Runner {
	return &Runner{interval: interval, logger: logger}
}

func (r *Runner) Run(ctx context.Context, fn func(context.Context)) {
	if r.interval <= 0 {
		return
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						r.logger.Error("scheduler panic", "error", recovered)
					}
				}()
				fn(ctx)
			}()
		}
	}
}
