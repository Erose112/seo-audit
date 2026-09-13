package crawler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const fixtureHTML = `<!DOCTYPE html><html><head><title>%s</title></head><body>%s</body></html>`

func linkPage(title string, links ...string) string {
	var b strings.Builder
	for _, href := range links {
		fmt.Fprintf(&b, `<a href="%s">link</a>`, href)
	}
	return fmt.Sprintf(fixtureHTML, title, b.String())
}

func newFixtureServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	var mu sync.Mutex
	requestLog := make([]string, 0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		mu.Lock()
		requestLog = append(requestLog, r.URL.Path)
		mu.Unlock()

		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: seo-audit\nDisallow: /private\n"))
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(linkPage("root", "/page1", "/page2", "/private", "/fail500")))
		case "/page1":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(linkPage("page1", "/page3")))
		case "/page2":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(linkPage("page2")))
		case "/page3":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(linkPage("page3")))
		case "/private":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(linkPage("private")))
		case "/fail500":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		t.Logf("request paths: %v", requestLog)
	})
	return srv, &requests
}

func crawlTestOptions() CrawlOptions {
	return CrawlOptions{
		MaxPages: 100,
		MaxDepth: 5,
		Delay:    0,
	}
}

func crawlFixture(t *testing.T, srv *httptest.Server, opts CrawlOptions) (CrawlResult, error) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Retry = fastRetry()
	fetcher, err := NewFetcher(NewClient(), cfg)
	if err != nil {
		t.Fatalf("NewFetcher: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return Crawl(ctx, fetcher, srv.URL+"/", opts)
}

func TestCrawlFixtureSite(t *testing.T) {
	srv, _ := newFixtureServer(t)
	result, err := crawlFixture(t, srv, crawlTestOptions())
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	wantPages := []string{"root", "page1", "page2", "page3"}
	if len(result.Pages) != len(wantPages) {
		t.Fatalf("got %d pages, want %d: %+v", len(result.Pages), len(wantPages), pageTitles(result.Pages))
	}
	for i, title := range wantPages {
		if result.Pages[i].Data.Title != title {
			t.Errorf("page[%d] title = %q, want %q", i, result.Pages[i].Data.Title, title)
		}
	}

	var fail500 bool
	for _, ce := range result.Errors {
		if ce.Kind == ErrKindHTTPStatus && ce.StatusCode == 500 {
			fail500 = true
		}
	}
	if !fail500 {
		t.Errorf("expected a 500 page error, got %+v", result.Errors)
	}

	for _, p := range result.Pages {
		if p.Data.Title == "private" {
			t.Error("robots-disallowed /private was crawled")
		}
	}
}

func TestCrawlDeterministicOrder(t *testing.T) {
	srv, _ := newFixtureServer(t)
	opts := crawlTestOptions()

	r1, err := crawlFixture(t, srv, opts)
	if err != nil {
		t.Fatalf("first crawl: %v", err)
	}
	r2, err := crawlFixture(t, srv, opts)
	if err != nil {
		t.Fatalf("second crawl: %v", err)
	}

	if titles := pageTitles(r1.Pages); strings.Join(titles, ",") != strings.Join(pageTitles(r2.Pages), ",") {
		t.Errorf("page order differed: %v vs %v", titles, pageTitles(r2.Pages))
	}
}

func TestCrawlRootDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	result, err := crawlFixture(t, srv, crawlTestOptions())
	if err == nil {
		t.Fatal("expected systemic error when root is down")
	}
	if len(result.Pages) != 0 {
		t.Errorf("expected no pages on root failure, got %d", len(result.Pages))
	}
}

