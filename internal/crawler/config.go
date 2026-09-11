package crawler

import (
	"fmt"
	"time"
)

// Config holds the crawl-wide settings the fetch layer needs. Stage 6 maps
// the CLI's config.CrawlConfig onto this; keeping it separate stops the
// internal packages from depending on Cobra flag wiring.
type Config struct {
	// MaxBodySize caps how many bytes of a response body are read.
	MaxBodySize int64
	UserAgent   string
	Retry       RetryConfig
}

// RetryConfig controls the per-page retry schedule. The per-attempt timeout
// escalates (BaseTimeout * TimeoutMultiplier^attempt) because a timeout often
// means the page needed more time, not that it is dead.
//
// Zero values are invalid: context.WithTimeout(parent, 0) expires immediately,
// so an unset MaxTotalPerPage would make every page fail before the first
// attempt. Call Validate (via NewFetcher / FetchWithRetry) rather than relying
// on silent defaults — a misconfigured schedule failing every page is worse
// than a construction error.
type RetryConfig struct {
	MaxRetries        int
	BaseTimeout       time.Duration
	TimeoutMultiplier float64
	BaseBackoff       time.Duration
	// MaxTotalPerPage is a hard ceiling across all attempts, enforced by a
	// context deadline in FetchWithRetry.
	MaxTotalPerPage time.Duration
}

const (
	DefaultMaxBodySize = 2 * 1024 * 1024
	DefaultUserAgent   = "seo-audit/0.1 (+https://github.com/Erose112/seo-audit)"
)

// DefaultRetryConfig gives 3 attempts at 15s, 22.5s, 33.75s with 1s and 2s
// backoffs between them — a nominal worst case of ~74s, under the 90s ceiling.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        2,
		BaseTimeout:       15 * time.Second,
		TimeoutMultiplier: 1.5,
		BaseBackoff:       time.Second,
		MaxTotalPerPage:   90 * time.Second,
	}
}

func DefaultConfig() Config {
	return Config{
		MaxBodySize: DefaultMaxBodySize,
		UserAgent:   DefaultUserAgent,
		Retry:       DefaultRetryConfig(),
	}
}

// Validate reports whether rc is safe to use as a FetchWithRetry schedule.
// MaxRetries may be 0 (single attempt). BaseBackoff may be 0 (no delay
// between attempts). Every other field must be strictly positive, and the
// multiplier must be at least 1 so retries cannot shrink the budget.
func (rc RetryConfig) Validate() error {
	if rc.MaxRetries < 0 {
		return fmt.Errorf("RetryConfig.MaxRetries must be >= 0, got %d", rc.MaxRetries)
	}
	if rc.BaseTimeout <= 0 {
		return fmt.Errorf("RetryConfig.BaseTimeout must be > 0, got %s", rc.BaseTimeout)
	}
	if rc.TimeoutMultiplier < 1 {
		return fmt.Errorf("RetryConfig.TimeoutMultiplier must be >= 1, got %v", rc.TimeoutMultiplier)
	}
	if rc.BaseBackoff < 0 {
		return fmt.Errorf("RetryConfig.BaseBackoff must be >= 0, got %s", rc.BaseBackoff)
	}
	if rc.MaxTotalPerPage <= 0 {
		return fmt.Errorf("RetryConfig.MaxTotalPerPage must be > 0, got %s", rc.MaxTotalPerPage)
	}
	if rc.MaxTotalPerPage < rc.BaseTimeout {
		return fmt.Errorf(
			"RetryConfig.MaxTotalPerPage (%s) must be >= BaseTimeout (%s)",
			rc.MaxTotalPerPage, rc.BaseTimeout,
		)
	}
	return nil
}
