package crawler

import (
	"context"
	"time"
)

// RateLimiter paces sequential crawl requests with a fixed inter-request delay.
type RateLimiter struct {
	delay time.Duration
}

func NewRateLimiter(delay time.Duration) *RateLimiter {
	return &RateLimiter{delay: delay}
}

// Wait blocks until the next request may proceed or ctx is canceled.
func (r *RateLimiter) Wait(ctx context.Context) error {
	if r.delay <= 0 {
		return nil
	}
	select {
	case <-time.After(r.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
