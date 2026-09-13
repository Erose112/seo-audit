package crawler

// Crawl takes *Fetcher; retry config flows via fetcher.RetryConfig().
// Parse/non-HTML failures use ErrKindParseFailure in CrawlResult.Errors.
// Frontier re-derives internal links via ResolveURL + SameDomain.
// Robots checked at enqueue on resolved path+query; scheduled uses Normalize.
// After fetch, Normalize(FinalURL) is recorded in visited so redirect aliases
// of an already-crawled page are not stored twice.
// Root URL always fetched; robots gate applies only to discovered links.
// FetchRobots always returns non-nil *RobotsPolicy (fail-open).
// Root fetch before FetchRobots, then BFS (arch §12 fail-fast).
import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Erose112/seo-audit/internal/parser"
)

type CrawlOptions struct {
	MaxPages int
	MaxDepth int
	Delay    time.Duration
	// Robots, when non-nil, skips FetchRobots (tests only).
	Robots *RobotsPolicy
}

type CrawlResult struct {
	Pages  []CrawledPage
	Errors []CrawlError
}

type CrawledPage struct {
	URL       string
	Depth     int
	Data      parser.PageData
	FetchTime time.Duration
}

type queueItem struct {
	url   string
	depth int
}

type pageOutcome struct {
	page     *CrawledPage
	data     parser.PageData
	finalURL string
	fetchErr error
	parseErr *CrawlError
}

// Crawl performs a sequential BFS from startURL. Returns (result, nil) on
// normal completion and (result, err) on systemic failure.
func Crawl(ctx context.Context, fetcher *Fetcher, startURL string, opts CrawlOptions) (CrawlResult, error) {
	if fetcher == nil {
		return CrawlResult{}, fmt.Errorf("fetcher is required")
	}

	result := CrawlResult{}
	var fetchAttempts, fetchFailures int

	normalizedRoot, err := Normalize(startURL)
	if err != nil {
		return result, fmt.Errorf("normalize root URL %q: %w", startURL, err)
	}

	// scheduled: request URLs already queued (or the root seed).
	// visited: FinalURLs already recorded after a successful fetch.
	scheduled := map[string]bool{normalizedRoot: true}
	visited := map[string]bool{}
	queue := []queueItem{}
	retry := fetcher.RetryConfig()

	// Root before robots; root ignores robots gate.
	rootOutcome := fetchAndProcess(ctx, fetcher, startURL, 0, retry)
	if rootOutcome.fetchErr != nil {
		return result, fmt.Errorf("root URL unreachable: %w", rootOutcome.fetchErr)
	}
	fetchAttempts++

	robots := opts.Robots
	if robots == nil {
		robots, _ = FetchRobots(ctx, fetcher.Client(), startURL, fetcher.UserAgent())
	}

	if claimVisit(visited, scheduled, rootOutcome.finalURL) {
		if rootOutcome.parseErr != nil {
			result.Errors = append(result.Errors, *rootOutcome.parseErr)
		}
		if rootOutcome.page != nil {
			result.Pages = append(result.Pages, *rootOutcome.page)
			enqueueLinks(rootOutcome.data, startURL, 0, opts, scheduled, &queue, robots)
		}
	}

	limiter := NewRateLimiter(opts.Delay)

	for len(queue) > 0 && len(result.Pages) < opts.MaxPages {
		if err := ctx.Err(); err != nil {
			return result, &FetchError{Kind: ErrKindCanceled, Err: err}
		}

		item := queue[0]
		queue = queue[1:]

		if err := limiter.Wait(ctx); err != nil {
			return result, &FetchError{Kind: ErrKindCanceled, Err: err}
		}

		outcome := fetchAndProcess(ctx, fetcher, item.url, item.depth, retry)
		if outcome.fetchErr != nil {
			var fe *FetchError
			if errors.As(outcome.fetchErr, &fe) && fe.Kind == ErrKindCanceled {
				return result, outcome.fetchErr
			}
			fetchAttempts++
			fetchFailures++
			result.Errors = append(result.Errors, crawlErrorFromFetch(item.url, outcome.fetchErr))
			if err := checkSystemicFailureRate(fetchAttempts, fetchFailures); err != nil {
				return result, err
			}
			continue
		}

		fetchAttempts++
		if !claimVisit(visited, scheduled, outcome.finalURL) {
			continue
		}
		if outcome.parseErr != nil {
			result.Errors = append(result.Errors, *outcome.parseErr)
		}
		if outcome.page != nil {
			result.Pages = append(result.Pages, *outcome.page)
			enqueueLinks(outcome.data, startURL, item.depth, opts, scheduled, &queue, robots)
		}
	}

	if err := checkSystemicFailureRate(fetchAttempts, fetchFailures); err != nil {
		return result, err
	}
	return result, nil
}

