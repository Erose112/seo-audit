package crawler

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiterWait(t *testing.T) {
	limiter := NewRateLimiter(20 * time.Millisecond)
	start := time.Now()
	if err := limiter.Wait(context.Background()); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Errorf("Wait returned too quickly: %s", elapsed)
	}
}

func TestRateLimiterCancel(t *testing.T) {
	limiter := NewRateLimiter(time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := limiter.Wait(ctx); err == nil {
		t.Fatal("expected cancel error")
	}
}

func TestRateLimiterZeroDelay(t *testing.T) {
	limiter := NewRateLimiter(0)
	start := time.Now()
	if err := limiter.Wait(context.Background()); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Millisecond {
		t.Errorf("zero delay should return immediately, took %s", elapsed)
	}
}
