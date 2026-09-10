package crawler

import (
	"errors"
	"net"
	"net/http"
	"time"
)

// maxRedirects bounds redirect chains so a misconfigured loop can't quietly
// consume a page's whole timeout budget before failing.
const maxRedirects = 5

// ErrTooManyRedirects is a sentinel so classifyFetchError can route a redirect
// loop to a non-retryable kind instead of the generic connection bucket.
var ErrTooManyRedirects = errors.New("stopped after 5 redirects")

// NewClient builds the single client shared by every request in a crawl. All
// requests target one host, so connection reuse is worth the shared transport.
//
// Neither Client.Timeout nor Transport.ResponseHeaderTimeout is set: both are
// per-client, not per-request, so either one would silently cap every retry at
// the same value and defeat the escalating per-attempt timeouts in
// FetchWithRetry. The context deadline passed into each attempt is the only
// timeout mechanism for request latency.
func NewClient(cfg Config) *http.Client {
	workers := cfg.Workers
	if workers < 1 {
		// A zero value here is a programming error, not user input; flag-level
		// validation of --workers lands with the worker pool in Stage 6.
		workers = DefaultWorkers
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		// Dial and TLS handshake are paid at most once per connection, so they
		// stay fixed and conservative rather than tracking the retry schedule.
		TLSHandshakeTimeout: 5 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: workers,
		MaxConnsPerHost:     workers,
	}

	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return ErrTooManyRedirects
			}
			return nil
		},
	}
}