func fetchAndProcess(
	ctx context.Context,
	fetcher *Fetcher,
	rawURL string,
	depth int,
	retry RetryConfig,
) pageOutcome {
	resp, err := fetcher.FetchWithRetry(ctx, rawURL, retry)
	if err != nil {
		return pageOutcome{fetchErr: err}
	}

	pageURL := resp.FinalURL
	if pageURL == "" {
		pageURL = rawURL
	}

	if resp.Truncated {
		return pageOutcome{
			finalURL: pageURL,
			parseErr: &CrawlError{
				URL:     pageURL,
				Kind:    ErrKindParseFailure,
				Message: "body truncated",
			},
		}
	}

	if !IsHTML(resp.ContentType) {
		return pageOutcome{
			finalURL: pageURL,
			parseErr: &CrawlError{
				URL:     pageURL,
				Kind:    ErrKindParseFailure,
				Message: fmt.Sprintf("content type %q is not HTML", resp.ContentType),
			},
		}
	}

	data, err := parser.Parse(pageURL, resp.Body)
	if err != nil {
		return pageOutcome{
			finalURL: pageURL,
			parseErr: &CrawlError{
				URL:     pageURL,
				Kind:    ErrKindParseFailure,
				Message: err.Error(),
			},
		}
	}

	return pageOutcome{
		page: &CrawledPage{
			URL:       pageURL,
			Depth:     depth,
			Data:      data,
			FetchTime: resp.Duration,
		},
		data:     data,
		finalURL: pageURL,
	}
}

// claimVisit marks Normalize(finalURL) in visited. Returns false if that
// landing page was already recorded (redirect alias or second fetch of the
// same URL after an earlier alias already claimed it). Also marks scheduled
// so the canonical URL is not enqueued again later.
func claimVisit(visited, scheduled map[string]bool, finalURL string) bool {
	if finalURL == "" {
		return true
	}
	key, err := Normalize(finalURL)
	if err != nil {
		return true
	}
	if visited[key] {
		return false
	}
	visited[key] = true
	scheduled[key] = true
	return true
}

func enqueueLinks(
	data parser.PageData,
	startURL string,
	depth int,
	opts CrawlOptions,
	scheduled map[string]bool,
	queue *[]queueItem,
	robots *RobotsPolicy,
) {
	if depth >= opts.MaxDepth {
		return
	}
	for _, link := range data.Links {
		resolved, err := ResolveURL(data.URL, link.Href)
		if err != nil {
			continue
		}
		if !SameDomain(resolved, startURL) {
			continue
		}
		path, err := robotsPath(resolved)
		if err != nil {
			continue
		}
		if robots != nil && !robots.Allowed(path) {
			continue
		}
		key, err := Normalize(resolved)
		if err != nil {
			continue
		}
		if scheduled[key] {
			continue
		}
		scheduled[key] = true
		*queue = append(*queue, queueItem{url: resolved, depth: depth + 1})
	}
}

func crawlErrorFromFetch(url string, err error) CrawlError {
	var fe *FetchError
	if errors.As(err, &fe) {
		msg := ""
		if fe.Err != nil {
			msg = fe.Err.Error()
		}
		return CrawlError{
			URL:        url,
			Kind:       fe.Kind,
			StatusCode: fe.StatusCode,
			Message:    msg,
		}
	}
	return CrawlError{URL: url, Kind: ErrKindConnection, Message: err.Error()}
}

// checkSystemicFailureRate returns ErrSystemicFailureRate when more than half
// of fetch attempts have failed after at least five attempts.
func checkSystemicFailureRate(attempts, failures int) error {
	if attempts < 5 {
		return nil
	}
	failRate := float64(failures) / float64(attempts)
	if failRate > 0.5 {
		return fmt.Errorf(
			"%w: %.0f%% of %d fetch attempts failed",
			ErrSystemicFailureRate,
			failRate*100,
			attempts,
		)
	}
	return nil
}
