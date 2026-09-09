package crawler

import "time"

// Config holds the crawl-wide settings the fetch layer needs. Stage 6 maps
// the CLI's config.CrawlConfig onto this; keeping it separate stops the
// internal packages from depending on Cobra flag wiring.
type Config struct {
	// Workers is the concurrent-fetch count. It also drives the transport's
	// per-host connection caps so the pool and the transport can't drift.
	Workers int
	// MaxBodySize caps how many bytes of a response body are read.
	MaxBodySize int64
	UserAgent   string
	Retry       RetryConfig
}

// RetryConfig controls the per-page retry schedule. The per-attempt timeout
// escalates (BaseTimeout * TimeoutMultiplier^attempt) because a timeout often
// means the page needed more time, not that it is dead.
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
	DefaultWorkers     = 4
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
		Workers:     DefaultWorkers,
		MaxBodySize: DefaultMaxBodySize,
		UserAgent:   DefaultUserAgent,
		Retry:       DefaultRetryConfig(),
	}
}
