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
// Transport timeouts and connection caps are fixed (arch §3); User-Agent and
// body limits live on Fetcher, not here.
//
// Neither Client.Timeout nor Transport.ResponseHeaderTimeout is set: both are
// per-client, not per-request, so either one would silently cap every retry at
// the same value and defeat the escalating per-attempt timeouts in
// FetchWithRetry. The context deadline passed into each attempt is the only
// timeout mechanism for request latency.
func NewClient() *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		// Dial and TLS handshake are paid at most once per connection, so they
		// stay fixed and conservative rather than tracking the retry schedule.
		TLSHandshakeTimeout: 5 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		// Sequential crawl: one in-flight request. Cap the transport to match
		// so a later concurrency change can't silently open more connections
		// than the crawl loop intends.
		MaxIdleConnsPerHost: 1,
		MaxConnsPerHost:     1,
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
