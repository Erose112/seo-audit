package crawler

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// NewCrawlContext returns a root context bounded by maxDuration and canceled on
// SIGINT or SIGTERM.
func NewCrawlContext(maxDuration time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), maxDuration)
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	return ctx, func() {
		stop()
		cancel()
	}
}
