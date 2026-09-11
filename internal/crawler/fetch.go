package crawler

import (
	"context"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"
)

// PageResponse is a successfully fetched page. Failures are reported through
// the error return rather than a field, so that a non-nil PageResponse always
// means "there is something here worth parsing".
type PageResponse struct {
	// URL is the URL that was requested; FinalURL is where it landed after
	// redirects and is the correct base for resolving relative links.
	URL         string
	FinalURL    string
	StatusCode  int
	ContentType string
	Body        []byte
	// Truncated reports that the body hit MaxBodySize. The page is recorded as
	// a check finding rather than parsed, since truncated HTML yields silent
	// garbage extraction.
	Truncated bool
	Duration  time.Duration
}

type Fetcher struct {
	client *http.Client
	cfg    Config
}

// NewFetcher builds a Fetcher after validating the retry schedule embedded in
// cfg. MaxBodySize / UserAgent still fall back to defaults at use time; a zero
// RetryConfig does not, because context.WithTimeout(parent, 0) would make
// every page fail before the first attempt.
func NewFetcher(client *http.Client, cfg Config) (*Fetcher, error) {
	if err := cfg.Retry.Validate(); err != nil {
		return nil, err
	}
	return &Fetcher{client: client, cfg: cfg}, nil
}

// IsHTML reports whether a Content-Type is worth handing to the parser. An
// absent Content-Type is treated as HTML: some servers omit it, and the parser
// tolerates non-HTML input better than the crawl tolerates a skipped page.
func IsHTML(contentType string) bool {
	if strings.TrimSpace(contentType) == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch mediaType {
	case "text/html", "application/xhtml+xml":
		return true
	}
	return false
}

// FetchWithRetry fetches one page, retrying transient failures with an
// exponential backoff and a per-attempt timeout that grows on each retry.
//
// Every attempt runs against a single ceiling-bound child context, so
// MaxTotalPerPage is a hard stop across the whole schedule rather than the sum
// of whatever the individual attempts happened to use.
func (f *Fetcher) FetchWithRetry(parent context.Context, rawURL string, rc RetryConfig) (*PageResponse, error) {
	// rc is passed per call (tests override it), so re-validate even when
	// NewFetcher already checked cfg.Retry — a zero MaxTotalPerPage here would
	// otherwise expire the ceiling context before the first attempt.
	if err := rc.Validate(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(parent, rc.MaxTotalPerPage)
	defer cancel()

	var lastErr error
	timeout := rc.BaseTimeout

	for attempt := 0; attempt <= rc.MaxRetries; attempt++ {
		if parent.Err() != nil {
			return nil, deadContextError(parent, ctx)
		}
		if ctx.Err() != nil {
			// The per-page ceiling is spent. Returning now beats burning the
			// remaining attempts on a context that fails instantly.
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			return nil, &FetchError{
				Kind: ErrKindTimeout,
				Err:  fmt.Errorf("per-page ceiling (%s) exceeded after %d attempt(s): %w", rc.MaxTotalPerPage, attempt, lastErr),
			}
		}

		resp, err := f.fetchOnce(ctx, rawURL, timeout)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		if !isRetryable(err) {
			return nil, err
		}

		if attempt < rc.MaxRetries {
			backoff := time.Duration(float64(rc.BaseBackoff) * math.Pow(2, float64(attempt)))
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, deadContextError(parent, ctx)
			}
			timeout = time.Duration(float64(timeout) * rc.TimeoutMultiplier)
		}
	}

	if parent.Err() != nil {
		return nil, deadContextError(parent, ctx)
	}
	return nil, fmt.Errorf("all retries exhausted: %w", lastErr)
}

// deadContextError decides whether a dead context is systemic or page-level.
//
// context.DeadlineExceeded alone can't answer this: it means the same thing
// whether the root crawl budget expired or only this page's ceiling did. A dead
// parent is always systemic — including when the cause is the root deadline —
// so parent state is checked before falling back to the page-level ceiling.
func deadContextError(parent, page context.Context) error {
	if err := parent.Err(); err != nil {
		return &FetchError{Kind: ErrKindCanceled, Err: err}
	}
	return &FetchError{Kind: ErrKindTimeout, Err: page.Err()}
}

// fetchOnce performs a single GET bounded by timeout, which is derived from ctx
// so that root cancellation still preempts a long per-attempt deadline.
func (f *Fetcher) fetchOnce(ctx context.Context, rawURL string, timeout time.Duration) (*PageResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, &FetchError{Kind: ErrKindPermanent, Err: err}
	}
	// Some sites block the default Go user agent outright.
	req.Header.Set("User-Agent", f.userAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.1")

	start := time.Now()
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, classifyFetchError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, &FetchError{
			Kind:       ErrKindHTTPStatus,
			StatusCode: resp.StatusCode,
			Err:        fmt.Errorf("unexpected status %s", resp.Status),
		}
	}

	page := &PageResponse{
		URL:         rawURL,
		FinalURL:    resp.Request.URL.String(),
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
	}

	if !IsHTML(page.ContentType) {
		page.Duration = time.Since(start)
		return page, nil
	}

	// Read one byte past the cap: io.LimitReader alone can't tell a body that
	// landed exactly on the cap from one that was truncated.
	limit := f.maxBodySize()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, classifyFetchError(err)
	}
	if int64(len(data)) > limit {
		page.Truncated = true
		data = data[:limit]
	}

	page.Body = data
	page.Duration = time.Since(start)
	return page, nil
}

func (f *Fetcher) userAgent() string {
	if f.cfg.UserAgent == "" {
		return DefaultUserAgent
	}
	return f.cfg.UserAgent
}

// maxBodySize applies to decompressed bytes: http.Transport gzip-decodes
// transparently, so this is never measured against wire size.
func (f *Fetcher) maxBodySize() int64 {
	if f.cfg.MaxBodySize <= 0 {
		return DefaultMaxBodySize
	}
	return f.cfg.MaxBodySize
}