func TestCrawlRule3EarlyExit(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/":
			w.Header().Set("Content-Type", "text/html")
			links := make([]string, 0, 20)
			for i := 1; i <= 20; i++ {
				links = append(links, fmt.Sprintf("/bad%d", i))
			}
			_, _ = w.Write([]byte(linkPage("root", links...)))
		default:
			http.Error(w, "fail", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	result, err := crawlFixture(t, srv, CrawlOptions{MaxPages: 100, MaxDepth: 2, Delay: 0})
	if !errors.Is(err, ErrSystemicFailureRate) {
		t.Fatalf("expected ErrSystemicFailureRate, got %v (pages=%d errors=%d requests=%d)",
			err, len(result.Pages), len(result.Errors), requests.Load())
	}
	if requests.Load() >= 20 {
		t.Errorf("Rule 3 should stop early; got %d requests", requests.Load())
	}
}

func TestCrawlRespectsMaxPages(t *testing.T) {
	srv, _ := newFixtureServer(t)
	result, err := crawlFixture(t, srv, CrawlOptions{MaxPages: 2, MaxDepth: 5, Delay: 0})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(result.Pages))
	}
}

func TestCrawlRespectsMaxDepth(t *testing.T) {
	srv, _ := newFixtureServer(t)
	result, err := crawlFixture(t, srv, CrawlOptions{MaxPages: 100, MaxDepth: 1, Delay: 0})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	for _, p := range result.Pages {
		if p.Depth > 1 {
			t.Errorf("page %q depth = %d, want <= 1", p.Data.Title, p.Depth)
		}
	}
	if len(result.Pages) < 3 {
		t.Fatalf("expected at least root + page1 + page2 at depth 1, got %d pages", len(result.Pages))
	}
}

func TestCrawlCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(linkPage("root", "/slow")))
		case "/slow":
			select {
			case <-time.After(2 * time.Second):
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(linkPage("slow")))
			case <-r.Context().Done():
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := DefaultConfig()
	cfg.Retry = fastRetry()
	fetcher, err := NewFetcher(NewClient(), cfg)
	if err != nil {
		t.Fatalf("NewFetcher: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = Crawl(ctx, fetcher, srv.URL+"/", CrawlOptions{MaxPages: 10, MaxDepth: 2, Delay: 0})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	var fe *FetchError
	if !errors.As(err, &fe) || fe.Kind != ErrKindCanceled {
		t.Fatalf("expected ErrKindCanceled, got %v", err)
	}
}

func TestCrawlRespectsDelay(t *testing.T) {
	var mu sync.Mutex
	times := make([]time.Time, 0, 4)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()

		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(linkPage("root", "/a", "/b")))
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(linkPage(r.URL.Path, "")))
		}
	}))
	t.Cleanup(srv.Close)

	cfg := DefaultConfig()
	cfg.Retry = fastRetry()
	fetcher, err := NewFetcher(NewClient(), cfg)
	if err != nil {
		t.Fatalf("NewFetcher: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = Crawl(ctx, fetcher, srv.URL+"/", CrawlOptions{
		MaxPages: 3,
		MaxDepth: 1,
		Delay:    30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// First request is root (no prior delay). Later BFS fetches should be spaced.
	for i := 2; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		if gap < 20*time.Millisecond {
			t.Errorf("gap between request %d and %d = %s, want >= delay", i-1, i, gap)
		}
	}
}

func TestParseFailureExcludedFromRule3(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(linkPage("root", "/badhtml")))
		case "/badhtml":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"not":"html"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	result, err := crawlFixture(t, srv, crawlTestOptions())
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	var parseFail bool
	for _, ce := range result.Errors {
		if ce.Kind == ErrKindParseFailure {
			parseFail = true
		}
	}
	if !parseFail {
		t.Fatal("expected parse failure recorded")
	}
}

func TestCrawlRedirectAliasDedup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(linkPage("root", "/page1", "/alias")))
		case "/page1":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(linkPage("page1")))
		case "/alias":
			http.Redirect(w, r, "/page1", http.StatusMovedPermanently)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	result, err := crawlFixture(t, srv, crawlTestOptions())
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	var page1Count int
	for _, p := range result.Pages {
		if p.Data.Title == "page1" {
			page1Count++
		}
	}
	if page1Count != 1 {
		t.Fatalf("page1 recorded %d times, want 1 (redirect alias should dedupe)", page1Count)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("got %d pages %v, want root + page1", len(result.Pages), pageTitles(result.Pages))
	}
}

func pageTitles(pages []CrawledPage) []string {
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.Data.Title
	}
	return out
}
